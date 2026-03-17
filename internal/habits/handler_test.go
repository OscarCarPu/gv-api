package habits_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/stretchr/testify/require"
)

func TestHandler_UpsertLog(t *testing.T) {
	t.Run("delegates to service on valid input", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().
			LogHabit(mock.Anything, mock.MatchedBy(func(req habits.LogUpsertRequest) bool {
				return req.HabitID == 1 && req.Value == 42.5
			})).
			Return(nil)
		handler := habits.NewHandler(svc)

		body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`
		req := httptest.NewRequest(http.MethodPost, "/habits/log", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.UpsertLog(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/habits/log", strings.NewReader("not json"))
		rec := httptest.NewRecorder()

		handler.UpsertLog(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().LogHabit(mock.Anything, mock.Anything).Return(errors.New("db error"))
		handler := habits.NewHandler(svc)

		body := `{"habit_id": 1, "date": "2025-01-31", "value": 42.5}`
		req := httptest.NewRequest(http.MethodPost, "/habits/log", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.UpsertLog(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to log")
	})
}

func TestHandler_GetDaily(t *testing.T) {
	t.Run("returns habits with logs as JSON", func(t *testing.T) {
		desc1 := "Daily workout"
		desc2 := "Read a book"
		val := float32(42.5)
		tmin := float32(50)
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetDailyView(mock.Anything, "2025-01-31").Return([]habits.HabitWithLog{
			{ID: 1, Name: "Exercise", Description: &desc1, Frequency: "daily", TargetMin: &tmin, RecordingRequired: true, LogValue: nil, PeriodValue: 0, CurrentStreak: 3, LongestStreak: 10},
			{ID: 2, Name: "Reading", Description: &desc2, Frequency: "weekly", RecordingRequired: true, LogValue: &val, PeriodValue: 42.5, CurrentStreak: 0, LongestStreak: 0},
		}, nil)
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/habits?date=2025-01-31", nil)
		rec := httptest.NewRecorder()

		handler.GetDaily(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var got []habits.HabitWithLog
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got, 2)
		assert.Equal(t, "Exercise", got[0].Name)
		assert.Equal(t, "daily", got[0].Frequency)
		assert.Equal(t, int32(3), got[0].CurrentStreak)
		assert.Equal(t, int32(10), got[0].LongestStreak)
		assert.Equal(t, "Reading", got[1].Name)
		assert.Equal(t, "weekly", got[1].Frequency)
		require.NotNil(t, got[1].LogValue)
		assert.Equal(t, float32(42.5), *got[1].LogValue)
		assert.Equal(t, float32(42.5), got[1].PeriodValue)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetDailyView(mock.Anything, "").Return(nil, errors.New("db error"))
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/habits", nil)
		rec := httptest.NewRecorder()

		handler.GetDaily(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_CreateHabit(t *testing.T) {
	t.Run("returns 201 with created habit", func(t *testing.T) {
		desc := "Test description"
		tmin := float32(5)
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().
			CreateHabit(mock.Anything, mock.MatchedBy(func(req habits.CreateHabitRequest) bool {
				return req.Name == "Exercise"
			})).
			Return(habits.CreateHabitResponse{ID: 1, Name: "Exercise", Description: &desc, Frequency: "weekly", TargetMin: &tmin, RecordingRequired: true}, nil)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "description": "Test description", "frequency": "weekly", "target_min": 5}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got habits.CreateHabitResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "Exercise", got.Name)
		assert.Equal(t, &desc, got.Description)
		assert.Equal(t, "weekly", got.Frequency)
		require.NotNil(t, got.TargetMin)
		assert.Equal(t, float32(5), *got.TargetMin)
	})

	t.Run("returns 400 for invalid frequency", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "frequency": "yearly"}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "frequency must be daily, weekly, or monthly")
	})

	t.Run("returns 400 for negative target_min", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "target_min": -1}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "target_min must be")
	})

	t.Run("returns 400 for negative target_max", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "target_max": -1}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "target_max must be")
	})

	t.Run("returns 400 when target_min > target_max", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "target_min": 10, "target_max": 5}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "target_min must be")
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader("not json"))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(`{"description": "no name provided"}`))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "name is required")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateHabit(mock.Anything, mock.Anything).Return(habits.CreateHabitResponse{}, errors.New("db error"))
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(`{"name": "Exercise"}`))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to create habit")
	})
}

func TestHandler_DeleteHabit(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteHabit(mock.Anything, int32(5)).Return(nil)
		handler := habits.NewHandler(svc)

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
		svc := mocks.NewMockServiceInterface(t)
		handler := habits.NewHandler(svc)

		req := httptest.NewRequest(http.MethodDelete, "/habits/abc", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.DeleteHabit(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteHabit(mock.Anything, int32(1)).Return(errors.New("db error"))
		handler := habits.NewHandler(svc)

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
