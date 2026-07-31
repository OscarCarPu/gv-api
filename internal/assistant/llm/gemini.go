package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiProvider is a Provider backed by Google's Gemini API (REST, no SDK). The
// default model (gemini-2.5-flash-lite) is Google's cheapest. Structured output
// for Decide uses JSON mode (responseMimeType=application/json).
type GeminiProvider struct {
	apiKey string
	model  string
	http   *http.Client
	base   string
}

func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	return &GeminiProvider{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second},
		base:   "https://generativelanguage.googleapis.com/v1beta",
	}
}

// geminiDecisionInstruction is appended to the shared system prompt so Gemini
// returns exactly the Decision JSON.
const geminiDecisionInstruction = `Responde ÚNICAMENTE con un objeto JSON válido (sin markdown, sin texto extra) con esta forma:
{"kind":"read|write|reject","sql":"...","action":{"domain":"...","operation":"...","args":{}},"explanation":"...","needs_summary":true,"reject":"..."}
Incluye "sql" solo si kind=read; "action" solo si kind=write; "reject" solo si kind=reject. "args" es un objeto JSON con los argumentos de la acción.`

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenConfig struct {
	ResponseMimeType string `json:"responseMimeType,omitempty"`
	MaxOutputTokens  int    `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	Contents          []geminiContent  `json:"contents"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int64 `json:"promptTokenCount"`
		CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
		CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
		TotalTokenCount         int64 `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (p *GeminiProvider) generate(ctx context.Context, systemPrompt, userText string, jsonMode bool, maxTokens int) (string, Usage, error) {
	genCfg := &geminiGenConfig{MaxOutputTokens: maxTokens}
	if jsonMode {
		genCfg.ResponseMimeType = "application/json"
	}
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: userText}}}},
		GenerationConfig:  genCfg,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", Usage{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", p.base, p.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.apiKey)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", Usage{}, fmt.Errorf("gemini status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var gr geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return "", Usage{}, err
	}

	var text strings.Builder
	if len(gr.Candidates) > 0 {
		for _, part := range gr.Candidates[0].Content.Parts {
			text.WriteString(part.Text)
		}
	}

	// Gemini reports promptTokenCount as the full prompt; cachedContentTokenCount
	// is the cached subset. Keep the buckets mutually exclusive for costing.
	inputTokens := max(gr.UsageMetadata.PromptTokenCount-gr.UsageMetadata.CachedContentTokenCount, 0)
	usage := Usage{
		Model:           p.model,
		InputTokens:     inputTokens,
		OutputTokens:    gr.UsageMetadata.CandidatesTokenCount,
		CacheReadTokens: gr.UsageMetadata.CachedContentTokenCount,
	}
	return text.String(), usage, nil
}

func (p *GeminiProvider) Decide(ctx context.Context, in DecideInput) (Decision, Usage, error) {
	system := in.SystemPrompt + "\n\n" + geminiDecisionInstruction
	userText := in.UserText
	if in.Prior != nil {
		priorJSON, _ := json.Marshal(in.Prior)
		userText = fmt.Sprintf("Propuesta anterior: %s\n\nEl usuario da feedback para corregirla: %s", string(priorJSON), in.Feedback)
	}

	text, usage, err := p.generate(ctx, system, userText, true, maxDecideTokens)
	usage.Phase = PhaseDecide
	if err != nil {
		return Decision{}, usage, err
	}

	d, perr := parseDecisionJSON(text)
	if perr != nil {
		return Decision{Kind: KindReject, Reject: "No pude interpretar la petición. Reformula, por favor."}, usage, nil
	}
	return d, usage, nil
}

func (p *GeminiProvider) Summarize(ctx context.Context, in SummarizeInput) (string, Usage, error) {
	system := "Resume en español, de forma breve y clara, el resultado de una consulta para el usuario. No muestres SQL."
	var b strings.Builder
	fmt.Fprintf(&b, "Petición: %s\n", in.UserText)
	fmt.Fprintf(&b, "Columnas: %s\n", strings.Join(in.Columns, ", "))
	b.WriteString("Filas:\n")
	b.WriteString(renderRows(in.Columns, in.Rows, 50))
	if in.Truncated {
		b.WriteString("\n(resultado truncado)\n")
	}

	text, usage, err := p.generate(ctx, system, b.String(), false, maxSummaryTokens)
	usage.Phase = PhaseSummarize
	return strings.TrimSpace(text), usage, err
}

// parseDecisionJSON unmarshals the model's JSON response into a Decision,
// tolerating stray code fences if the model adds them.
func parseDecisionJSON(text string) (Decision, error) {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var d Decision
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return Decision{}, err
	}
	return d, nil
}
