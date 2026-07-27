package assistant

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/assistant/llm"
	"gv-api/internal/testutil"

	"github.com/stretchr/testify/require"
)

// TestSuggest_Integration_ExploresAgainstDatabase proves the auto-approved
// internal reads really run: the model's exploratory query hits Postgres through
// the same read-only executor as approved queries, and its rows come back to the
// model (not just a row count).
func TestSuggest_Integration_ExploresAgainstDatabase(t *testing.T) {
	pool := testutil.NewPool(t)

	p := &fakeProvider{
		decision:    llm.Decision{Kind: llm.KindRead, SQL: "SELECT count(*) FROM habits", Explanation: "cuenta hábitos"},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5", Phase: llm.PhaseDecide},
		explore:     []string{"SELECT 7 AS n, 'gym' AS name"},
	}
	reg := NewActionRegistry(&fakeTaskService{}, &fakeHabitService{}, &fakeFinanceService{}, &fakePlanService{}, &fakeRutasService{}, &fakeVarietyService{})
	svc := NewService(p, NewReadExecutor(pool, 3000, 200, 40), reg, &capturingRecorder{}, "secret", time.Minute, 3, haikuPrices(), time.UTC)

	out, err := svc.Suggest(context.Background(), SuggestRequest{Text: "cuántos hábitos hay"})
	require.NoError(t, err)

	require.Len(t, p.exploreSeen, 1)
	require.Empty(t, p.exploreSeen[0].Err)
	require.Equal(t, []string{"n", "name"}, p.exploreSeen[0].Columns)
	require.Len(t, p.exploreSeen[0].Rows, 1)

	require.Len(t, out.Steps, 1)
	require.Equal(t, 1, out.Steps[0].RowCount)
	require.Empty(t, out.Steps[0].Error)
}

// TestSuggest_Integration_ExploreCannotWrite pins the safety boundary: an
// internal query that tries to modify data is rejected before it reaches the
// database, and the failure is reported to the model instead of being executed.
func TestSuggest_Integration_ExploreCannotWrite(t *testing.T) {
	pool := testutil.NewPool(t)

	p := &fakeProvider{
		decision:    llm.Decision{Kind: llm.KindRead, SQL: "SELECT 1", Explanation: "uno"},
		decideUsage: llm.Usage{Model: "claude-haiku-4-5", Phase: llm.PhaseDecide},
		explore: []string{
			"UPDATE habits SET name = 'x'",
			"SELECT 1; DROP TABLE habits",
		},
	}
	reg := NewActionRegistry(&fakeTaskService{}, &fakeHabitService{}, &fakeFinanceService{}, &fakePlanService{}, &fakeRutasService{}, &fakeVarietyService{})
	svc := NewService(p, NewReadExecutor(pool, 3000, 200, 40), reg, &capturingRecorder{}, "secret", time.Minute, 3, haikuPrices(), time.UTC)

	_, err := svc.Suggest(context.Background(), SuggestRequest{Text: "haz algo raro"})
	require.NoError(t, err)
	require.Len(t, p.exploreSeen, 2)
	require.Contains(t, p.exploreSeen[0].Err, "SELECT")
	require.Contains(t, p.exploreSeen[1].Err, "multiple statements")

	var exists bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables
		   WHERE table_schema = 'public' AND table_name = 'habits')`).Scan(&exists))
	require.True(t, exists, "habits table must still exist")
}
