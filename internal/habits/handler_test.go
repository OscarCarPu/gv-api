package habits_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gv-api/internal/habits"
	"gv-api/internal/habits/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Handler tests cover HTTP-layer concerns only: status codes, decode errors,
// id parsing, error→status mapping. Business rules are covered by service tests.

func newReq(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

func withIDParam(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandler_UpsertLog(t *testing.T) {
	body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`

	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().LogHabit(mock.Anything, mock.Anything).Return(nil)
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).UpsertLog(rec, newReq(http.MethodPost, "/", body))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).UpsertLog(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().LogHabit(mock.Anything, mock.Anything).Return(errors.New("db error"))
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).UpsertLog(rec, newReq(http.MethodPost, "/", body))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetDaily(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetDailyView(mock.Anything, "2025-01-31").Return([]habits.HabitWithLog{{ID: 1, Name: "Exercise"}}, nil)
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).GetDaily(rec, newReq(http.MethodGet, "/?date=2025-01-31", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetDailyView(mock.Anything, "").Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).GetDaily(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_CreateHabit(t *testing.T) {
	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateHabit(mock.Anything, mock.Anything).Return(habits.CreateHabitResponse{ID: 1, Name: "Exercise"}, nil)
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).CreateHabit(rec, newReq(http.MethodPost, "/", `{"name": "Exercise"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).CreateHabit(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// Cross-field rule unique to this endpoint, worth one explicit test.
	t.Run("400 when target_min > target_max", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).CreateHabit(rec, newReq(http.MethodPost, "/", `{"name": "E", "target_min": 10, "target_max": 5}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateHabit(mock.Anything, mock.Anything).Return(habits.CreateHabitResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).CreateHabit(rec, newReq(http.MethodPost, "/", `{"name": "Exercise"}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_DeleteHabit(t *testing.T) {
	t.Run("204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteHabit(mock.Anything, int32(5)).Return(nil)
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).DeleteHabit(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "5"))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).DeleteHabit(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteHabit(mock.Anything, int32(1)).Return(errors.New("db error"))
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).DeleteHabit(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "1"))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetHistory(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetHistory(mock.Anything, int32(1), "daily", "2026-03-01", "2026-03-19").Return(habits.HistoryResponse{
			StartAt: "2026-03-01", EndAt: "2026-03-19", Data: []habits.HistoryPoint{},
		}, nil)
		rec := httptest.NewRecorder()
		habits.NewHandler(svc).GetHistory(rec, withIDParam(newReq(http.MethodGet, "/?frequency=daily&start_at=2026-03-01&end_at=2026-03-19", ""), "1"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).GetHistory(rec, withIDParam(newReq(http.MethodGet, "/?frequency=daily", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("400 on invalid frequency", func(t *testing.T) {
		rec := httptest.NewRecorder()
		habits.NewHandler(mocks.NewMockServiceInterface(t)).GetHistory(rec, withIDParam(newReq(http.MethodGet, "/?frequency=yearly", ""), "1"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}
