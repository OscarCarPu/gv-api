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
// returns exactly the Decision JSON. %s is the list of valid kinds: "explore" is
// only offered when the model may run internal reads, and it must appear in this
// enumeration or the model treats it as an invalid value and never uses it.
const geminiDecisionInstruction = `Responde ÚNICAMENTE con un objeto JSON válido (sin markdown, sin texto extra) con esta forma:
{"kind":"%s","sql":"...","action":{"domain":"...","operation":"...","args":{}},"explanation":"...","needs_summary":true,"reject":"..."}
Incluye "sql" solo si kind=read; "action" solo si kind=write; "reject" solo si kind=reject. "args" es un objeto JSON con los argumentos de la acción.`

// geminiExploreInstruction documents the exploration turn for Gemini, which has
// no tool-calling here: it asks for internal reads by returning a different
// JSON shape, and we feed the results back as the next user turn.
const geminiExploreInstruction = `Con kind="explore" pides consultas internas en lugar de decidir. En ese caso el objeto es exactamente:
{"kind":"explore","queries":["SELECT ..."]}
Te devolveré los resultados y entonces podrás pedir más consultas o emitir la decisión final.`

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

// generate posts one turn of the conversation. contents is the full history so
// far (exploration rounds append to it).
func (p *GeminiProvider) generate(ctx context.Context, systemPrompt string, contents []geminiContent, jsonMode bool, maxTokens int) (string, Usage, error) {
	genCfg := &geminiGenConfig{MaxOutputTokens: maxTokens}
	if jsonMode {
		genCfg.ResponseMimeType = "application/json"
	}
	reqBody := geminiRequest{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		Contents:          contents,
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

func (p *GeminiProvider) Decide(ctx context.Context, in DecideInput) (DecideResult, error) {
	var out DecideResult

	userText := in.UserText
	if in.Prior != nil {
		priorJSON, _ := json.Marshal(in.Prior)
		userText = fmt.Sprintf("Propuesta anterior: %s\n\nEl usuario da feedback para corregirla: %s", string(priorJSON), in.Feedback)
	}

	if in.Today != "" {
		userText = fmt.Sprintf("Hoy es %s.\n\n%s", in.Today, userText)
	}

	kinds := "read|write|reject"
	remaining := 0
	if in.Runner != nil && in.MaxQueries > 0 {
		remaining = in.MaxQueries
		kinds = "read|write|reject|explore"
	}
	system := in.SystemPrompt + "\n\n" + fmt.Sprintf(geminiDecisionInstruction, kinds)
	if remaining > 0 {
		system += "\n\n" + fmt.Sprintf(exploreSystemNote, in.MaxQueries) + "\n" + geminiExploreInstruction
	}

	contents := []geminiContent{{Role: "user", Parts: []geminiPart{{Text: userText}}}}
	reject := func(u Usage) DecideResult {
		u.Phase = PhaseDecide
		out.Usages = append(out.Usages, u)
		out.Decision = Decision{Kind: KindReject, Reject: "No pude interpretar la petición. Reformula, por favor."}
		return out
	}

	for {
		text, usage, err := p.generate(ctx, system, contents, true, maxDecideTokens)
		if err != nil {
			usage.Phase = PhaseDecide
			out.Usages = append(out.Usages, usage)
			return out, err
		}

		// An exploration turn is only honoured while budget remains; once it is
		// spent the reply is read as the final decision.
		if remaining > 0 {
			if queries := parseExploreJSON(text); len(queries) > 0 {
				usage.Phase = PhaseExplore
				out.Usages = append(out.Usages, usage)

				var fed strings.Builder
				fed.WriteString("Resultados de tus consultas internas:\n")
				for _, sql := range queries {
					fmt.Fprintf(&fed, "\n-- %s\n", sql)
					if remaining <= 0 {
						fed.WriteString(budgetExhaustedNote + "\n")
						continue
					}
					remaining--
					res, rerr := in.Runner.RunQuery(ctx, sql)
					if rerr != nil {
						return out, rerr
					}
					fed.WriteString(renderQueryResult(res))
				}
				fed.WriteString("\nAhora emite la decisión final, o pide más consultas si aún te faltan datos.")

				contents = append(contents,
					geminiContent{Role: "model", Parts: []geminiPart{{Text: text}}},
					geminiContent{Role: "user", Parts: []geminiPart{{Text: fed.String()}}},
				)
				continue
			}
		}

		d, perr := parseDecisionJSON(text)
		if perr != nil {
			return reject(usage), nil
		}
		usage.Phase = PhaseDecide
		out.Usages = append(out.Usages, usage)
		out.Decision = d
		return out, nil
	}
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

	contents := []geminiContent{{Role: "user", Parts: []geminiPart{{Text: b.String()}}}}
	text, usage, err := p.generate(ctx, system, contents, false, maxSummaryTokens)
	usage.Phase = PhaseSummarize
	return strings.TrimSpace(text), usage, err
}

// stripFences removes the code fences Gemini sometimes wraps JSON in.
func stripFences(text string) string {
	s := strings.TrimSpace(text)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// parseDecisionJSON unmarshals the model's JSON response into a Decision.
func parseDecisionJSON(text string) (Decision, error) {
	var d Decision
	if err := json.Unmarshal([]byte(stripFences(text)), &d); err != nil {
		return Decision{}, err
	}
	return d, nil
}

// parseExploreJSON returns the queries of an exploration turn, or nil when the
// reply is a decision instead.
func parseExploreJSON(text string) []string {
	var e exploreEnvelope
	if err := json.Unmarshal([]byte(stripFences(text)), &e); err != nil {
		return nil
	}
	if e.Kind != "explore" {
		return nil
	}
	out := make([]string, 0, len(e.Queries))
	for _, q := range e.Queries {
		if q = strings.TrimSpace(q); q != "" {
			out = append(out, q)
		}
	}
	return out
}
