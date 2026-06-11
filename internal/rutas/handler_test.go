package rutas_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gv-api/internal/rutas"
	"gv-api/internal/rutas/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Handler tests cover HTTP-layer concerns only: status codes, decode errors,
// id parsing, error→status mapping. The service is a pass-through to the
// repository, so business behavior lives in repository tests.

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

func errMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error JSON: %v", err)
	}
	return body["error"]
}

func TestHandler_List(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List(mock.Anything).Return([]rutas.ConcelloMark{{ID: 1, Name: "Arzúa"}}, nil)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).List(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).List(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Get(t *testing.T) {
	t.Run("400 on non-numeric id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "invalid mark id", errMessage(t, rec))
	})

	t.Run("400 on zero id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "0"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "invalid mark id", errMessage(t, rec))
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Get(mock.Anything, int32(7)).Return(rutas.ConcelloMark{}, rutas.ErrNotFound)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "7"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "mark not found", errMessage(t, rec))
	})

	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Get(mock.Anything, int32(7)).Return(rutas.ConcelloMark{ID: 7, Name: "Melide"}, nil)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "7"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHandler_Create(t *testing.T) {
	t.Run("400 on malformed JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "invalid body", errMessage(t, rec))
	})

	t.Run("400 on whitespace-only name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", `{"name": "   ", "visited_on": "2026-06-11"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "name is required", errMessage(t, rec))
	})

	t.Run("400 on name over 200 chars", func(t *testing.T) {
		long := strings.Repeat("a", 201)
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", `{"name": "`+long+`", "visited_on": "2026-06-11"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "name too long", errMessage(t, rec))
	})

	t.Run("400 on missing visited_on", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", `{"name": "Arzúa"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "visited_on is required", errMessage(t, rec))
	})

	t.Run("400 on non YYYY-MM-DD visited_on", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", `{"name": "Arzúa", "visited_on": "11/06/2026"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, errMessage(t, rec), "must be YYYY-MM-DD")
	})

	t.Run("201 on success with trimmed name", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.MatchedBy(func(req rutas.CreateMarkRequest) bool {
			return req.Name == "Arzúa" && req.VisitedOn == "2026-06-11"
		})).Return(rutas.ConcelloMark{ID: 1, Name: "Arzúa"}, nil)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/", `{"name": "  Arzúa  ", "visited_on": "2026-06-11"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Run("400 on missing visited_on", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rutas.NewHandler(mocks.NewMockServiceInterface(t)).Update(rec, withIDParam(newReq(http.MethodPut, "/", `{"description": "x"}`), "3"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "visited_on is required", errMessage(t, rec))
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.Anything).Return(rutas.ConcelloMark{}, rutas.ErrNotFound)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Update(rec, withIDParam(newReq(http.MethodPut, "/", `{"visited_on": "2026-06-11"}`), "3"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "mark not found", errMessage(t, rec))
	})

	t.Run("200 on success with ID set from URL param", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.MatchedBy(func(req rutas.UpdateMarkRequest) bool {
			return req.ID == 3 && req.VisitedOn == "2026-06-11"
		})).Return(rutas.ConcelloMark{ID: 3}, nil)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Update(rec, withIDParam(newReq(http.MethodPut, "/", `{"visited_on": "2026-06-11"}`), "3"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHandler_Delete(t *testing.T) {
	// Delete does not map ErrNotFound; any service error is a 500. This test
	// documents the current contract.
	t.Run("500 when service returns ErrNotFound", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Delete(mock.Anything, int32(9)).Return(rutas.ErrNotFound)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Delete(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "9"))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Delete(mock.Anything, int32(9)).Return(nil)
		rec := httptest.NewRecorder()
		rutas.NewHandler(svc).Delete(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "9"))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}
