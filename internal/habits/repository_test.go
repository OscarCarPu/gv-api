package habits_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/habits"
	testutil "gv-api/internal/testutils"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) (*habits.PostgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "habit_logs", "habits")
	return habits.NewRepository(habitsdb.New(pool)), pool
}

// readStreaks returns (current_streak, longest_streak) for the habit row.
func readStreaks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int32) (int32, int32) {
	t.Helper()
	var cs, ls int32
	err := pool.QueryRow(ctx, "SELECT current_streak, longest_streak FROM habits WHERE id = $1", id).Scan(&cs, &ls)
	require.NoError(t, err)
	return cs, ls
}

func iso(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// --- streak SQL function: behavior parity with the former Go logic ---

func TestIntegration_RecalculateStreak_DailyChain(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, true)
	require.NoError(t, err)

	for _, d := range []string{"2026-03-13", "2026-03-15", "2026-03-16", "2026-03-17"} {
		require.NoError(t, repo.UpsertLog(ctx, h.ID, iso(d), 1))
	}
	// today=2026-03-17, last 3 days are met; 2026-03-13 met but isolated → longest=3.
	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(3), cs)
	require.Equal(t, int32(3), ls)
}

func TestIntegration_RecalculateStreak_CurrentPeriodNotMet_Skips(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, true)
	require.NoError(t, err)

	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-15"), 1.5))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-16"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-17"), 0.5))

	// Today (03-17) sum=0.5 < target. Should skip current and count 03-16, 03-15 → streak=2.
	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(2), cs)
	require.Equal(t, int32(2), ls)
}

func TestIntegration_RecalculateStreak_BrokenStreak_ResetsToZero(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, true)
	require.NoError(t, err)

	// Today not met, yesterday missing, day-2 met → streak=0, longest=1
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-15"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-17"), 0.5))

	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(0), cs)
	require.Equal(t, int32(1), ls)
}

func TestIntegration_RecalculateStreak_WeeklyTarget(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "weekly", ptr(float32(3)), nil, true)
	require.NoError(t, err)

	// Week of 2026-03-16 (Mon-Sun): 2 sessions, not yet meeting 3
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-16"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-18"), 1))
	// Week of 2026-03-09: 4 sessions
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-09"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-10"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-12"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-14"), 1))
	// Week of 2026-03-02: 3 sessions
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-02"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-04"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-06"), 1))

	// today=2026-03-17 (a Tue, current week not met) → skip current, last two weeks met → streak=2
	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(2), cs)
	require.Equal(t, int32(2), ls)
}

func TestIntegration_RecalculateStreak_RangeTarget(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(60)), ptr(float32(80)), true)
	require.NoError(t, err)

	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-15"), 90)) // out of range
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-16"), 65))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-17"), 70))

	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(2), cs)
	require.Equal(t, int32(2), ls)
}

func TestIntegration_RecalculateStreak_RecordingNotRequired_CarryForward(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, false)
	require.NoError(t, err)

	// Logs day-3=1, today=1; days -1,-2 missing — carry-forward fills with 1 → streak=4
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-14"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-17"), 1))

	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(4), cs)
	require.Equal(t, int32(4), ls)
}

func TestIntegration_RecalculateStreak_RecordingNotRequired_BreakOnZero(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, false)
	require.NoError(t, err)

	// day-3=1, day-2=1, day-1=0 (recorded), today=missing → carries forward 0
	// Today and day-1 not met → streak=0; longest run [day-3, day-2] = 2
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-14"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-15"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-16"), 0))

	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(0), cs)
	require.Equal(t, int32(2), ls)
}

func TestIntegration_RecalculateStreak_LongestStreakReplaces(t *testing.T) {
	ctx := context.Background()
	repo, pool := newRepo(t)

	h, err := repo.CreateHabit(ctx, "h", nil, "daily", ptr(float32(1)), nil, true)
	require.NoError(t, err)

	// Seed a longest_streak directly that the recompute should overwrite.
	_, err = pool.Exec(ctx, "UPDATE habits SET longest_streak = 99 WHERE id = $1", h.ID)
	require.NoError(t, err)

	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-15"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-16"), 1))
	require.NoError(t, repo.UpsertLog(ctx, h.ID, iso("2026-03-17"), 1))

	require.NoError(t, repo.RecalculateStreak(ctx, h.ID, iso("2026-03-17")))
	cs, ls := readStreaks(t, ctx, pool, h.ID)
	require.Equal(t, int32(3), cs)
	require.Equal(t, int32(3), ls, "longest_streak is replaced with the recomputed value, not max(prev, new)")
}

func TestIntegration_GetHabitHistory_FillZeros(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "habit_logs", "habits")
	repo := habits.NewRepository(habitsdb.New(pool))

	h, err := repo.CreateHabit(ctx, "daily", nil, "daily", nil, nil, true)
	require.NoError(t, err)

	mustLog := func(iso string, val float64) {
		t.Helper()
		d, err := time.Parse("2006-01-02", iso)
		require.NoError(t, err)
		require.NoError(t, repo.UpsertLog(ctx, h.ID, d, float32(val)))
	}

	mustLog("2026-03-01", 5)
	mustLog("2026-03-03", 3)

	start, _ := time.Parse("2026-01-02", "2026-01-02")
	start, _ = time.Parse("2006-01-02", "2026-03-01")
	end, _ := time.Parse("2006-01-02", "2026-03-04")

	t.Run("fill_zeros=true returns all 4 days", func(t *testing.T) {
		got, err := repo.GetHabitHistory(ctx, h.ID, "day", start, end, true)
		require.NoError(t, err)
		require.Len(t, got, 4)
		require.Equal(t, "2026-03-01", got[0].Date)
		require.Equal(t, float32(5), got[0].Value)
		require.Equal(t, "2026-03-02", got[1].Date)
		require.Equal(t, float32(0), got[1].Value)
		require.Equal(t, float32(3), got[2].Value)
		require.Equal(t, float32(0), got[3].Value)
	})

	t.Run("fill_zeros=false returns only days with logs", func(t *testing.T) {
		got, err := repo.GetHabitHistory(ctx, h.ID, "day", start, end, false)
		require.NoError(t, err)
		require.Len(t, got, 2)
	})
}

