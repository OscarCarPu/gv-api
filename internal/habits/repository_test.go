package habits

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/sqlc"
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
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d results, want 2", len(got))
		}
		if got[0].ID != 1 || got[0].Name != "Exercise" {
			t.Errorf("got %#v, want ID=1 Name=Exercise", got[0])
		}
		if got[0].LogValue == nil {
			t.Fatalf("got nil LogValue, want 42.5")
		}
		if *got[0].LogValue != 42.5 {
			t.Errorf("got LogValue %f, want 42.5", *got[0].LogValue)
		}
		if got[1].LogValue != nil {
			t.Errorf("got LogValue %v, want nil", got[1].LogValue)
		}
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getHabitsWithLogsFn: func(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetHabitsWithLogs(context.Background(), time.Now())
		if err == nil {
			t.Fatal("got nil, want error")
		}
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
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got.HabitID != 5 {
			t.Errorf("got HabitID %d, want 5", got.HabitID)
		}
		if !got.LogDate.Equal(wantDate) {
			t.Errorf("got date %v, want %v", got.LogDate, wantDate)
		}
		if got.Value != 100.0 {
			t.Errorf("got value %f, want 100.0", got.Value)
		}
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			upsertLogFn: func(ctx context.Context, arg sqlc.UpsertLogParams) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		err := repo.UpsertLog(context.Background(), 1, time.Now(), 10.0)
		if err == nil {
			t.Fatal("got nil, want error")
		}
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
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if got.ID != 7 {
			t.Errorf("got ID %d, want 7", got.ID)
		}
		if got.Name != "Meditation" {
			t.Errorf("got name %q, want %q", got.Name, "Meditation")
		}
		if got.Description == nil || *got.Description != desc {
			t.Errorf("got description %v, want %q", got.Description, desc)
		}
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createHabitFn: func(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
				return sqlc.Habit{}, errors.New("unique violation")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateHabit(context.Background(), "Exercise", nil)
		if err == nil {
			t.Fatal("got nil, want error")
		}
	})
}
