package llm

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGeminiProvider_Live exercises the real Gemini API. It is skipped unless
// GEMINI_API_KEY is set, so it never runs in CI / -short.
func TestGeminiProvider_Live(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	model := os.Getenv("ASSISTANT_MODEL")
	if model == "" {
		model = "gemini-3.1-flash-lite"
	}
	p := NewGeminiProvider(key, model)
	ctx := context.Background()

	sys := `Eres el asistente de una app personal. Devuelve una decisión estructurada.
Tablas (PostgreSQL): habits(id, name), transactions(id, type, amount, occurred_at).
- kind="read": consulta SELECT de solo lectura en "sql" cuando el usuario quiere ver/contar datos, con needs_summary=true.
- kind="write": action{domain,operation,args} para crear/registrar algo.
- kind="reject": si es ambiguo.
Campos: kind, sql, action, explanation (español), needs_summary, reject.`

	t.Run("decide read", func(t *testing.T) {
		res, err := p.Decide(ctx, DecideInput{SystemPrompt: sys, UserText: "¿cuántos hábitos tengo?"})
		require.NoError(t, err)
		d := res.Decision
		require.Len(t, res.Usages, 1, "no runner: exactly one model call")
		u := res.Usages[0]
		t.Logf("kind=%q sql=%q explanation=%q", d.Kind, d.SQL, d.Explanation)
		t.Logf("usage: in=%d out=%d cacheRead=%d model=%s", u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.Model)
		require.NotEmpty(t, d.Kind)
		require.Greater(t, u.InputTokens, int64(0))
		require.Greater(t, u.OutputTokens, int64(0))
		require.Equal(t, PhaseDecide, u.Phase)
	})

	t.Run("decide with exploration", func(t *testing.T) {
		runner := &recordingRunner{result: QueryResult{
			Columns: []string{"id", "name"},
			Rows:    [][]any{{1, "gimnasio"}, {2, "leer"}},
		}}
		res, err := p.Decide(ctx, DecideInput{
			SystemPrompt: sys,
			UserText:     "registra que hoy hice el hábito de ir al gimnasio",
			Runner:       runner,
			MaxQueries:   3,
		})
		require.NoError(t, err)
		t.Logf("internal queries: %v", runner.queries)
		t.Logf("decision: kind=%q action=%+v explanation=%q", res.Decision.Kind, res.Decision.Action, res.Decision.Explanation)

		// Exploring is the model's choice, so don't require it — but the loop must
		// terminate with a decision, respect the budget, and meter every round
		// with only the last one as "decide".
		require.NotEmpty(t, res.Decision.Kind)
		require.LessOrEqual(t, len(runner.queries), 3)
		require.NotEmpty(t, res.Usages)
		for i, u := range res.Usages {
			want := PhaseExplore
			if i == len(res.Usages)-1 {
				want = PhaseDecide
			}
			require.Equal(t, want, u.Phase, "usage %d", i)
		}
	})

	t.Run("summarize", func(t *testing.T) {
		out, u, err := p.Summarize(ctx, SummarizeInput{
			UserText: "¿cuántos hábitos tengo?",
			SQL:      "SELECT count(*) AS total FROM habits",
			Columns:  []string{"total"},
			Rows:     [][]any{{7}},
		})
		require.NoError(t, err)
		t.Logf("summary=%q", out)
		t.Logf("usage: in=%d out=%d", u.InputTokens, u.OutputTokens)
		require.NotEmpty(t, out)
		require.Equal(t, PhaseSummarize, u.Phase)
	})
}
