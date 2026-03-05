package habits

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuerier struct {
	getHabitsWithLogsFn func(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error)
	upsertLogFn         func(ctx context.Context, arg sqlc.UpsertLogParams) error
	createHabitFn       func(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error)
}

func (m *mockQuerier) GetHabitsWithLogs(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
	if m.getHabitsWithLogsFn != nil {
		return m.getHabitsWithLogsFn(ctx, logDate)
	}
	return nil, nil
}

func (m *mockQuerier) UpsertLog(ctx context.Context, arg sqlc.UpsertLogParams) error {
	if m.upsertLogFn != nil {
		return m.upsertLogFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) CreateHabit(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
	if m.createHabitFn != nil {
		return m.createHabitFn(ctx, arg)
	}
	return sqlc.Habit{}, nil
}

func (m *mockQuerier) CreateProject(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
	return sqlc.CreateProjectRow{}, nil
}

func (m *mockQuerier) CreateTask(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error) {
	return sqlc.CreateTaskRow{}, nil
}

func (m *mockQuerier) CreateTodo(ctx context.Context, arg sqlc.CreateTodoParams) (sqlc.CreateTodoRow, error) {
	return sqlc.CreateTodoRow{}, nil
}

func (m *mockQuerier) CreateTimeEntry(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error) {
	return sqlc.TimeEntry{}, nil
}

func (m *mockQuerier) FinishTimeEntry(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error) {
	return sqlc.TimeEntry{}, nil
}

func (m *mockQuerier) FinishTask(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error) {
	return sqlc.Task{}, nil
}

func (m *mockQuerier) FinishProject(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error) {
	return sqlc.Project{}, nil
}

func (m *mockQuerier) GetRootProjects(ctx context.Context) ([]sqlc.GetRootProjectsRow, error) {
	return nil, nil
}

func TestRepository_GetHabitsWithLogs(t *testing.T) {
	t.Run("maps rows to domain types", func(t *testing.T) {
		desc := "Daily workout"
		val := float32(42.5)

		mock := &mockQuerier{
			getHabitsWithLogsFn: func(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
				return []sqlc.GetHabitsWithLogsRow{
					{ID: 1, Name: "Exercise", Description: &desc, Value: &val},
					{ID: 2, Name: "Reading", Description: nil, Value: nil},
				}, nil
			},
		}
		repo := NewRepository(mock)

		date := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		got, err := repo.GetHabitsWithLogs(context.Background(), date)
		require.NoError(t, err)
		require.Len(t, got, 2)

		assert.Equal(t, int32(1), got[0].ID)
		assert.Equal(t, "Exercise", got[0].Name)
		require.NotNil(t, got[0].LogValue)
		assert.Equal(t, float32(42.5), *got[0].LogValue)
		assert.Nil(t, got[1].LogValue)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getHabitsWithLogsFn: func(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetHabitsWithLogs(context.Background(), time.Now())
		assert.Error(t, err)
	})
}

func TestRepository_UpsertLog(t *testing.T) {
	t.Run("passes correct params", func(t *testing.T) {
		var got sqlc.UpsertLogParams

		mock := &mockQuerier{
			upsertLogFn: func(ctx context.Context, arg sqlc.UpsertLogParams) error {
				got = arg
				return nil
			},
		}
		repo := NewRepository(mock)

		wantDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		err := repo.UpsertLog(context.Background(), 5, wantDate, 100.0)
		require.NoError(t, err)

		assert.Equal(t, int32(5), got.HabitID)
		assert.True(t, got.LogDate.Equal(wantDate))
		assert.Equal(t, float32(100.0), got.Value)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			upsertLogFn: func(ctx context.Context, arg sqlc.UpsertLogParams) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		err := repo.UpsertLog(context.Background(), 1, time.Now(), 10.0)
		assert.Error(t, err)
	})
}

func TestRepository_CreateHabit(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "test desc"

		mock := &mockQuerier{
			createHabitFn: func(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
				return sqlc.Habit{ID: 7, Name: arg.Name, Description: arg.Description}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateHabit(context.Background(), "Meditation", &desc)
		require.NoError(t, err)

		assert.Equal(t, int32(7), got.ID)
		assert.Equal(t, "Meditation", got.Name)
		assert.Equal(t, &desc, got.Description)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createHabitFn: func(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
				return sqlc.Habit{}, errors.New("unique violation")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateHabit(context.Background(), "Exercise", nil)
		assert.Error(t, err)
	})
}
