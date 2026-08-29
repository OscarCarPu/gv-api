package plan_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/plan"
	"gv-api/internal/tasks"
	testutil "gv-api/internal/testutil"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decimalFromFloat(f float64) decimal.Decimal { return decimal.NewFromFloat(f) }

// newPlanRepo deliberately does NOT truncate "tasks": that table is shared with
// internal/tasks_test's own integration tests, and go test runs different packages
// concurrently by default — two packages truncating the same table races for real (this is
// exactly how TestIntegration_EnsureRecurringBlocks_GeneratesAndRespectsSkips and
// TestIntegration_GetByEventRef_UniqueConstraint were first seen failing, intermittently, with
// a task_id foreign key violation: a still-fresh task got wiped mid-test by the other
// package's truncate). Not resetting the tasks table's identity sequence is harmless here —
// every test only ever references the task it just created, by the id CreateTask returns.
func newPlanRepo(t *testing.T) (*plan.PostgresRepository, *tasks.PostgresRepository) {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "recurring_commitment_skips", "recurring_commitments", "plan_blocks")
	return plan.NewRepository(pool), tasks.NewRepository(pool)
}

func mustCreateTask(t *testing.T, repo *tasks.PostgresRepository, name string) int32 {
	t.Helper()
	task, err := repo.CreateTask(context.Background(), nil, name, nil, nil, "standard", nil, 3, nil)
	require.NoError(t, err)
	return task.ID
}

func madrid(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	return loc
}

func TestIntegration_SumBusyHoursByDate_CountsTaskOrEventLinkedBlocksOnly(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Refactor")

	day := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)

	// 90 minutes, linked to a task — counts.
	_, err := repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day, StartedAt: day.Add(9 * time.Hour), EndedAt: day.Add(10*time.Hour + 30*time.Minute),
		TaskID: &taskID, Label: "Refactor",
	})
	require.NoError(t, err)

	// 60 minutes, linked to an event only — counts.
	ref := "42"
	_, err = repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day, StartedAt: day.Add(14 * time.Hour), EndedAt: day.Add(15 * time.Hour),
		Label: "Festival", EventRef: &ref,
	})
	require.NoError(t, err)

	// 30 minutes, plain label only — must NOT count.
	_, err = repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day, StartedAt: day.Add(18 * time.Hour), EndedAt: day.Add(18*time.Hour + 30*time.Minute),
		Label: "Free time",
	})
	require.NoError(t, err)

	busy, err := repo.SumBusyHoursByDate(ctx, day, day.AddDate(0, 0, 1), loc.String())
	require.NoError(t, err)
	require.Contains(t, busy, "2026-09-01")
	assert.True(t, busy["2026-09-01"].Equal(decimalFromFloat(2.5)), "got %s", busy["2026-09-01"])
}

func TestIntegration_SumBusyHoursByDate_SplitsMultiDayBlockAcrossDays(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Festival trip")

	day1 := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)
	started := day1.Add(18 * time.Hour)               // day1 18:00
	ended := day1.AddDate(0, 0, 2).Add(8 * time.Hour) // day3 08:00

	_, err := repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day1, StartedAt: started, EndedAt: ended, TaskID: &taskID, Label: "Trip",
	})
	require.NoError(t, err)

	busy, err := repo.SumBusyHoursByDate(ctx, day1, day1.AddDate(0, 0, 4), loc.String())
	require.NoError(t, err)

	assert.True(t, busy["2026-09-01"].Equal(decimalFromFloat(6)), "day1: got %s", busy["2026-09-01"])
	assert.True(t, busy["2026-09-02"].Equal(decimalFromFloat(24)), "day2: got %s", busy["2026-09-02"])
	assert.True(t, busy["2026-09-03"].Equal(decimalFromFloat(8)), "day3: got %s", busy["2026-09-03"])
	_, hasDay4 := busy["2026-09-04"]
	assert.False(t, hasDay4, "day4 should not be touched by this block")
}

