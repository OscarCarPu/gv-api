package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gv-api/internal/assistant/llm"

	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	decision    llm.Decision
	decideUsage llm.Usage
	summary     string
	sumUsage    llm.Usage
	lastDecide  llm.DecideInput
}

func (f *fakeProvider) Decide(_ context.Context, in llm.DecideInput) (llm.Decision, llm.Usage, error) {
	f.lastDecide = in
	return f.decision, f.decideUsage, nil
}

func (f *fakeProvider) Summarize(_ context.Context, _ llm.SummarizeInput) (string, llm.Usage, error) {
	return f.summary, f.sumUsage, nil
}

func newTestService(p llm.Provider) (*Service, *capturingRecorder, *fakeTaskService) {
	tw := &fakeTaskService{}
	reg := NewActionRegistry(tw, &fakeHabitService{}, &fakeFinanceService{}, &fakePlanService{}, &fakeRutasService{}, &fakeVarietyService{})
	rec := &capturingRecorder{}
	svc := NewService(p, NewReadExecutor(nil, 0, 0, 0), reg, rec, "secret", time.Minute, haikuPrices(), time.UTC)
	return svc, rec, tw
}

func TestSuggest_Read_ReturnsTokenAndMeters(t *testing.T) {
	p := &fakeProvider{
		decision:    llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1", Explanation: "uno", NeedsSummary: true},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5", Phase: llm.PhaseDecide, InputTokens: 100, OutputTokens: 10},
	}
	svc, rec, _ := newTestService(p)

	out, err := svc.Suggest(context.Background(), SuggestRequest{Text: "cuántos hábitos hay"})
	require.NoError(t, err)
	require.Equal(t, llm.KindRead, out.Kind)
	require.Equal(t, "SELECT 1", out.Query)
	require.NotEmpty(t, out.Token)
	require.Len(t, rec.got, 1, "decide should be metered once")
	require.Equal(t, llm.PhaseDecide, rec.got[0].Phase)
}

func TestSuggest_Write_ReturnsWarningAndToken(t *testing.T) {
	p := &fakeProvider{
		decision: llm.Decision{
			Kind:        llm.KindWrite,
			Explanation: "crea tarea",
			Action:      &llm.ActionCall{Domain: "tasks", Operation: "create_task", Args: json.RawMessage(`{"name":"x"}`)},
		},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5", Phase: llm.PhaseDecide},
	}
	svc, _, _ := newTestService(p)

	out, err := svc.Suggest(context.Background(), SuggestRequest{Text: "crea una tarea x"})
	require.NoError(t, err)
	require.Equal(t, llm.KindWrite, out.Kind)
	require.NotEmpty(t, out.Warning)
	require.NotEmpty(t, out.Token)
}

func TestSuggest_Reject_NoToken(t *testing.T) {
	p := &fakeProvider{decision: llm.Decision{Kind: llm.KindReject, Reject: "no entendí"}}
	svc, _, _ := newTestService(p)

	out, err := svc.Suggest(context.Background(), SuggestRequest{Text: "??"})
	require.NoError(t, err)
	require.Equal(t, llm.KindReject, out.Kind)
	require.Empty(t, out.Token)
	require.Equal(t, "no entendí", out.Explanation)
}

func TestSuggest_FeedbackRound_PassesPrior(t *testing.T) {
	p := &fakeProvider{
		decision:    llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1", Explanation: "uno"},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5"},
	}
	svc, _, _ := newTestService(p)

	first, err := svc.Suggest(context.Background(), SuggestRequest{Text: "consulta"})
	require.NoError(t, err)
	require.NotEmpty(t, first.Token)

	_, err = svc.Suggest(context.Background(), SuggestRequest{Text: "mejor por mes", Token: first.Token})
	require.NoError(t, err)
	require.NotNil(t, p.lastDecide.Prior, "feedback round should pass the prior decision")
	require.Equal(t, "mejor por mes", p.lastDecide.Feedback)
	require.Equal(t, "SELECT 1", p.lastDecide.Prior.SQL)
}

func TestExecute_Write_DispatchesToRegistry(t *testing.T) {
	p := &fakeProvider{
		decision: llm.Decision{
			Kind:        llm.KindWrite,
			Explanation: "crea tarea",
			Action:      &llm.ActionCall{Domain: "tasks", Operation: "create_task", Args: json.RawMessage(`{"name":"Comprar pan"}`)},
		},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5"},
	}
	svc, rec, tw := newTestService(p)

	sug, err := svc.Suggest(context.Background(), SuggestRequest{Text: "crea tarea comprar pan"})
	require.NoError(t, err)

	out, err := svc.Execute(context.Background(), ExecuteRequest{Token: sug.Token})
	require.NoError(t, err)
	require.Equal(t, llm.KindWrite, out.Kind)
	require.Equal(t, "Comprar pan", tw.last.Name)
	require.Contains(t, out.Summary, "Comprar pan")
	// Only the decide call is metered (writes don't summarize).
	require.Len(t, rec.got, 1)
}

func TestExecute_BadToken(t *testing.T) {
	svc, _, _ := newTestService(&fakeProvider{})
	_, err := svc.Execute(context.Background(), ExecuteRequest{Token: "garbage.sig"})
	require.ErrorIs(t, err, ErrBadToken)
}

func TestMonthlyUsage_SetsMonthAndCurrency(t *testing.T) {
	svc, _, _ := newTestService(&fakeProvider{})
	mu, err := svc.MonthlyUsage(context.Background(), time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "2026-07", mu.Month)
	require.Equal(t, "USD", mu.Currency)
}
