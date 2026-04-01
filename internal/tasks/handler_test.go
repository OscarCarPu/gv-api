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

func withIDParam(req *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// --- Create Tests ---

func TestHandler_CreateProject(t *testing.T) {
	t.Run("returns 201 with created project", func(t *testing.T) {
		desc := "Test description"
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{ID: 1, Name: "My Project", Description: &desc}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader(`{"name": "My Project", "description": "Test description"}`))
		rec := httptest.NewRecorder()
		handler.CreateProject(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "My Project", got.Name)
		assert.Equal(t, &desc, got.Description)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		handler.CreateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader(`{"description": "no name"}`))
		rec := httptest.NewRecorder()
		handler.CreateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "name is required")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader(`{"name": "My Project"}`))
		rec := httptest.NewRecorder()
		handler.CreateProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to create project")
	})
}

func TestHandler_CreateTask(t *testing.T) {
	t.Run("returns 201 with created task", func(t *testing.T) {
		projectID := int32(1)
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 1, ProjectID: &projectID, Name: "My Task"}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(`{"project_id": 1, "name": "My Task"}`))
		rec := httptest.NewRecorder()
		handler.CreateTask(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "My Task", got.Name)
		assert.Equal(t, &projectID, got.ProjectID)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		handler.CreateTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(`{"project_id": 1}`))
		rec := httptest.NewRecorder()
		handler.CreateTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "name is required")
	})

	t.Run("passes depends_on to service and returns it", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTask(mock.Anything, mock.MatchedBy(func(req tasks.CreateTaskRequest) bool {
			return req.Name == "My Task" && len(req.DependsOn) == 2 && req.DependsOn[0] == 2 && req.DependsOn[1] == 3
		})).Return(tasks.TaskResponse{
			ID: 1, Name: "My Task",
			DependsOn:   []tasks.TaskDepRef{{ID: 2, Name: "Dep A"}, {ID: 3, Name: "Dep B"}},
			Blocks: []tasks.TaskDepRef{},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(`{"name": "My Task", "depends_on": [2, 3]}`))
		rec := httptest.NewRecorder()
		handler.CreateTask(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got.DependsOn, 2)
		assert.Equal(t, int32(2), got.DependsOn[0].ID)
		assert.Equal(t, int32(3), got.DependsOn[1].ID)
		assert.Empty(t, got.Blocks)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(`{"name": "My Task"}`))
		rec := httptest.NewRecorder()
		handler.CreateTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to create task")
	})
}

func TestHandler_CreateTodo(t *testing.T) {
	t.Run("returns 201 with created todo", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{ID: 1, TaskID: 5, Name: "My Todo"}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(`{"task_id": 5, "name": "My Todo"}`))
		rec := httptest.NewRecorder()
		handler.CreateTodo(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.TodoResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(5), got.TaskID)
		assert.Equal(t, "My Todo", got.Name)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		handler.CreateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 400 when task_id is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(`{"name": "My Todo"}`))
		rec := httptest.NewRecorder()
		handler.CreateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "task_id is required")
	})

	t.Run("returns 400 when name is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(`{"task_id": 5}`))
		rec := httptest.NewRecorder()
		handler.CreateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "name is required")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(`{"task_id": 5, "name": "My Todo"}`))
		rec := httptest.NewRecorder()
		handler.CreateTodo(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to create todo")
	})
}

func TestHandler_CreateTimeEntry(t *testing.T) {
	t.Run("returns 201 with created time entry", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{ID: 1, TaskID: 3, StartedAt: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(`{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`))
		rec := httptest.NewRecorder()
		handler.CreateTimeEntry(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)
		var got tasks.TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader("not json"))
		rec := httptest.NewRecorder()
		handler.CreateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 400 when task_id is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(`{"started_at": "2026-03-01T09:00:00Z"}`))
		rec := httptest.NewRecorder()
		handler.CreateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "task_id is required")
	})

	t.Run("returns 400 when started_at is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(`{"task_id": 3}`))
		rec := httptest.NewRecorder()
		handler.CreateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "started_at is required")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(`{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`))
		rec := httptest.NewRecorder()
		handler.CreateTimeEntry(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to create time entry")
	})
}

// --- Update Tests ---

