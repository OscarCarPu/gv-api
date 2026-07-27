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
	provider   llm.Provider
	exec       *ReadExecutor
	registry   *ActionRegistry
	usage      UsageRecorder
	tok        *tokenizer
	prices     map[string]config.ModelPrice
	loc        *time.Location
	system     string // stable system prompt (schema + rules + action catalog)
	maxQueries int    // internal reads the model may run per Suggest
}

// NewService builds the assistant service. tokenTTL bounds how long an approved
// suggestion stays executable; maxQueries bounds the read-only queries the model
// may run on its own while deciding (non-positive disables exploration).
func NewService(
	provider llm.Provider,
	exec *ReadExecutor,
	registry *ActionRegistry,
	usage UsageRecorder,
	signingSecret string,
	tokenTTL time.Duration,
	maxQueries int,
	prices map[string]config.ModelPrice,
	loc *time.Location,
) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{
		provider:   provider,
		exec:       exec,
		registry:   registry,
		usage:      usage,
		tok:        newTokenizer(signingSecret, tokenTTL),
		prices:     prices,
		loc:        loc,
		system:     buildSystemPrompt(registry),
		maxQueries: maxQueries,
	}
}

// queryRunner gives the model auto-approved read access while it decides. Reads
// go through the same ReadExecutor as approved ones (read-only transaction,
// statement timeout, row/column caps), so nothing here can modify data. The
// budget is enforced server-side rather than trusted to the prompt, and every
// query is recorded so the proposal can show what informed it.
type queryRunner struct {
	exec  *ReadExecutor
	limit int
	steps []SuggestStep
}

// RunQuery executes one internal read. A failed query is reported back to the
// model (so it can fix its SQL) rather than failing the request; only a
// cancelled context aborts the loop.
func (r *queryRunner) RunQuery(ctx context.Context, sql string) (llm.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return llm.QueryResult{}, err
	}
	if len(r.steps) >= r.limit {
		return llm.QueryResult{Err: "límite de consultas internas alcanzado"}, nil
	}
	step := SuggestStep{SQL: sql}

	res, err := r.exec.Run(ctx, sql)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return llm.QueryResult{}, ctxErr
		}
		slog.InfoContext(ctx, "assistant internal read failed", "sql", sql, "error", err)
		step.Error = err.Error()
		r.steps = append(r.steps, step)
		return llm.QueryResult{Err: err.Error()}, nil
	}

	step.RowCount = len(res.Rows)
	r.steps = append(r.steps, step)
	return llm.QueryResult{Columns: res.Columns, Rows: res.Rows, Truncated: res.Truncated}, nil
}

// Suggest turns the user's text (optionally refining a prior suggestion carried
// in req.Token) into a signed, approvable proposal. While deciding, the model may
// run up to maxQueries read-only queries of its own — auto-approved, since a read
// cannot modify data — and those are reported back in Steps.
func (s *Service) Suggest(ctx context.Context, req SuggestRequest) (SuggestResponse, error) {
	in := llm.DecideInput{
		SystemPrompt: s.system,
		UserText:     req.Text,
		// The system prompt is built once at startup, so today's date has to ride
		// along with the request — otherwise "hoy" resolves to whenever the
		// process booted, or to the model's training-time guess.
		Today: time.Now().In(s.loc).Format("2006-01-02"),
	}
	if req.Token != "" {
		p, err := s.tok.verify(req.Token)
		if err != nil {
			return SuggestResponse{}, err
		}
		prior := priorFromPayload(p)
		in.Prior = &prior
		in.Feedback = req.Text
	}

	var runner *queryRunner
	if s.maxQueries > 0 && s.exec != nil {
		runner = &queryRunner{exec: s.exec, limit: s.maxQueries}
		in.Runner = runner
		in.MaxQueries = s.maxQueries
	}

	res, err := s.provider.Decide(ctx, in)
	// Meter every model call the loop made, even on failure: the tokens of the
	// rounds that did succeed were still spent.
	for _, u := range res.Usages {
		s.meter(ctx, u)
	}
	if err != nil {
		return SuggestResponse{}, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	decision := res.Decision

	var steps []SuggestStep
	if runner != nil {
		steps = runner.steps
	}

	switch decision.Kind {
	case llm.KindReject:
		return SuggestResponse{Kind: llm.KindReject, Explanation: rejectText(decision), Steps: steps}, nil
	case llm.KindRead:
		token, err := s.tok.sign(decision)
		if err != nil {
			return SuggestResponse{}, err
		}
		return SuggestResponse{Kind: llm.KindRead, Explanation: decision.Explanation, Query: decision.SQL, Token: token, Steps: steps}, nil
	case llm.KindWrite:
		if decision.Action == nil {
			return SuggestResponse{Kind: llm.KindReject, Explanation: "No pude construir la acción. Reformula, por favor.", Steps: steps}, nil
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
			Steps:       steps,
		}, nil
	default:
		return SuggestResponse{Kind: llm.KindReject, Explanation: "No entendí la petición. Reformula, por favor.", Steps: steps}, nil
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
