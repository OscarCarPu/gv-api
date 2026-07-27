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
// opts are extra SDK request options (tests point the client at a stub server).
func NewAnthropicProvider(apiKey, model string, opts ...option.RequestOption) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)...),
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

// queryToolSchema is the input schema for the run_read_query tool the model uses
// to look things up before deciding.
func queryToolSchema() anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"sql":    map[string]any{"type": "string", "description": "Una única consulta SELECT/WITH de solo lectura (PostgreSQL)"},
			"reason": map[string]any{"type": "string", "description": "Qué quieres averiguar con ella, en español"},
		},
		Required: []string{"sql"},
	}
}

// Decide runs the tool loop: the model may call run_read_query as many times as
// its budget allows (results are fed straight back, no user approval — reads
// cannot modify data) and finishes by calling emit_decision.
func (p *AnthropicProvider) Decide(ctx context.Context, in DecideInput) (DecideResult, error) {
	var out DecideResult

	userText := in.UserText
	if in.Prior != nil {
		priorJSON, _ := json.Marshal(in.Prior)
		userText = fmt.Sprintf("Propuesta anterior: %s\n\nEl usuario da feedback para corregirla: %s", string(priorJSON), in.Feedback)
	}
	if in.Today != "" {
		userText = fmt.Sprintf("Hoy es %s.\n\n%s", in.Today, userText)
	}

	decisionTool := anthropic.ToolParam{
		Name:        "emit_decision",
		Description: anthropic.String("Devuelve la decisión estructurada (read/write/reject) para la petición del usuario."),
		InputSchema: decisionToolSchema(),
	}
	queryTool := anthropic.ToolParam{
		Name:        "run_read_query",
		Description: anthropic.String("Ejecuta una consulta interna de solo lectura y devuelve sus filas. Úsala para informarte antes de decidir."),
		InputSchema: queryToolSchema(),
	}

	system := in.SystemPrompt
	tools := []anthropic.ToolUnionParam{{OfTool: &decisionTool}}
	remaining := 0
	if in.Runner != nil && in.MaxQueries > 0 {
		remaining = in.MaxQueries
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &queryTool})
		system += "\n\n" + fmt.Sprintf(exploreSystemNote, in.MaxQueries)
	}

	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(userText))}

	for {
		// Once the budget is spent, force emit_decision so the loop always
		// terminates with an answer instead of more questions.
		choice := anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{Name: "emit_decision"}}
		if remaining > 0 {
			choice = anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
		}

		msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(p.model),
			MaxTokens: maxDecideTokens,
			System: []anthropic.TextBlockParam{{
				Text:         system,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}},
			Messages:   messages,
			Tools:      tools,
			ToolChoice: choice,
		})
		if err != nil {
			return out, err
		}
		// Provisionally an exploratory round; relabelled below if this call is
		// the one that produced the decision.
		usage := p.usage(msg, PhaseExplore)

		var queries []anthropic.ToolUseBlock
		for _, block := range msg.Content {
			tu, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			switch tu.Name {
			case "emit_decision":
				usage.Phase = PhaseDecide
				out.Usages = append(out.Usages, usage)
				var d Decision
				if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &d); err != nil {
					return out, fmt.Errorf("decode decision: %w", err)
				}
				out.Decision = d
				return out, nil
			case "run_read_query":
				queries = append(queries, tu)
			}
		}

		out.Usages = append(out.Usages, usage)
		if len(queries) == 0 {
			// No decision and nothing to look up: give up rather than loop.
			out.Decision = Decision{Kind: KindReject, Reject: "No pude interpretar la petición. Reformula, por favor."}
			return out, nil
		}

		// The model may ask for several queries at once; every tool_use needs a
		// matching tool_result in a single user message.
		messages = append(messages, msg.ToParam())
		results := make([]anthropic.ContentBlockParamUnion, 0, len(queries))
		for _, q := range queries {
			var args struct {
				SQL string `json:"sql"`
			}
			if err := json.Unmarshal([]byte(q.JSON.Input.Raw()), &args); err != nil {
				results = append(results, anthropic.NewToolResultBlock(q.ID, "ERROR: argumentos ilegibles", true))
				continue
			}
			if remaining <= 0 {
				results = append(results, anthropic.NewToolResultBlock(q.ID, budgetExhaustedNote, true))
				continue
			}
			remaining--
			res, err := in.Runner.RunQuery(ctx, args.SQL)
			if err != nil {
				return out, err
			}
			results = append(results, anthropic.NewToolResultBlock(q.ID, renderQueryResult(res), res.Err != ""))
		}
		messages = append(messages, anthropic.NewUserMessage(results...))
	}
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
