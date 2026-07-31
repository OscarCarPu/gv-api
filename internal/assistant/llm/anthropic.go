package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider is the default Provider, backed by Claude (Haiku 4.5 by
// default). The vendor SDK is confined to this file.
type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

// NewAnthropicProvider builds a provider for the given API key and model id.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

const maxDecideTokens = 1024
const maxSummaryTokens = 1024

// decisionToolSchema is the input schema for the emit_decision tool. It is
// intentionally permissive (no strict mode) because action.args is free-form;
// the ActionRegistry re-validates every write.
func decisionToolSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"kind":          map[string]any{"type": "string", "enum": []string{"read", "write", "reject"}},
			"sql":           map[string]any{"type": "string", "description": "SELECT de solo lectura (solo si kind=read)"},
			"explanation":   map[string]any{"type": "string", "description": "Qué hará la consulta o acción, en español"},
			"needs_summary": map[string]any{"type": "boolean"},
			"reject":        map[string]any{"type": "string", "description": "Motivo si kind=reject"},
			"action": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"domain":    map[string]any{"type": "string"},
					"operation": map[string]any{"type": "string"},
					"args":      map[string]any{"type": "object"},
				},
			},
		},
		Required: []string{"kind", "explanation"},
	}
}

func (p *AnthropicProvider) Decide(ctx context.Context, in DecideInput) (Decision, Usage, error) {
	userText := in.UserText
	if in.Prior != nil {
		priorJSON, _ := json.Marshal(in.Prior)
		userText = fmt.Sprintf("Propuesta anterior: %s\n\nEl usuario da feedback para corregirla: %s", string(priorJSON), in.Feedback)
	}

	tool := anthropic.ToolParam{
		Name:        "emit_decision",
		Description: anthropic.String("Devuelve la decisión estructurada (read/write/reject) para la petición del usuario."),
		InputSchema: decisionToolSchema(),
	}

	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxDecideTokens,
		System: []anthropic.TextBlockParam{{
			Text:         in.SystemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userText)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: "emit_decision"},
		},
	})
	if err != nil {
		return Decision{}, Usage{}, err
	}

	usage := p.usage(msg, PhaseDecide)

	for _, block := range msg.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == "emit_decision" {
			var d Decision
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &d); err != nil {
				return Decision{}, usage, fmt.Errorf("decode decision: %w", err)
			}
			return d, usage, nil
		}
	}
	return Decision{Kind: KindReject, Reject: "No pude interpretar la petición. Reformula, por favor."}, usage, nil
}

func (p *AnthropicProvider) Summarize(ctx context.Context, in SummarizeInput) (string, Usage, error) {
	var b strings.Builder
	b.WriteString("Resume en español, de forma breve y clara, el resultado de esta consulta para el usuario. No muestres SQL.\n\n")
	fmt.Fprintf(&b, "Petición: %s\n", in.UserText)
	fmt.Fprintf(&b, "Columnas: %s\n", strings.Join(in.Columns, ", "))
	b.WriteString("Filas:\n")
	b.WriteString(renderRows(in.Columns, in.Rows, 50))
	if in.Truncated {
		b.WriteString("\n(resultado truncado)\n")
	}

	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: maxSummaryTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(b.String())),
		},
	})
	if err != nil {
		return "", Usage{}, err
	}
	usage := p.usage(msg, PhaseSummarize)

	var out strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			out.WriteString(tb.Text)
		}
	}
	return strings.TrimSpace(out.String()), usage, nil
}

// usage maps the SDK usage onto our Usage, tagging the controlled model id (not
// resp.Model, which is a dated snapshot that would miss the price table).
func (p *AnthropicProvider) usage(msg *anthropic.Message, phase string) Usage {
	return Usage{
		Model:            p.model,
		Phase:            phase,
		InputTokens:      msg.Usage.InputTokens,
		OutputTokens:     msg.Usage.OutputTokens,
		CacheReadTokens:  msg.Usage.CacheReadInputTokens,
		CacheWriteTokens: msg.Usage.CacheCreationInputTokens,
	}
}

// renderRows formats up to max rows as a compact text table for the summarizer.
func renderRows(cols []string, rows [][]any, max int) string {
	var b strings.Builder
	for i, row := range rows {
		if i >= max {
			fmt.Fprintf(&b, "… (%d filas más)\n", len(rows)-max)
			break
		}
		parts := make([]string, len(row))
		for j, v := range row {
			name := ""
			if j < len(cols) {
				name = cols[j] + "="
			}
			parts[j] = fmt.Sprintf("%s%v", name, v)
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString("\n")
	}
	return b.String()
}
