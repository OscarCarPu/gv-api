package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuerier struct {
	createProjectFn func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error)
}

func (m *mockQuerier) CreateProject(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, arg)
	}
	return sqlc.CreateProjectRow{}, nil
}

func (m *mockQuerier) CreateHabit(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
	return sqlc.Habit{}, nil
}

func (m *mockQuerier) GetHabitsWithLogs(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
	return nil, nil
}

func (m *mockQuerier) UpsertLog(ctx context.Context, arg sqlc.UpsertLogParams) error {
	return nil
}

func TestRepository_CreateProject(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "test desc"
		parentID := int32(2)
		dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{
					ID:          1,
					Name:        arg.Name,
					Description: arg.Description,
					DueAt:       dueDate,
					ParentID:    arg.ParentID,
				}, nil
			},
		}
		repo := NewRepository(mock)

		dueAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		got, err := repo.CreateProject(context.Background(), "test", &desc, &dueAt, &parentID)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "test", got.Name)
		assert.Equal(t, &desc, got.Description)
		assert.Equal(t, &dueAt, got.DueAt)
		assert.Equal(t, &parentID, got.ParentID)
	})

	t.Run("returns nil DueAt when date is invalid", func(t *testing.T) {
		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{
					ID:   2,
					Name: arg.Name,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateProject(context.Background(), "no date", nil, nil, nil)
		require.NoError(t, err)
		assert.Nil(t, got.DueAt)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateProject(context.Background(), "fail", nil, nil, nil)
		assert.Error(t, err)
	})
}
