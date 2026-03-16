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
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetDailyView(mock.Anything, "2025-01-31").Return([]habits.HabitWithLog{
			{ID: 1, Name: "Exercise", Description: &desc1, LogValue: nil},
			{ID: 2, Name: "Reading", Description: &desc2, LogValue: &val},
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
		assert.Equal(t, "Reading", got[1].Name)
		require.NotNil(t, got[1].LogValue)
		assert.Equal(t, float32(42.5), *got[1].LogValue)
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
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().
			CreateHabit(mock.Anything, mock.MatchedBy(func(req habits.CreateHabitRequest) bool {
				return req.Name == "Exercise"
			})).
			Return(habits.CreateHabitResponse{ID: 1, Name: "Exercise", Description: &desc}, nil)
		handler := habits.NewHandler(svc)

		body := `{"name": "Exercise", "description": "Test description"}`
		req := httptest.NewRequest(http.MethodPost, "/habits", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateHabit(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got habits.CreateHabitResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "Exercise", got.Name)
		assert.Equal(t, &desc, got.Description)
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
