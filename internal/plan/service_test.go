package plan_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gv-api/internal/plan"
	"gv-api/internal/plan/mocks"
	"gv-api/internal/tasks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type stubTasksSummary struct {
	resp tasks.TimeEntrySummaryResponse
	err  error
}

func (s stubTasksSummary) GetTimeEntrySummary(_ context.Context) (tasks.TimeEntrySummaryResponse, error) {
	return s.resp, s.err
}

func ptr[T any](v T) *T { return &v }

func TestService_Create(t *testing.T) {
	t.Run("rejects ended_at <= started_at", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		start := time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC)
		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: start,
			EndedAt:   start,
			Label:     ptr("foo"),
		})
		assert.ErrorIs(t, err, plan.ErrInvalidTimeRange)
	})

	t.Run("requires label or task_id", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		})
		assert.ErrorIs(t, err, plan.ErrLabelRequired)
	})

	t.Run("rejects label > 200 chars", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			Label:     ptr(strings.Repeat("a", 201)),
		})
		assert.ErrorIs(t, err, plan.ErrLabelTooLong)
	})

	t.Run("free-time block with explicit label", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			HasOverlap(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*int32)(nil)).
			Return(false, nil)
		repo.EXPECT().
			Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*int32)(nil), "comer", (*string)(nil)).
			Return(plan.PlanBlockResponse{ID: 1, Label: "comer"}, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		got, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC),
			Label:     ptr("  comer  "), // gets trimmed
		})
		require.NoError(t, err)
		assert.Equal(t, "comer", got.Label)
	})

	t.Run("rejects overlap on create", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			HasOverlap(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*int32)(nil)).
			Return(true, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC),
			Label:     ptr("comer"),
		})
		assert.ErrorIs(t, err, plan.ErrOverlap)
	})

	t.Run("linked block falls back to task name when label omitted", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTaskName(mock.Anything, int32(42)).
			Return("Refactor agenda", nil)
		repo.EXPECT().
			HasOverlap(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*int32)(nil)).
			Return(false, nil)
		repo.EXPECT().
			Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything, ptr(int32(42)), "Refactor agenda", (*string)(nil)).
			Return(plan.PlanBlockResponse{ID: 2, TaskID: ptr(int32(42)), Label: "Refactor agenda"}, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		got, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			TaskID:    ptr(int32(42)),
		})
		require.NoError(t, err)
		assert.Equal(t, "Refactor agenda", got.Label)
	})

	t.Run("linked block keeps caller-supplied label override", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		// GetTaskName must NOT be called because the caller provided a label.
		repo.EXPECT().
			HasOverlap(mock.Anything, mock.Anything, mock.Anything, mock.Anything, (*int32)(nil)).
			Return(false, nil)
		repo.EXPECT().
			Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything, ptr(int32(42)), "Sprint planning", (*string)(nil)).
			Return(plan.PlanBlockResponse{ID: 3, TaskID: ptr(int32(42)), Label: "Sprint planning"}, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			TaskID:    ptr(int32(42)),
			Label:     ptr("Sprint planning"),
		})
		require.NoError(t, err)
	})

	t.Run("propagates ErrTaskNotFound from GetTaskName", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			GetTaskName(mock.Anything, int32(999)).
			Return("", plan.ErrTaskNotFound)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		_, err := svc.Create(context.Background(), plan.CreatePlanBlockRequest{
			StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
			EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			TaskID:    ptr(int32(999)),
		})
		assert.ErrorIs(t, err, plan.ErrTaskNotFound)
	})
}

func TestService_Update(t *testing.T) {
	t.Run("rejects ended_at <= started_at when both provided", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		start := time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC)
		end := start.Add(-time.Hour)
		_, err := svc.Update(context.Background(), plan.UpdatePlanBlockRequest{
			ID:        1,
			StartedAt: &start,
			EndedAt:   &end,
		})
		assert.ErrorIs(t, err, plan.ErrInvalidTimeRange)
	})

	t.Run("checks against persisted side when only one bound provided", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			Get(mock.Anything, int32(1)).
			Return(plan.PlanBlockResponse{
				ID:        1,
				StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
				EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
			}, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		// Push started_at forward past ended_at — must fail.
		newStart := time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC)
		_, err := svc.Update(context.Background(), plan.UpdatePlanBlockRequest{
			ID:        1,
			StartedAt: &newStart,
		})
		assert.ErrorIs(t, err, plan.ErrInvalidTimeRange)
	})

	t.Run("trims label and rejects empty", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		empty := "   "
		_, err := svc.Update(context.Background(), plan.UpdatePlanBlockRequest{
			ID:    1,
			Label: &empty,
		})
		assert.ErrorIs(t, err, plan.ErrLabelRequired)
	})

	t.Run("rejects overlap on update, excluding self", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().
			HasOverlap(mock.Anything, mock.Anything, mock.Anything, mock.Anything, ptr(int32(7))).
			Return(true, nil)
		svc := plan.NewService(repo, stubTasksSummary{}, time.UTC)

		start := time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC)
		end := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
		_, err := svc.Update(context.Background(), plan.UpdatePlanBlockRequest{
			ID:        7,
			StartedAt: &start,
			EndedAt:   &end,
		})
		assert.ErrorIs(t, err, plan.ErrOverlap)
	})
}

func TestService_GetToday(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		ListByDate(mock.Anything, mock.Anything).
		Return([]plan.PlanBlockResponse{
			// 1h linked
			{
				ID:        1,
				StartedAt: time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC),
				EndedAt:   time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
				TaskID:    ptr(int32(42)),
				Label:     "Refactor",
			},
			// 2h free
			{
				ID:        2,
				StartedAt: time.Date(2026, 5, 10, 13, 0, 0, 0, time.UTC),
				EndedAt:   time.Date(2026, 5, 10, 15, 0, 0, 0, time.UTC),
				Label:     "thing 1",
			},
			// 30m linked
			{
				ID:        3,
				StartedAt: time.Date(2026, 5, 10, 16, 0, 0, 0, time.UTC),
				EndedAt:   time.Date(2026, 5, 10, 16, 30, 0, 0, time.UTC),
				TaskID:    ptr(int32(43)),
				Label:     "Email",
			},
		}, nil)

	svc := plan.NewService(repo, stubTasksSummary{
		resp: tasks.TimeEntrySummaryResponse{
			Today:               7200,
			Week:                100000,
			DailyTargetSeconds:  30000,
			WeeklyTargetSeconds: tasks.WeeklyTaskTargetSeconds,
		},
	}, time.UTC)

	resp, err := svc.GetToday(context.Background())
	require.NoError(t, err)
	assert.Len(t, resp.Blocks, 3)
	// 1h + 30m of task time = 5400s ; 2h free = 7200s
	assert.Equal(t, int64(5400), resp.Totals.TaskSeconds)
	assert.Equal(t, int64(7200), resp.Totals.FreeSeconds)
	assert.Equal(t, int64(100000), resp.Budget.Week)
	assert.Equal(t, int64(30000), resp.Budget.DailyTargetSeconds)
}

func TestService_GetToday_PropagatesBudgetError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().
		ListByDate(mock.Anything, mock.Anything).
		Return([]plan.PlanBlockResponse{}, nil)

	boom := errors.New("boom")
	svc := plan.NewService(repo, stubTasksSummary{err: boom}, time.UTC)

	_, err := svc.GetToday(context.Background())
	assert.ErrorIs(t, err, boom)
}
