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

func TestGetDailyView_DateParsing(t *testing.T) {
	repo := &mockRepo{
		getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
			expectedDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
			if !date.Equal(expectedDate) {
				t.Errorf("expected date %v, got %v", expectedDate, date)
			}
			return []HabitWithLog{}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.GetDailyView(context.Background(), "2025-01-31")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	_, err = svc.GetDailyView(context.Background(), "invalid-date")
	if err == nil {
		t.Errorf("expected error, got %v", err)
	}
}

func TestGetDailyView_EmptyResults(t *testing.T) {
	repo := &mockRepo{
		getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
			return []HabitWithLog{}, nil
		},
	}
	svc := NewService(repo)

	res, _ := svc.GetDailyView(context.Background(), "")

	if len(res) != 0 {
		t.Errorf("expected 0 results, got %d", len(res))
	}
}

func TestLogHabit_DelegatesToRepo(t *testing.T) {
	var capturedHabitID int32
	var capturedDate time.Time
	var capturedValue float32

	mock := &mockRepo{
		upsertLogFn: func(ctx context.Context, habitID int32, date time.Time, value float32) error {
			capturedHabitID = habitID
			capturedDate = date
			capturedValue = value
			return nil
		},
	}
	service := NewService(mock)

	expectedDate := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	req := LogUpsertRequest{
		HabitID: 5,
		Date:    "2025-01-31",
		Value:   100.0,
	}

	err := service.LogHabit(context.Background(), req)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if capturedHabitID != 5 {
		t.Errorf("expected habitID 5, got %d", capturedHabitID)
	}
	if !capturedDate.Equal(expectedDate) {
		t.Errorf("expected date %v, got %v", expectedDate, capturedDate)
	}
	if capturedValue != 100.0 {
		t.Errorf("expected value 100.0, got %f", capturedValue)
	}
}

func TestLogHabit_InvalidDate(t *testing.T) {
	service := NewService(&mockRepo{})

	req := LogUpsertRequest{
		HabitID: 1,
		Date:    "not-a-date",
		Value:   10.0,
	}

	err := service.LogHabit(context.Background(), req)
	if err == nil {
		t.Error("expected error for invalid date, got nil")
	}
}

func TestGetDailyView_EmptyDateUsesToday(t *testing.T) {
	var capturedDate time.Time
	repo := &mockRepo{
		getHabitsFn: func(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
			capturedDate = date
			return []HabitWithLog{}, nil
		},
	}
	svc := NewService(repo)

	_, err := svc.GetDailyView(context.Background(), "")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	now := time.Now().UTC()
	expectedDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if !capturedDate.Equal(expectedDate) {
		t.Errorf("expected today's date %v, got %v", expectedDate, capturedDate)
	}
}

func TestCreateHabit_DelegatesToRepo(t *testing.T) {
	var capturedName string
	var capturedDesc *string

	desc := "test description"
	mock := &mockRepo{
		createHabitFn: func(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
			capturedName = name
			capturedDesc = description
			return CreateHabitResponse{ID: 1, Name: name, Description: description}, nil
		},
	}
	service := NewService(mock)

	req := CreateHabitRequest{
		Name:        "Exercise",
		Description: &desc,
	}

	resp, err := service.CreateHabit(context.Background(), req)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if capturedName != "Exercise" {
		t.Errorf("expected name 'Exercise', got %s", capturedName)
	}
	if capturedDesc == nil || *capturedDesc != "test description" {
		t.Errorf("expected description 'test description', got %v", capturedDesc)
	}
	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.ID)
	}
}

func TestCreateHabit_NilDescription(t *testing.T) {
	var capturedDesc *string

	mock := &mockRepo{
		createHabitFn: func(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
			capturedDesc = description
			return CreateHabitResponse{ID: 1, Name: name}, nil
		},
	}
	service := NewService(mock)

	req := CreateHabitRequest{
		Name:        "Exercise",
		Description: nil,
	}

	_, err := service.CreateHabit(context.Background(), req)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if capturedDesc != nil {
		t.Errorf("expected nil description, got %v", capturedDesc)
	}
}
