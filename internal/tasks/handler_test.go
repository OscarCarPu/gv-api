package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gv-api/internal/history"
	"gv-api/internal/tasks"
	"gv-api/internal/tasks/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Handler tests assert HTTP-layer concerns only: status codes, decode errors,
// id parsing, error→status mapping. Business rules (priority defaults,
// dependency resolution, validation specifics) are covered by service tests.

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

// --- Create ---

func TestHandler_CreateProject(t *testing.T) {
	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{ID: 1, Name: "P"}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateProject(rec, newReq(http.MethodPost, "/", `{"name": "P"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateProject(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// One representative validation test — the "name length" rule is enforced by
	// the same validator across many endpoints; one assertion is enough to prove
	// the handler wires it up. See validation_test.go (if added) for matrix coverage.
	t.Run("400 when name exceeds limit", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateProject(rec, newReq(http.MethodPost, "/", `{"name": "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddx"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "name must be at most 40 characters")
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateProject(rec, newReq(http.MethodPost, "/", `{"name": "P"}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_CreateTask(t *testing.T) {
	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 1, Name: "T"}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTask(rec, newReq(http.MethodPost, "/", `{"name": "T"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateTask(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// task_type/recurrence rule is a handler-layer cross-field check unique to
	// this endpoint, so it's worth one test here.
	t.Run("400 when recurring without recurrence", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateTask(rec, newReq(http.MethodPost, "/", `{"name": "T", "task_type": "recurring"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "recurrence is required")
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTask(rec, newReq(http.MethodPost, "/", `{"name": "T"}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_CreateTodo(t *testing.T) {
	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{ID: 1, TaskID: 5, Name: "T"}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTodo(rec, newReq(http.MethodPost, "/", `{"task_id": 5, "name": "T"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateTodo(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTodo(rec, newReq(http.MethodPost, "/", `{"task_id": 5, "name": "T"}`))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_CreateTimeEntry(t *testing.T) {
	body := `{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`

	t.Run("201 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{ID: 1, TaskID: 3}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTimeEntry(rec, newReq(http.MethodPost, "/", body))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("400 on invalid JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).CreateTimeEntry(rec, newReq(http.MethodPost, "/", "not json"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	// Unique HTTP-level mapping — sentinel error → 409.
	t.Run("409 when active entry exists", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, tasks.ErrActiveTimeEntryExists)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTimeEntry(rec, newReq(http.MethodPost, "/", body))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).CreateTimeEntry(rec, newReq(http.MethodPost, "/", body))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// --- Update ---

func TestHandler_UpdateProject(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{ID: 5, Name: "P"}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"name": "P"}`), "5")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateProject(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "abc")
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).UpdateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateProject(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_UpdateTask(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 7, Name: "T"}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"name": "T"}`), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTask(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	// Tri-state nullable fields are a real handler-layer concern: the JSON
	// distinction between "field omitted", "field: null", and "field: value"
	// must round-trip through to the service.
	t.Run("explicit null due_at marks field set with nil value", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
			return req.DueAt.Set && req.DueAt.Value == nil
		})).Return(tasks.TaskResponse{ID: 7}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"due_at": null}`), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTask(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("omitted due_at leaves field unset", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
			return !req.DueAt.Set
		})).Return(tasks.TaskResponse{ID: 7}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"name": "T"}`), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTask(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "abc")
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).UpdateTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTask(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_UpdateTodo(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{ID: 3, IsDone: true}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"is_done": true}`), "3")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTodo(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "abc")
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).UpdateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTodo(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTodo(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_UpdateTimeEntry(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{ID: 7}, nil)
		req := withIDParam(newReq(http.MethodPatch, "/", `{"finished_at": "2026-03-01T17:00:00Z"}`), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "abc")
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodPatch, "/", `{}`), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// --- Get ---

func TestHandler_GetRootProjects(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetRootProjects(mock.Anything).Return([]tasks.ProjectResponse{{ID: 1}}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetRootProjects(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetRootProjects(mock.Anything).Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetRootProjects(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetActiveTree(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything, mock.Anything).Return([]tasks.ActiveTreeNode{{ID: 1, Type: "task"}}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetActiveTree(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid query param", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetActiveTree(rec, newReq(http.MethodGet, "/?min_priority=6", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetActiveTree(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetProject(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(5)).Return(tasks.ProjectDetailResponse{ID: 5}, nil)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "5")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetProject(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid id", func(t *testing.T) {
		req := withIDParam(newReq(http.MethodGet, "/", ""), "abc")
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(999)).Return(tasks.ProjectDetailResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetProject(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(1)).Return(tasks.ProjectDetailResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodGet, "/", ""), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetTask(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(7)).Return(tasks.TaskFullResponse{ID: 7}, nil)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTask(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(999)).Return(tasks.TaskFullResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTask(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(1)).Return(tasks.TaskFullResponse{}, errors.New("db error"))
		req := withIDParam(newReq(http.MethodGet, "/", ""), "1")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetProjectChildren(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProjectChildren(mock.Anything, int32(5)).Return(tasks.ProjectChildrenResponse{
			Project: tasks.ProjectDetailResponse{ID: 5},
		}, nil)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "5")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetProjectChildren(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProjectChildren(mock.Anything, int32(999)).Return(tasks.ProjectChildrenResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetProjectChildren(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_GetTaskTimeEntries(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(7)).Return(tasks.TaskTimeEntriesResponse{
			Task: tasks.TaskDetailResponse{ID: 7},
		}, nil)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "7")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTaskTimeEntries(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(999)).Return(tasks.TaskTimeEntriesResponse{}, tasks.ErrNotFound)
		req := withIDParam(newReq(http.MethodGet, "/", ""), "999")
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTaskTimeEntries(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestHandler_GetTasksByDueDate(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything, mock.Anything).Return([]tasks.TaskByDueDateResponse{{ID: 1}}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTasksByDueDate(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("400 on invalid query param", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetTasksByDueDate(rec, newReq(http.MethodGet, "/?min_priority=abc", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything, mock.Anything).Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTasksByDueDate(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// --- Delete (uniform shape — one happy + one bad-id + one error per endpoint) ---

func TestHandler_DeleteProject(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().DeleteProject(mock.Anything, int32(5)).Return(nil)
	rec := httptest.NewRecorder()
	tasks.NewHandler(svc).DeleteProject(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "5"))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_DeleteTask(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().DeleteTask(mock.Anything, int32(3)).Return(nil)
	rec := httptest.NewRecorder()
	tasks.NewHandler(svc).DeleteTask(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "3"))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_DeleteTodo(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().DeleteTodo(mock.Anything, int32(7)).Return(nil)
	rec := httptest.NewRecorder()
	tasks.NewHandler(svc).DeleteTodo(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "7"))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_DeleteTimeEntry(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().DeleteTimeEntry(mock.Anything, int32(9)).Return(nil)
	rec := httptest.NewRecorder()
	tasks.NewHandler(svc).DeleteTimeEntry(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "9"))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// One representative bad-id and service-error case for the delete shape — the
// underlying handler logic is identical across the four delete endpoints.
func TestHandler_Delete_BadIDAndServiceError(t *testing.T) {
	t.Run("400 on invalid id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).DeleteProject(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "abc"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteProject(mock.Anything, int32(1)).Return(errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).DeleteProject(rec, withIDParam(newReq(http.MethodDelete, "/", ""), "1"))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

// --- Active time entry / history / by-date-range ---

func TestHandler_GetActiveTimeEntry(t *testing.T) {
	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.ActiveTimeEntryResponse{ID: 1, TaskID: 5, StartedAt: time.Now()}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetActiveTimeEntry(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("404 when none active", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.ActiveTimeEntryResponse{}, tasks.ErrNotFound)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetActiveTimeEntry(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.ActiveTimeEntryResponse{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetActiveTimeEntry(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetTimeEntryHistory(t *testing.T) {
	t.Run("400 when frequency missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetTimeEntryHistory(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("400 when frequency invalid", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetTimeEntryHistory(rec, newReq(http.MethodGet, "/?frequency=yearly", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTimeEntryHistory(mock.Anything, "daily", "2026-03-01", "2026-03-19").Return(history.Response{
			StartAt: "2026-03-01", EndAt: "2026-03-19", Data: []history.Point{},
		}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTimeEntryHistory(rec, newReq(http.MethodGet, "/?frequency=daily&start_at=2026-03-01&end_at=2026-03-19", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTimeEntryHistory(mock.Anything, "daily", "", "").Return(history.Response{}, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTimeEntryHistory(rec, newReq(http.MethodGet, "/?frequency=daily", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestHandler_GetTimeEntriesByDateRange(t *testing.T) {
	t.Run("400 when start_time missing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetTimeEntriesByDateRange(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("400 on bad date format", func(t *testing.T) {
		rec := httptest.NewRecorder()
		tasks.NewHandler(mocks.NewMockServiceInterface(t)).GetTimeEntriesByDateRange(rec, newReq(http.MethodGet, "/?start_time=bad", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("200 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTimeEntriesByDateRange(mock.Anything, "2026-03-01", "2026-03-31").Return([]tasks.TimeEntryWithTaskResponse{{ID: 1}}, nil)
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTimeEntriesByDateRange(rec, newReq(http.MethodGet, "/?start_time=2026-03-01&end_time=2026-03-31", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTimeEntriesByDateRange(mock.Anything, "2026-03-01", "2026-03-31").Return(nil, errors.New("db error"))
		rec := httptest.NewRecorder()
		tasks.NewHandler(svc).GetTimeEntriesByDateRange(rec, newReq(http.MethodGet, "/?start_time=2026-03-01&end_time=2026-03-31", ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}
