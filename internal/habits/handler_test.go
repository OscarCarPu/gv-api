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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockService struct {
	logHabitFn     func(ctx context.Context, req LogUpsertRequest) error
	getDailyViewFn func(ctx context.Context, dateStr string) ([]HabitWithLog, error)
	createHabitFn  func(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error)
	deleteHabitFn  func(ctx context.Context, id int32) error
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

func (m *mockService) DeleteHabit(ctx context.Context, id int32) error {
	if m.deleteHabitFn != nil {
		return m.deleteHabitFn(ctx, id)
	}
	return nil
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

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, int32(1), got.HabitID)
		assert.Equal(t, float32(42.5), got.Value)
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

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
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

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var habits []HabitWithLog
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&habits))
		require.Len(t, habits, 2)
		assert.Equal(t, "Exercise", habits[0].Name)
		assert.Equal(t, "Reading", habits[1].Name)
		require.NotNil(t, habits[1].LogValue)
		assert.Equal(t, float32(42.5), *habits[1].LogValue)
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

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
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

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got CreateHabitResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "Exercise", got.Name)
		assert.Equal(t, &desc, got.Description)
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

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_DeleteHabit(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		mock := &mockService{
			deleteHabitFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(5), id)
				return nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodDelete, "/habits/5", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "5")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.DeleteHabit(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodDelete, "/habits/abc", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.DeleteHabit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		mock := &mockService{
			deleteHabitFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodDelete, "/habits/1", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.DeleteHabit(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to delete habit")
	})
}
