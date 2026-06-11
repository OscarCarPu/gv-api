package varieties_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gv-api/internal/varieties"
	"gv-api/internal/varieties/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Handler tests cover HTTP-layer concerns only: status codes, decode errors,
// id parsing, validation messages, error→status mapping.

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

func TestHandler_Get(t *testing.T) {
	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		varieties.NewHandler(mocks.NewMockServiceInterface(t)).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "invalid variety id", errMessage(t, rec))
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Get(mock.Anything, int32(7)).Return(varieties.Variety{}, varieties.ErrNotFound)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "7"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Equal(t, "variety not found", errMessage(t, rec))
	})

	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Get(mock.Anything, int32(7)).Return(varieties.Variety{ID: 7, Name: "Amnesia"}, nil)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Get(rec, withIDParam(newReq(http.MethodGet, "/", ""), "7"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHandler_List(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List(mock.Anything).Return([]varieties.Variety{{ID: 1}}, nil)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).List(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).List(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// validBody returns a create/update payload with one field overridden.
func validBody(field string, value any) string {
	m := map[string]any{
		"name": "Amnesia", "judge": "Oscar",
		"scent": 7, "flavor": 8, "power": 6, "quality": 7.5, "price": 10,
	}
	if field != "" {
		m[field] = value
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestHandler_Create_Validation(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"missing name", validBody("name", ""), "name is required"},
		{"name over 40 chars", validBody("name", strings.Repeat("a", 41)), "name must be at most 40 characters"},
		{"whitespace-only judge", validBody("judge", "   "), "judge is required"},
		{"judge over 40 chars", validBody("judge", strings.Repeat("j", 41)), "judge must be at most 40 characters"},
	}
	for _, field := range []string{"scent", "flavor", "power", "quality"} {
		cases = append(cases,
			struct {
				name    string
				body    string
				wantMsg string
			}{field + " below 0", validBody(field, -1), fmt.Sprintf("%s must be between 0 and 10", field)},
			struct {
				name    string
				body    string
				wantMsg string
			}{field + " above 10", validBody(field, 11), fmt.Sprintf("%s must be between 0 and 10", field)},
		)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			varieties.NewHandler(mocks.NewMockServiceInterface(t)).Create(rec, newReq(http.MethodPost, "/", tc.body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, tc.wantMsg, errMessage(t, rec))
		})
	}
}

func TestHandler_Create(t *testing.T) {
	t.Run("201 on success with trimmed judge", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.MatchedBy(func(req varieties.CreateVarietyRequest) bool {
			return req.Name == "Amnesia" && req.Judge == "Oscar"
		})).Return(varieties.Variety{ID: 1, Name: "Amnesia"}, nil)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/", validBody("judge", "  Oscar  ")))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Create(mock.Anything, mock.Anything).Return(varieties.Variety{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Create(rec, newReq(http.MethodPost, "/", validBody("", nil)))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_Update(t *testing.T) {
	t.Run("400 on missing name", func(t *testing.T) {
		rec := httptest.NewRecorder()
		varieties.NewHandler(mocks.NewMockServiceInterface(t)).Update(rec, withIDParam(newReq(http.MethodPut, "/", validBody("name", "")), "1"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "name is required", errMessage(t, rec))
	})

	t.Run("400 on out-of-range score", func(t *testing.T) {
		rec := httptest.NewRecorder()
		varieties.NewHandler(mocks.NewMockServiceInterface(t)).Update(rec, withIDParam(newReq(http.MethodPut, "/", validBody("power", 11)), "1"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "power must be between 0 and 10", errMessage(t, rec))
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.Anything).Return(varieties.Variety{}, varieties.ErrNotFound)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Update(rec, withIDParam(newReq(http.MethodPut, "/", validBody("", nil)), "1"))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("200 on success with ID from URL param and trimmed judge", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Update(mock.Anything, mock.MatchedBy(func(req varieties.UpdateVarietyRequest) bool {
			return req.ID == 5 && req.Judge == "Oscar"
		})).Return(varieties.Variety{ID: 5}, nil)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Update(rec, withIDParam(newReq(http.MethodPut, "/", validBody("judge", " Oscar ")), "5"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestHandler_Delete(t *testing.T) {
	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		varieties.NewHandler(mocks.NewMockServiceInterface(t)).Delete(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().Delete(mock.Anything, int32(5)).Return(nil)
		rec := httptest.NewRecorder()
		varieties.NewHandler(svc).Delete(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "5"))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}
