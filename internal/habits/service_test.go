package habits

import (
	"context"
	"testing"
	"time"
)

type mockRepo struct {
	getHabitsFn   func(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	upsertLogFn   func(ctx context.Context, habitID int32, date time.Time, value float32) error
	createHabitFn func(ctx context.Context, name string, description *string) (CreateHabitResponse, error)
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

func TestService_GetDailyView(t *testing.T) {
	t.Run("parses date correctly", func(t *testing.T) {
		want := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, got time.Time) ([]HabitWithLog, error) {
				if !got.Equal(want) {
					t.Errorf("got date %v, want %v", got, want)
				}
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo)

		_, err := svc.GetDailyView(context.Background(), "2025-01-31")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
	})

	t.Run("returns error for invalid date", func(t *testing.T) {
		svc := NewService(&mockRepo{})

		_, err := svc.GetDailyView(context.Background(), "invalid-date")
		if err == nil {
			t.Fatal("got nil, want error")
		}
	})

	t.Run("returns empty results", func(t *testing.T) {
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo)

		got, err := svc.GetDailyView(context.Background(), "2025-01-31")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results, want 0", len(got))
		}
	})

	t.Run("uses today when date is empty", func(t *testing.T) {
		var got time.Time
		repo := &mockRepo{
			getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
				got = date
				return []HabitWithLog{}, nil
			},
		}
		svc := NewService(repo)

		_, err := svc.GetDailyView(context.Background(), "")
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}

		now := time.Now().UTC()
		want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("got date %v, want %v", got, want)
		}
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
		svc := NewService(mock)

		req := LogUpsertRequest{HabitID: 5, Date: "2025-01-31", Value: 100.0}
		err := svc.LogHabit(context.Background(), req)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}

		wantDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
		if gotHabitID != 5 {
			t.Errorf("got habitID %d, want 5", gotHabitID)
		}
		if !gotDate.Equal(wantDate) {
			t.Errorf("got date %v, want %v", gotDate, wantDate)
		}
		if gotValue != 100.0 {
			t.Errorf("got value %f, want 100.0", gotValue)
		}
	})

	t.Run("returns error for invalid date", func(t *testing.T) {
		svc := NewService(&mockRepo{})

		req := LogUpsertRequest{HabitID: 1, Date: "not-a-date", Value: 10.0}
		err := svc.LogHabit(context.Background(), req)
		if err == nil {
			t.Fatal("got nil, want error")
		}
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
		svc := NewService(mock)

		req := CreateHabitRequest{Name: "Exercise", Description: &desc}
		got, err := svc.CreateHabit(context.Background(), req)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if gotName != "Exercise" {
			t.Errorf("got name %q, want %q", gotName, "Exercise")
		}
		if gotDesc == nil || *gotDesc != "test description" {
			t.Errorf("got description %v, want %q", gotDesc, "test description")
		}
		if got.ID != 1 {
			t.Errorf("got ID %d, want 1", got.ID)
		}
	})

	t.Run("handles nil description", func(t *testing.T) {
		var gotDesc *string

		mock := &mockRepo{
			createHabitFn: func(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
				gotDesc = description
				return CreateHabitResponse{ID: 1, Name: name}, nil
			},
		}
		svc := NewService(mock)

		req := CreateHabitRequest{Name: "Exercise", Description: nil}
		_, err := svc.CreateHabit(context.Background(), req)
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if gotDesc != nil {
			t.Errorf("got description %v, want nil", gotDesc)
		}
	})
}
