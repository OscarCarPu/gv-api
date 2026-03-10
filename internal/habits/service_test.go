package habits

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	getHabitsFn   func(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	upsertLogFn   func(ctx context.Context, habitID int32, date time.Time, value float32) error
	createHabitFn func(ctx context.Context, name string, description *string) (CreateHabitResponse, error)
	deleteHabitFn func(ctx context.Context, id int32) error
}

func (m *mockRepo) GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
	if m.getHabitsFn != nil {
		return m.getHabitsFn(ctx, date)
	}
	return nil, nil
}

func (m *mockRepo) UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error {
	if m.upsertLogFn != nil {
		return m.upsertLogFn(ctx, habitID, date, value)
	}
	return nil
}

func (m *mockRepo) CreateHabit(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
	if m.createHabitFn != nil {
		return m.createHabitFn(ctx, name, description)
	}
	return CreateHabitResponse{}, nil
}

func (m *mockRepo) DeleteHabit(ctx context.Context, id int32) error {
	if m.deleteHabitFn != nil {
		return m.deleteHabitFn(ctx, id)
	}
	return nil
}

func TestService_GetDailyView(t *testing.T) {
	t.Run("parses date correctly", func(t *testing.T) {
		want := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, got time.Time) ([]HabitWithLog, error) {
				assert.True(t, got.Equal(want))
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo, nil)

		_, err := svc.GetDailyView(context.Background(), "2025-01-31")
		require.NoError(t, err)
	})

	t.Run("returns error for invalid date", func(t *testing.T) {
		svc := NewService(&mockRepo{}, nil)

		_, err := svc.GetDailyView(context.Background(), "invalid-date")
		assert.Error(t, err)
	})

	t.Run("returns empty results", func(t *testing.T) {
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo, nil)

		got, err := svc.GetDailyView(context.Background(), "2025-01-31")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("uses today when date is empty", func(t *testing.T) {
		var got time.Time
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
				got = date
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo, nil)

		_, err := svc.GetDailyView(context.Background(), "")
		require.NoError(t, err)

		now := time.Now().UTC()
		want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		assert.True(t, got.Equal(want))
	})
}

func TestService_LogHabit(t *testing.T) {
	t.Run("delegates to repository", func(t *testing.T) {
		var gotHabitID int32
		var gotDate time.Time
		var gotValue float32

		mock := &mockRepo{
			upsertLogFn: func(ctx context.Context, habitID int32, date time.Time, value float32) error {
				gotHabitID = habitID
				gotDate = date
				gotValue = value
				return nil
			},
		}
		svc := NewService(mock, nil)

		req := LogUpsertRequest{HabitID: 5, Date: "2025-01-31", Value: 100.0}
		err := svc.LogHabit(context.Background(), req)
		require.NoError(t, err)

		wantDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, int32(5), gotHabitID)
		assert.True(t, gotDate.Equal(wantDate))
		assert.Equal(t, float32(100.0), gotValue)
	})

	t.Run("returns error for invalid date", func(t *testing.T) {
		svc := NewService(&mockRepo{}, nil)

		req := LogUpsertRequest{HabitID: 1, Date: "not-a-date", Value: 10.0}
		err := svc.LogHabit(context.Background(), req)
		assert.Error(t, err)
	})
}

func TestService_CreateHabit(t *testing.T) {
	t.Run("delegates to repository", func(t *testing.T) {
		var gotName string
		var gotDesc *string

		desc := "test description"
		mock := &mockRepo{
			createHabitFn: func(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
				gotName = name
				gotDesc = description
				return CreateHabitResponse{ID: 1, Name: name, Description: description}, nil
			},
		}
		svc := NewService(mock, nil)

		req := CreateHabitRequest{Name: "Exercise", Description: &desc}
		got, err := svc.CreateHabit(context.Background(), req)
		require.NoError(t, err)

		assert.Equal(t, "Exercise", gotName)
		assert.Equal(t, &desc, gotDesc)
		assert.Equal(t, int32(1), got.ID)
	})

	t.Run("handles nil description", func(t *testing.T) {
		var gotDesc *string

		mock := &mockRepo{
			createHabitFn: func(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
				gotDesc = description
				return CreateHabitResponse{ID: 1, Name: name}, nil
			},
		}
		svc := NewService(mock, nil)

		req := CreateHabitRequest{Name: "Exercise", Description: nil}
		_, err := svc.CreateHabit(context.Background(), req)
		require.NoError(t, err)
		assert.Nil(t, gotDesc)
	})
}

func TestService_DeleteHabit(t *testing.T) {
	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			deleteHabitFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(5), id)
				return nil
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteHabit(context.Background(), 5)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			deleteHabitFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteHabit(context.Background(), 1)
		require.Error(t, err)
	})
}
