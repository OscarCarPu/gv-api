package habits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Test Helpers ---

func assertStatus(t testing.TB, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got status %d, want %d", got, want)
	}
}

func assertBodyContains(t testing.TB, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("body %q does not contain %q", body, want)
	}
}

// --- Mocks ---

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

func ptrFloat32(f float32) *float32 {
	return &f
}

// --- Handler Tests ---

func TestHandler_UpsertLog(t *testing.T) {
	t.Run("delegates to service on valid input", func(t *testing.T) {
		var got LogUpsertRequest
		mock := &mockService{
			logHabitFn: func(ctx context.Context, req LogUpsertRequest) error {
				got = req
				return nil
			},
		}
		handler := NewHandler(mock)

		body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`
		req := httptest.NewRequest(http.MethodPost, "/habits/log", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.UpsertLog(rec, req)

		assertStatus(t, rec.Code, http.StatusOK)
		if got.HabitID != 1 {
			t.Errorf("got HabitID %d, want 1", got.HabitID)
		}
		if got.Value != 42.5 {
			t.Errorf("got Value %f, want 42.5", got.Value)
		}
	})

	errorCases := []struct {
		name       string
		body       string
		setupMock  func() *mockService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 400 for invalid JSON",
			body:       "not json",
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid Body",
		},
		{
			name: "returns 500 when service fails",
			body: `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`,
			setupMock: func() *mockService {
				return &mockService{
					logHabitFn: func(ctx context.Context, req LogUpsertRequest) error {
						return errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to log",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/habits/log", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.UpsertLog(rec, req)

			assertStatus(t, rec.Code, tc.wantStatus)
			assertBodyContains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_GetDaily(t *testing.T) {
	t.Run("returns habits with logs as JSON", func(t *testing.T) {
		desc1 := "Daily workout"
		desc2 := "Read a book"
		mock := &mockService{
			getDailyViewFn: func(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
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

		assertStatus(t, rec.Code, http.StatusOK)

		got := rec.Header().Get("Content-Type")
		want := "application/json"
		if got != want {
			t.Errorf("got Content-Type %q, want %q", got, want)
		}

		var habits []HabitWithLog
		if err := json.NewDecoder(rec.Body).Decode(&habits); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(habits) != 2 {
			t.Fatalf("got %d habits, want 2", len(habits))
		}
		if habits[0].Name != "Exercise" {
			t.Errorf("got name %q, want %q", habits[0].Name, "Exercise")
		}
		if habits[1].Name != "Reading" {
			t.Errorf("got name %q, want %q", habits[1].Name, "Reading")
		}
		if habits[1].LogValue == nil {
			t.Fatalf("got nil LogValue, want 42.5")
		}
		if *habits[1].LogValue != 42.5 {
			t.Errorf("got LogValue %f, want 42.5", *habits[1].LogValue)
		}
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			getDailyViewFn: func(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
				return nil, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/habits", nil)
		rec := httptest.NewRecorder()

		handler.GetDaily(rec, req)

		assertStatus(t, rec.Code, http.StatusInternalServerError)
	})
}

func TestHandler_CreateHabit(t *testing.T) {
	t.Run("returns 201 with created habit", func(t *testing.T) {
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

		assertStatus(t, rec.Code, http.StatusCreated)

		var got CreateHabitResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.ID != 1 {
			t.Errorf("got ID %d, want 1", got.ID)
		}
		if got.Name != "Exercise" {
			t.Errorf("got name %q, want %q", got.Name, "Exercise")
		}
		if got.Description == nil || *got.Description != desc {
			t.Errorf("got description %v, want %q", got.Description, desc)
		}
	})

	errorCases := []struct {
		name       string
		body       string
		setupMock  func() *mockService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "returns 400 for invalid JSON",
			body:       "not json",
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid Body",
		},
		{
			name:       "returns 400 when name is missing",
			body:       `{"description": "no name provided"}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
		{
			name: "returns 500 when service fails",
			body: `{"name": "Exercise"}`,
			setupMock: func() *mockService {
				return &mockService{
					createHabitFn: func(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
						return CreateHabitResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create habit",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateHabit(rec, req)

			assertStatus(t, rec.Code, tc.wantStatus)
			assertBodyContains(t, rec.Body.String(), tc.wantBody)
		})
	}
}