func TestHandler_UpdateProject(t *testing.T) {
	t.Run("returns 200 with updated project", func(t *testing.T) {
		now := time.Now()
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.MatchedBy(func(req tasks.UpdateProjectRequest) bool {
			return req.ID == 5 && *req.Name == "updated name"
		})).Return(tasks.ProjectResponse{ID: 5, Name: "updated name", FinishedAt: &now}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/5", strings.NewReader(`{"name": "updated name"}`))
		req = withIDParam(req, "5")
		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.ID)
		assert.Equal(t, "updated name", got.Name)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/abc", strings.NewReader(`{}`))
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 400 for invalid body", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/1", strings.NewReader("not json"))
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/999", strings.NewReader(`{}`))
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateProject(mock.Anything, mock.Anything).Return(tasks.ProjectResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/1", strings.NewReader(`{}`))
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update project")
	})
}

func TestHandler_UpdateTask(t *testing.T) {
	t.Run("returns 200 with updated task", func(t *testing.T) {
		now := time.Now()
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 7, Name: "test task", FinishedAt: &now}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7", strings.NewReader(`{"finished_at": "2026-03-01T17:00:00Z"}`))
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/abc", strings.NewReader(`{}`))
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/999", strings.NewReader(`{}`))
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("passes depends_on to service and returns it", func(t *testing.T) {
		deps := []int32{2, 3}
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
			return req.ID == 7 && req.DependsOn != nil && len(*req.DependsOn) == 2
		})).Return(tasks.TaskResponse{
			ID: 7, Name: "test task",
			DependsOn:   []tasks.TaskDepRef{{ID: 2, Name: "A"}, {ID: 3, Name: "B"}},
			Blocks: []tasks.TaskDepRef{{ID: 5, Name: "C"}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7", strings.NewReader(`{"depends_on": [2, 3]}`))
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got.DependsOn, 2)
		assert.Equal(t, int32(2), got.DependsOn[0].ID)
		require.Len(t, got.Blocks, 1)
		assert.Equal(t, int32(5), got.Blocks[0].ID)
		_ = deps
	})

	t.Run("clears depends_on with empty array", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
			return req.ID == 7 && req.DependsOn != nil && len(*req.DependsOn) == 0
		})).Return(tasks.TaskResponse{
			ID: 7, Name: "test task",
			DependsOn: []tasks.TaskDepRef{}, Blocks: []tasks.TaskDepRef{},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7", strings.NewReader(`{"depends_on": []}`))
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got.DependsOn)
	})

	t.Run("omitted depends_on does not modify dependencies", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTaskRequest) bool {
			return req.ID == 7 && req.DependsOn == nil
		})).Return(tasks.TaskResponse{
			ID: 7, Name: "updated name",
			DependsOn: []tasks.TaskDepRef{{ID: 2, Name: "A"}}, Blocks: []tasks.TaskDepRef{},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7", strings.NewReader(`{"name": "updated name"}`))
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got.DependsOn, 1)
		assert.Equal(t, int32(2), got.DependsOn[0].ID)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/1", strings.NewReader(`{}`))
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update task")
	})
}

func TestHandler_UpdateTodo(t *testing.T) {
	t.Run("returns 200 with updated todo", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.MatchedBy(func(req tasks.UpdateTodoRequest) bool {
			return req.ID == 3 && *req.IsDone == true
		})).Return(tasks.TodoResponse{ID: 3, TaskID: 5, Name: "My Todo", IsDone: true}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/3", strings.NewReader(`{"is_done": true}`))
		req = withIDParam(req, "3")
		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TodoResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(3), got.ID)
		assert.True(t, got.IsDone)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/abc", strings.NewReader(`{}`))
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid todo id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/999", strings.NewReader(`{}`))
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "todo not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTodo(mock.Anything, mock.Anything).Return(tasks.TodoResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/1", strings.NewReader(`{}`))
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update todo")
	})
}

func TestHandler_UpdateTimeEntry(t *testing.T) {
	t.Run("returns 200 with updated time entry", func(t *testing.T) {
		now := time.Now()
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{ID: 7, TaskID: 3, StartedAt: now, FinishedAt: &now}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/7", strings.NewReader(`{"finished_at": "2026-03-01T17:00:00Z"}`))
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/abc", strings.NewReader(`{}`))
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid time entry id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/999", strings.NewReader(`{}`))
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "time entry not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().UpdateTimeEntry(mock.Anything, mock.Anything).Return(tasks.TimeEntryResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/1", strings.NewReader(`{}`))
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update time entry")
	})
}

// --- Get Tests ---

