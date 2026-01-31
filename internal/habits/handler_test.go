package habits

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockService struct {
	logHabitFn     func(ctx context.Context, req LogUpsertRequest) error
	getDailyViewFn func(ctx context.Context, dateStr string) ([]HabitWithLog, error)
	createHabitFn  func(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error)
}

func (m *mockService) GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
	if m.getDailyViewFn != nil {
		return m.getDailyViewFn(ctx, dateStr)
	}
	return nil, nil
}

func (m *mockService) LogHabit(ctx context.Context, req LogUpsertRequest) error {
	if m.logHabitFn != nil {
		return m.logHabitFn(ctx, req)
	}
	return nil
}

func (m *mockService) CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
	if m.createHabitFn != nil {
		return m.createHabitFn(ctx, req)
	}
	return CreateHabitResponse{}, nil
}

func TestUpsertLog_Success(t *testing.T) {
	var receivedReq LogUpsertRequest
	mock := &mockService{
		logHabitFn: func(ctx context.Context, req LogUpsertRequest) error {
			receivedReq = req
			return nil
		},
	}
	handler := NewHandler(mock)

	body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`
	req := httptest.NewRequest(http.MethodPost, "/habits/log", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.UpsertLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if receivedReq.HabitID != 1 {
		t.Errorf("expected HabitID 1, got %d", receivedReq.HabitID)
	}
	if receivedReq.Value != 42.5 {
		t.Errorf("expected Value 42.5, got %f", receivedReq.Value)
	}
}

func TestGetDaily_ReturnsJSONArray(t *testing.T) {
	mock := &mockService{
		getDailyViewFn: func(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
			desc1 := "Daily workout"
			desc2 := "Read a book"
			return []HabitWithLog{
				{ID: 1, Name: "Exercise", Description: &desc1, LogValue: nil},
				{ID: 2, Name: "Reading", Description: &desc2, LogValue: ptrFloat32(42.5)},
			}, nil
		},
	}
	handler := NewHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/habits?date=2025-01-31", nil)
	rec := httptest.NewRecorder()

	handler.GetDaily(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Exercise") {
		t.Errorf("expected body to contain 'Exercise', got %s", body)
	}

	if !strings.Contains(body, "Reading") {
		t.Errorf("expected body to contain 'Reading', got %s", body)
	}
}

func ptrFloat32(f float32) *float32 {
	return &f
}

func TestUpsertLog_InvalidJSON(t *testing.T) {
	handler := NewHandler(&mockService{})

	req := httptest.NewRequest(http.MethodPost, "/habits/log", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.UpsertLog(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid Body") {
		t.Errorf("expected 'Invalid Body' error, got %s", rec.Body.String())
	}
}

func TestUpsertLog_ServiceError(t *testing.T) {
	mock := &mockService{
		logHabitFn: func(ctx context.Context, req LogUpsertRequest) error {
			return errors.New("db error")
		},
	}
	handler := NewHandler(mock)

	body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`
	req := httptest.NewRequest(http.MethodPost, "/habits/log", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.UpsertLog(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Failed to log") {
		t.Errorf("expected 'Failed to log' error, got %s", rec.Body.String())
	}
}

func TestGetDaily_ServiceError(t *testing.T) {
	mock := &mockService{
		getDailyViewFn: func(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
			return nil, errors.New("db error")
		},
	}
	handler := NewHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/habits", nil)
	rec := httptest.NewRecorder()

	handler.GetDaily(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestCreateHabit_Success(t *testing.T) {
	desc := "Test description"
	mock := &mockService{
		createHabitFn: func(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
			return CreateHabitResponse{ID: 1, Name: req.Name, Description: req.Description}, nil
		},
	}
	handler := NewHandler(mock)

	body := `{"name": "Exercise", "description": "Test description"}`
	req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateHabit(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", rec.Code)
	}
	respBody := rec.Body.String()
	if !strings.Contains(respBody, "Exercise") {
		t.Errorf("expected response to contain 'Exercise', got %s", respBody)
	}
	_ = desc
}

func TestCreateHabit_InvalidJSON(t *testing.T) {
	handler := NewHandler(&mockService{})

	req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.CreateHabit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid Body") {
		t.Errorf("expected 'Invalid Body' error, got %s", rec.Body.String())
	}
}

func TestCreateHabit_MissingName(t *testing.T) {
	handler := NewHandler(&mockService{})

	body := `{"description": "no name provided"}`
	req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateHabit(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "name is required") {
		t.Errorf("expected 'name is required' error, got %s", rec.Body.String())
	}
}

func TestCreateHabit_ServiceError(t *testing.T) {
	mock := &mockService{
		createHabitFn: func(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
			return CreateHabitResponse{}, errors.New("db error")
		},
	}
	handler := NewHandler(mock)

	body := `{"name": "Exercise"}`
	req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.CreateHabit(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Failed to create habit") {
		t.Errorf("expected 'Failed to create habit' error, got %s", rec.Body.String())
	}
}
