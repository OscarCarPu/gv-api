package habits_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/habits"
	testutil "gv-api/internal/testutils"

	"github.com/stretchr/testify/require"
)

func TestIntegration_HabitLogs_AcrossPeriodBoundary(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "habit_logs", "habits")
	repo := habits.NewRepository(habitsdb.New(pool))

	h, err := repo.CreateHabit(ctx, "weekly", nil, "weekly", nil, nil, true)
	require.NoError(t, err)

	mustLog := func(iso string, val float64) {
		t.Helper()
		d, err := time.Parse("2006-01-02", iso)
		require.NoError(t, err)
		require.NoError(t, repo.UpsertLog(ctx, h.ID, d, float32(val)))
	}

	mustLog("2026-04-26", 1.0)
	mustLog("2026-04-27", 2.0)

	groups, err := repo.GetHabitLogs(ctx, h.ID)
	require.NoError(t, err)
	require.Len(t, groups, 2)
}
