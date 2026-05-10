package plan_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gv-api/internal/plan"
	"gv-api/internal/plan/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func withIDParam(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func newReq(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

func TestHandler_GetToday(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetToday(mock.Anything).Return(plan.PlanTodayResponse{Date: "2026-05-10"}, nil)

		rec := httptest.NewRecorder()
		plan.NewHandler(svc).GetToday(rec, newReq(http.MethodGet, "/plan/today", ""))
		assert.Equal(t, http.StatusOK, rec.Code)

		var got plan.PlanTodayResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, "2026-05-10", got.Date)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetToday(mock.Anything).Return(plan.PlanTodayResponse{}, errors.New("db error"))

		rec := httptest.NewRecorder()
		plan.NewHandler(svc).GetToday(rec, newReq(http.MethodGet, "/plan/today", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Create(t *testing.T) {
	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.Anything).Return(plan.PlanBlockResponse{ID: 1, Label: "comer"}, nil)

		rec := httptest.NewRecorder()
		body := `{"started_at":"2026-05-10T12:00:00Z","ended_at":"2026-05-10T13:00:00Z","label":"comer"}`
		plan.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/plan/blocks", body))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		plan.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/plan/blocks", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("400 on validation error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.Anything).Return(plan.PlanBlockResponse{}, plan.ErrInvalidTimeRange)

		rec := httptest.NewRecorder()
		plan.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/plan/blocks",
			`{"started_at":"2026-05-10T12:00:00Z","ended_at":"2026-05-10T11:00:00Z","label":"x"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ended_at must be after started_at")
	})

	t.Run("400 when task not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.Anything).Return(plan.PlanBlockResponse{}, plan.ErrTaskNotFound)

		rec := httptest.NewRecorder()
		plan.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/plan/blocks",
			`{"started_at":"2026-05-10T12:00:00Z","ended_at":"2026-05-10T13:00:00Z","task_id":999}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on unexpected error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.Anything).Return(plan.PlanBlockResponse{}, errors.New("db down"))

		rec := httptest.NewRecorder()
		plan.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/plan/blocks",
			`{"started_at":"2026-05-10T12:00:00Z","ended_at":"2026-05-10T13:00:00Z","label":"x"}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.MatchedBy(func(req plan.UpdatePlanBlockRequest) bool {
			return req.ID == 5
		})).Return(plan.PlanBlockResponse{ID: 5, Label: "updated"}, nil)

		rec := httptest.NewRecorder()
		req := newReq(http.MethodPut, "/plan/blocks/5", `{"label":"updated"}`)
		plan.NewHandler(svc).Update(rec, withIDParam(req, "5"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := newReq(http.MethodPut, "/plan/blocks/abc", `{}`)
		plan.NewHandler(mocks.NewMockServiceInterface(t)).Update(rec, withIDParam(req, "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when block missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.Anything).Return(plan.PlanBlockResponse{}, plan.ErrNotFound)

		rec := httptest.NewRecorder()
		req := newReq(http.MethodPut, "/plan/blocks/5", `{}`)
		plan.NewHandler(svc).Update(rec, withIDParam(req, "5"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_Delete(t *testing.T) {
	t.Run("204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Delete(mock.Anything, int32(7)).Return(nil)

		rec := httptest.NewRecorder()
		req := newReq(http.MethodDelete, "/plan/blocks/7", "")
		plan.NewHandler(svc).Delete(rec, withIDParam(req, "7"))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Delete(mock.Anything, int32(7)).Return(errors.New("db error"))

		rec := httptest.NewRecorder()
		req := newReq(http.MethodDelete, "/plan/blocks/7", "")
		plan.NewHandler(svc).Delete(rec, withIDParam(req, "7"))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_DeleteFuture(t *testing.T) {
	t.Run("204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteFuture(mock.Anything).Return(nil)

		rec := httptest.NewRecorder()
		req := newReq(http.MethodDelete, "/plan/blocks/future", "")
		plan.NewHandler(svc).DeleteFuture(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteFuture(mock.Anything).Return(errors.New("db error"))

		rec := httptest.NewRecorder()
		req := newReq(http.MethodDelete, "/plan/blocks/future", "")
		plan.NewHandler(svc).DeleteFuture(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
