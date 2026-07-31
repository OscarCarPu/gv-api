package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gv-api/internal/assistant/llm"
	"gv-api/internal/config"

	"github.com/shopspring/decimal"
)

// UsageRecorder persists and aggregates LLM usage. Implemented by
// *UsageRepository; abstracted for testing.
type UsageRecorder interface {
	Record(ctx context.Context, u llm.Usage, cost decimal.Decimal) error
	AggregateMonth(ctx context.Context, start, end time.Time, tz string) (MonthlyUsage, error)
}

// ServiceInterface is the assistant's public surface (mockable for handler tests).
type ServiceInterface interface {
	Suggest(ctx context.Context, req SuggestRequest) (SuggestResponse, error)
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error)
	MonthlyUsage(ctx context.Context, month time.Time) (MonthlyUsage, error)
}

// Service orchestrates the suggest/execute flow and meters LLM cost.
type Service struct {
	provider llm.Provider
	exec     *ReadExecutor
	registry *ActionRegistry
	usage    UsageRecorder
	tok      *tokenizer
	prices   map[string]config.ModelPrice
	loc      *time.Location
	system   string // stable system prompt (schema + rules + action catalog)
}

// NewService builds the assistant service. tokenTTL bounds how long an approved
// suggestion stays executable.
func NewService(
	provider llm.Provider,
	exec *ReadExecutor,
	registry *ActionRegistry,
	usage UsageRecorder,
	signingSecret string,
	tokenTTL time.Duration,
	prices map[string]config.ModelPrice,
	loc *time.Location,
) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{
		provider: provider,
		exec:     exec,
		registry: registry,
		usage:    usage,
		tok:      newTokenizer(signingSecret, tokenTTL),
		prices:   prices,
		loc:      loc,
		system:   buildSystemPrompt(registry),
	}
}

// Suggest turns the user's text (optionally refining a prior suggestion carried
// in req.Token) into a signed, approvable proposal.
func (s *Service) Suggest(ctx context.Context, req SuggestRequest) (SuggestResponse, error) {
	in := llm.DecideInput{SystemPrompt: s.system, UserText: req.Text}
	if req.Token != "" {
		p, err := s.tok.verify(req.Token)
		if err != nil {
			return SuggestResponse{}, err
		}
		prior := priorFromPayload(p)
		in.Prior = &prior
		in.Feedback = req.Text
	}

	decision, usage, err := s.provider.Decide(ctx, in)
	if err != nil {
		return SuggestResponse{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	s.meter(ctx, usage)

	switch decision.Kind {
	case llm.KindReject:
		return SuggestResponse{Kind: llm.KindReject, Explanation: rejectText(decision)}, nil
	case llm.KindRead:
		token, err := s.tok.sign(decision)
		if err != nil {
			return SuggestResponse{}, err
		}
		return SuggestResponse{Kind: llm.KindRead, Explanation: decision.Explanation, Query: decision.SQL, Token: token}, nil
	case llm.KindWrite:
		if decision.Action == nil {
			return SuggestResponse{Kind: llm.KindReject, Explanation: "No pude construir la acción. Reformula, por favor."}, nil
		}
		token, err := s.tok.sign(decision)
		if err != nil {
			return SuggestResponse{}, err
		}
		return SuggestResponse{
			Kind:        llm.KindWrite,
			Explanation: decision.Explanation,
			Query:       fmt.Sprintf("%s.%s %s", decision.Action.Domain, decision.Action.Operation, string(decision.Action.Args)),
			Warning:     "Esta acción modifica datos.",
			Token:       token,
		}, nil
	default:
		return SuggestResponse{Kind: llm.KindReject, Explanation: "No entendí la petición. Reformula, por favor."}, nil
	}
}

// Execute runs an approved suggestion carried in the opaque token.
func (s *Service) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResponse, error) {
	p, err := s.tok.verify(req.Token)
	if err != nil {
		return ExecuteResponse{}, err
	}

	switch p.Kind {
	case llm.KindRead:
		res, err := s.exec.Run(ctx, p.SQL)
		if err != nil {
			return ExecuteResponse{}, err
		}
		n := len(res.Rows)
		summary := ""
		if p.NeedsSummary {
			out, usage, serr := s.provider.Summarize(ctx, llm.SummarizeInput{
				SystemPrompt: s.system,
				UserText:     p.Explanation,
				SQL:          p.SQL,
				Columns:      res.Columns,
				Rows:         res.Rows,
				Truncated:    res.Truncated,
			})
			if serr != nil {
				return ExecuteResponse{}, fmt.Errorf("%w: %v", ErrProvider, serr)
			}
			s.meter(ctx, usage)
			summary = out
		}
		if summary == "" {
			summary = fmt.Sprintf("La consulta devolvió %d fila(s).", n)
		}
		return ExecuteResponse{Kind: llm.KindRead, Summary: summary, RowCount: &n}, nil

	case llm.KindWrite:
		confirmation, err := s.registry.Execute(ctx, p.Action)
		if err != nil {
			// Pass validation/unsupported errors through; wrap other domain
			// failures (e.g. a delete blocked by dependencies) so the handler
			// returns a clean 422 instead of a scary 500.
			if errors.Is(err, ErrInvalidAction) || errors.Is(err, ErrUnsupportedAction) {
				return ExecuteResponse{}, err
			}
			return ExecuteResponse{}, fmt.Errorf("%w: %v", ErrActionFailed, err)
		}
		return ExecuteResponse{Kind: llm.KindWrite, Summary: confirmation}, nil

	default:
		return ExecuteResponse{}, ErrBadToken
	}
}

// MonthlyUsage aggregates LLM spend for the calendar month containing `month`,
// using the configured timezone for month boundaries and day bucketing.
func (s *Service) MonthlyUsage(ctx context.Context, month time.Time) (MonthlyUsage, error) {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.loc)
	end := start.AddDate(0, 1, 0)
	mu, err := s.usage.AggregateMonth(ctx, start, end, s.loc.String())
	if err != nil {
		return MonthlyUsage{}, err
	}
	mu.Month = start.Format("2006-01")
	mu.Currency = "USD"
	return mu, nil
}

// meter computes and records the cost of one LLM call, best-effort: a failure is
// logged but never fails the user's request.
func (s *Service) meter(ctx context.Context, u llm.Usage) {
	var cost decimal.Decimal
	if price, ok := s.prices[u.Model]; ok {
		cost = Cost(price, u)
	} else {
		slog.WarnContext(ctx, "no price configured for model; recording zero cost", "model", u.Model)
	}
	if err := s.usage.Record(ctx, u, cost); err != nil {
		slog.ErrorContext(ctx, "failed to record assistant usage", "error", err)
	}
}

func priorFromPayload(p tokenPayload) llm.Decision {
	return llm.Decision{
		Kind:         p.Kind,
		SQL:          p.SQL,
		Action:       p.Action,
		Explanation:  p.Explanation,
		NeedsSummary: p.NeedsSummary,
	}
}

func rejectText(d llm.Decision) string {
	if d.Reject != "" {
		return d.Reject
	}
	if d.Explanation != "" {
		return d.Explanation
	}
	return "No entendí la petición. Reformula, por favor."
}