func TestHandler_GetRootProjects(t *testing.T) {
	t.Run("returns 200 with projects", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetRootProjects(mock.Anything).Return([]tasks.ProjectResponse{
			{ID: 1, Name: "Project A"},
			{ID: 2, Name: "Project B"},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()
		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Len(t, got, 2)
		assert.Equal(t, "Project A", got[0].Name)
	})

	t.Run("returns 200 with empty list", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetRootProjects(mock.Anything).Return([]tasks.ProjectResponse{}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()
		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetRootProjects(mock.Anything).Return(nil, errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()
		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get projects")
	})
}

func TestHandler_GetActiveTree(t *testing.T) {
	projDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	taskDesc := "do stuff"
	taskDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	taskStarted := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	t.Run("returns 200 with tree nodes", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything).Return([]tasks.ActiveTreeNode{
			{ID: 1, Type: "project", Name: "Project A", DueAt: &projDue, Children: []tasks.ActiveTreeNode{
				{ID: 1, Type: "task", Name: "Task 1", Description: &taskDesc, DueAt: &taskDue, StartedAt: &taskStarted},
			}},
			{ID: 2, Type: "task", Name: "Orphan Task"},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.ActiveTreeNode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Len(t, got, 2)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, &projDue, got[0].DueAt)
		require.Len(t, got[0].Children, 1)
		assert.Equal(t, &taskDesc, got[0].Children[0].Description)
		assert.Equal(t, "task", got[1].Type)
		assert.Nil(t, got[1].Children)
	})

	t.Run("returns 200 with empty array", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything).Return([]tasks.ActiveTreeNode{}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.ActiveTreeNode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything).Return(nil, errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get active tree")
	})

	t.Run("includes depends_on and blocks in response", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTree(mock.Anything).Return([]tasks.ActiveTreeNode{
			{ID: 1, Type: "task", Name: "Task A",
				DependsOn:   []tasks.TaskDepRef{{ID: 3, Name: "X"}},
				Blocks: []tasks.TaskDepRef{{ID: 5, Name: "Y"}}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.ActiveTreeNode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got, 1)
		require.Len(t, got[0].DependsOn, 1)
		assert.Equal(t, int32(3), got[0].DependsOn[0].ID)
		require.Len(t, got[0].Blocks, 1)
		assert.Equal(t, int32(5), got[0].Blocks[0].ID)
	})
}

func TestHandler_GetProject(t *testing.T) {
	t.Run("returns 200 with project detail", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(5)).Return(tasks.ProjectDetailResponse{ID: 5, Name: "My Project", TimeSpent: 7200}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/5", nil)
		req = withIDParam(req, "5")
		rec := httptest.NewRecorder()
		handler.GetProject(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.ProjectDetailResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.ID)
		assert.Equal(t, int64(7200), got.TimeSpent)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.GetProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(999)).Return(tasks.ProjectDetailResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/999", nil)
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.GetProject(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProject(mock.Anything, int32(1)).Return(tasks.ProjectDetailResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.GetProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get project")
	})
}

func TestHandler_GetTask(t *testing.T) {
	t.Run("returns 200 with task and todos", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(7)).Return(tasks.TaskFullResponse{
			ID: 7, Name: "My Task", TimeSpent: 3600,
			Todos: []tasks.TodoResponse{{ID: 1, TaskID: 7, Name: "Todo 1", IsDone: true}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/7", nil)
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.GetTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskFullResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.Equal(t, int64(3600), got.TimeSpent)
		assert.Len(t, got.Todos, 1)
		assert.True(t, got.Todos[0].IsDone)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.GetTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(999)).Return(tasks.TaskFullResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/999", nil)
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.GetTask(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTask(mock.Anything, int32(1)).Return(tasks.TaskFullResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.GetTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get task")
	})
}

func TestHandler_GetProjectChildren(t *testing.T) {
	t.Run("returns 200 with project children", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProjectChildren(mock.Anything, int32(5)).Return(tasks.ProjectChildrenResponse{
			Project:  tasks.ProjectDetailResponse{ID: 5, Name: "Root"},
			Children: []tasks.ProjectChildNode{{ID: 1, Type: "task", Name: "Task 1"}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/5/children", nil)
		req = withIDParam(req, "5")
		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.ProjectChildrenResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.Project.ID)
		assert.Len(t, got.Children, 1)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/abc/children", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProjectChildren(mock.Anything, int32(999)).Return(tasks.ProjectChildrenResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/999/children", nil)
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetProjectChildren(mock.Anything, int32(1)).Return(tasks.ProjectChildrenResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/1/children", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get project children")
	})
}

func TestHandler_GetTaskTimeEntries(t *testing.T) {
	t.Run("returns 200 with task time entries", func(t *testing.T) {
		now := time.Now()
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(7)).Return(tasks.TaskTimeEntriesResponse{
			Task:        tasks.TaskDetailResponse{ID: 7, Name: "My Task", TimeSpent: 3600},
			TimeEntries: []tasks.TimeEntryResponse{{ID: 1, TaskID: 7, StartedAt: now}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/7/time-entries", nil)
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskTimeEntriesResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.Task.ID)
		assert.Equal(t, int64(3600), got.Task.TimeSpent)
		assert.Len(t, got.TimeEntries, 1)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/abc/time-entries", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(999)).Return(tasks.TaskTimeEntriesResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/999/time-entries", nil)
		req = withIDParam(req, "999")
		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(1)).Return(tasks.TaskTimeEntriesResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodGet, "/tasks/1/time-entries", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get task time entries")
	})

	t.Run("includes depends_on and blocks in task detail", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTaskTimeEntries(mock.Anything, int32(7)).Return(tasks.TaskTimeEntriesResponse{
			Task: tasks.TaskDetailResponse{
				ID: 7, Name: "My Task", TimeSpent: 3600,
				DependsOn:   []tasks.TaskDepRef{{ID: 2, Name: "A"}},
				Blocks: []tasks.TaskDepRef{{ID: 4, Name: "B"}},
			},
			TimeEntries: []tasks.TimeEntryResponse{},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/7/time-entries", nil)
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TaskTimeEntriesResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got.Task.DependsOn, 1)
		assert.Equal(t, int32(2), got.Task.DependsOn[0].ID)
		require.Len(t, got.Task.Blocks, 1)
		assert.Equal(t, int32(4), got.Task.Blocks[0].ID)
	})
}

func TestHandler_GetTasksByDueDate(t *testing.T) {
	t.Run("returns 200 with tasks", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		projectID := int32(5)
		projectName := "My Project"
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything).Return([]tasks.TaskByDueDateResponse{
			{ID: 1, Name: "Task A", DueAt: &now, TimeSpent: 3600, ProjectID: &projectID, ProjectName: &projectName},
			{ID: 2, Name: "Task B"},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/by-due-date", nil)
		rec := httptest.NewRecorder()
		handler.GetTasksByDueDate(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.TaskByDueDateResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Len(t, got, 2)
		assert.Equal(t, "Task A", got[0].Name)
		assert.Equal(t, int64(3600), got[0].TimeSpent)
		assert.Equal(t, &projectID, got[0].ProjectID)
	})

	t.Run("returns 200 with empty list", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything).Return([]tasks.TaskByDueDateResponse{}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/by-due-date", nil)
		rec := httptest.NewRecorder()
		handler.GetTasksByDueDate(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.TaskByDueDateResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything).Return(nil, errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/by-due-date", nil)
		rec := httptest.NewRecorder()
		handler.GetTasksByDueDate(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get tasks by due date")
	})

	t.Run("includes depends_on and blocks in response", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTasksByDueDate(mock.Anything).Return([]tasks.TaskByDueDateResponse{
			{ID: 1, Name: "Task A",
				DependsOn:   []tasks.TaskDepRef{{ID: 2, Name: "X"}},
				Blocks: []tasks.TaskDepRef{{ID: 3, Name: "Y"}, {ID: 4, Name: "Z"}}},
		}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tasks/by-due-date", nil)
		rec := httptest.NewRecorder()
		handler.GetTasksByDueDate(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got []tasks.TaskByDueDateResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		require.Len(t, got, 1)
		require.Len(t, got[0].DependsOn, 1)
		assert.Equal(t, int32(2), got[0].DependsOn[0].ID)
		require.Len(t, got[0].Blocks, 2)
	})
}

// --- Delete Tests ---

func TestHandler_DeleteProject(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteProject(mock.Anything, int32(5)).Return(nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodDelete, "/tasks/projects/5", nil)
		req = withIDParam(req, "5")
		rec := httptest.NewRecorder()
		handler.DeleteProject(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/projects/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.DeleteProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteProject(mock.Anything, int32(1)).Return(errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodDelete, "/tasks/projects/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.DeleteProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to delete project")
	})
}

func TestHandler_DeleteTask(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTask(mock.Anything, int32(3)).Return(nil)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/tasks/3", nil)
		req = withIDParam(req, "3")
		rec := httptest.NewRecorder()
		handler.DeleteTask(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/tasks/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.DeleteTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTask(mock.Anything, int32(1)).Return(errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/tasks/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.DeleteTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to delete task")
	})
}

func TestHandler_DeleteTodo(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTodo(mock.Anything, int32(7)).Return(nil)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/todos/7", nil)
		req = withIDParam(req, "7")
		rec := httptest.NewRecorder()
		handler.DeleteTodo(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/todos/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.DeleteTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTodo(mock.Anything, int32(1)).Return(errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/todos/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.DeleteTodo(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to delete todo")
	})
}

func TestHandler_DeleteTimeEntry(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTimeEntry(mock.Anything, int32(9)).Return(nil)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/time-entries/9", nil)
		req = withIDParam(req, "9")
		rec := httptest.NewRecorder()
		handler.DeleteTimeEntry(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("returns 400 for invalid ID", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/time-entries/abc", nil)
		req = withIDParam(req, "abc")
		rec := httptest.NewRecorder()
		handler.DeleteTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteTimeEntry(mock.Anything, int32(1)).Return(errors.New("db error"))
		handler := tasks.NewHandler(svc)
		req := httptest.NewRequest(http.MethodDelete, "/tasks/time-entries/1", nil)
		req = withIDParam(req, "1")
		rec := httptest.NewRecorder()
		handler.DeleteTimeEntry(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to delete time entry")
	})
}

// --- Active Time Entry ---

func TestHandler_GetActiveTimeEntry(t *testing.T) {
	now := time.Now()
	comment := "working on feature"

	t.Run("returns 200 with active time entry", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.TimeEntryResponse{ID: 1, TaskID: 5, StartedAt: now, Comment: &comment}, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/active", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTimeEntry(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got tasks.TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(5), got.TaskID)
		assert.Equal(t, &comment, got.Comment)
	})

	t.Run("returns 404 when no active time entry", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.TimeEntryResponse{}, tasks.ErrNotFound)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/active", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTimeEntry(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "no active time entry")
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetActiveTimeEntry(mock.Anything).Return(tasks.TimeEntryResponse{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/active", nil)
		rec := httptest.NewRecorder()
		handler.GetActiveTimeEntry(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get active time entry")
	})
}

func TestHandler_GetTimeEntryHistory(t *testing.T) {
	t.Run("returns 400 when frequency is missing", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/history", nil)
		rec := httptest.NewRecorder()
		handler.GetTimeEntryHistory(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "frequency is required")
	})

	t.Run("returns 400 when frequency is invalid", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/history?frequency=yearly", nil)
		rec := httptest.NewRecorder()
		handler.GetTimeEntryHistory(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "frequency must be")
	})

	t.Run("returns 200 with history data", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		resp := history.Response{
			StartAt: "2026-03-01",
			EndAt:   "2026-03-19",
			Data: []history.Point{
				{Date: "2026-03-01", Value: 1.5},
				{Date: "2026-03-02", Value: 2.0},
			},
		}
		svc.EXPECT().GetTimeEntryHistory(mock.Anything, "daily", "2026-03-01", "2026-03-19").Return(resp, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/history?frequency=daily&start_at=2026-03-01&end_at=2026-03-19", nil)
		rec := httptest.NewRecorder()
		handler.GetTimeEntryHistory(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got history.Response
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, "2026-03-01", got.StartAt)
		assert.Equal(t, "2026-03-19", got.EndAt)
		require.Len(t, got.Data, 2)
		assert.Equal(t, float32(1.5), got.Data[0].Value)
	})

	t.Run("returns 200 with defaults when no dates", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		resp := history.Response{
			StartAt: "2026-01-05",
			EndAt:   "2026-03-23",
			Data:    []history.Point{},
		}
		svc.EXPECT().GetTimeEntryHistory(mock.Anything, "weekly", "", "").Return(resp, nil)
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/history?frequency=weekly", nil)
		rec := httptest.NewRecorder()
		handler.GetTimeEntryHistory(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().GetTimeEntryHistory(mock.Anything, "daily", "", "").Return(history.Response{}, errors.New("db error"))
		handler := tasks.NewHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/tasks/time-entries/history?frequency=daily", nil)
		rec := httptest.NewRecorder()
		handler.GetTimeEntryHistory(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get time entry history")
	})
}