func TestIntegration_HasOverlap_CatchesConflictOnSecondDayOfMultiDayBlock(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Festival trip")

	day1 := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)
	_, err := repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day1, StartedAt: day1.Add(20 * time.Hour), EndedAt: day1.AddDate(0, 0, 2).Add(10 * time.Hour),
		TaskID: &taskID, Label: "Trip",
	})
	require.NoError(t, err)

	day2 := day1.AddDate(0, 0, 1)
	overlap, err := repo.HasOverlap(ctx, day2.Add(9*time.Hour), day2.Add(11*time.Hour), nil)
	require.NoError(t, err)
	assert.True(t, overlap, "a plan on day 2 inside the multi-day block's span must be caught")
}

func TestIntegration_SumPlannedHoursByTask_ExcludesPastBlocks(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Write report")

	now := time.Now().In(loc)
	past := now.AddDate(0, 0, -2)
	future := now.AddDate(0, 0, 2)

	_, err := repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: past, StartedAt: past, EndedAt: past.Add(2 * time.Hour), TaskID: &taskID, Label: "old",
	})
	require.NoError(t, err)
	_, err = repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: future, StartedAt: future, EndedAt: future.Add(3 * time.Hour), TaskID: &taskID, Label: "upcoming",
	})
	require.NoError(t, err)

	planned, err := repo.SumPlannedHoursByTask(ctx, []int32{taskID}, now)
	require.NoError(t, err)
	assert.True(t, planned[taskID].Equal(decimalFromFloat(3)), "only the future block should count, got %s", planned[taskID])
}

func TestIntegration_EnsureRecurringBlocks_GeneratesAndRespectsSkips(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Work")

	svc := plan.NewService(repo, stubTasksSummary{}, loc)

	commitment, err := svc.CreateCommitment(ctx, plan.CreateCommitmentRequest{
		TaskID: taskID, Label: "Work", DaysOfWeek: []int32{1, 2, 3, 4, 5}, // Mon-Fri
		StartTime: "09:00", EndTime: "17:00",
	})
	require.NoError(t, err)

	// 2026-09-07 is a Monday.
	from := time.Date(2026, 9, 7, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 7) // one full week: Mon-Sun

	require.NoError(t, svc.EnsureRecurringBlocks(ctx, from, to))
	blocks, err := repo.ListByDateRange(ctx, from, to)
	require.NoError(t, err)
	assert.Len(t, blocks, 5, "Mon-Fri only, weekend excluded")

	// Delete one (Wednesday) — Service.Delete should register a skip.
	var wedID int32
	for _, b := range blocks {
		if b.PlanDate.Weekday() == time.Wednesday {
			wedID = b.ID
		}
	}
	require.NotZero(t, wedID)
	require.NoError(t, svc.Delete(ctx, wedID))

	require.NoError(t, svc.EnsureRecurringBlocks(ctx, from, to))
	blocksAfter, err := repo.ListByDateRange(ctx, from, to)
	require.NoError(t, err)
	assert.Len(t, blocksAfter, 4, "the skipped Wednesday must not come back")

	_ = commitment
}

func TestIntegration_GetByEventRef_UniqueConstraint(t *testing.T) {
	repo, taskRepo := newPlanRepo(t)
	ctx := context.Background()
	loc := madrid(t)
	taskID := mustCreateTask(t, taskRepo, "Task")
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, loc)
	ref := "99"

	_, err := repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day, StartedAt: day.Add(9 * time.Hour), EndedAt: day.Add(10 * time.Hour),
		TaskID: &taskID, Label: "a", EventRef: &ref,
	})
	require.NoError(t, err)

	_, err = repo.Create(ctx, plan.CreatePlanBlockParams{
		PlanDate: day, StartedAt: day.Add(11 * time.Hour), EndedAt: day.Add(12 * time.Hour),
		TaskID: &taskID, Label: "b", EventRef: &ref,
	})
	assert.Error(t, err, "a second block with the same event_ref must be rejected")
}
