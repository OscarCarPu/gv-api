package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockService struct {
	createProjectFn   func(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error)
	createTaskFn      func(ctx context.Context, req CreateTaskRequest) (TaskResponse, error)
	createTodoFn      func(ctx context.Context, req CreateTodoRequest) (TodoResponse, error)
	createTimeEntryFn func(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error)
	updateProjectFn   func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error)
	updateTaskFn      func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error)
	updateTodoFn      func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error)
	updateTimeEntryFn func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error)
	getRootProjectsFn    func(ctx context.Context) ([]ProjectResponse, error)
	getActiveTreeFn      func(ctx context.Context) ([]ActiveTreeNode, error)
	getProjectChildrenFn   func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	getTaskTimeEntriesFn   func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
}

func (m *mockService) CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, req)
	}
	return ProjectResponse{}, nil
}

func (m *mockService) CreateTask(ctx context.Context, req CreateTaskRequest) (TaskResponse, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, req)
	}
	return TaskResponse{}, nil
}

func (m *mockService) CreateTodo(ctx context.Context, req CreateTodoRequest) (TodoResponse, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, req)
	}
	return TodoResponse{}, nil
}

func (m *mockService) CreateTimeEntry(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, req)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockService) UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, req)
	}
	return ProjectResponse{}, nil
}

func (m *mockService) UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, req)
	}
	return TaskResponse{}, nil
}

func (m *mockService) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	if m.updateTodoFn != nil {
		return m.updateTodoFn(ctx, req)
	}
	return TodoResponse{}, nil
}

func (m *mockService) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	if m.updateTimeEntryFn != nil {
		return m.updateTimeEntryFn(ctx, req)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockService) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockService) GetActiveTree(ctx context.Context) ([]ActiveTreeNode, error) {
	if m.getActiveTreeFn != nil {
		return m.getActiveTreeFn(ctx)
	}
	return nil, nil
}

func (m *mockService) GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
	if m.getProjectChildrenFn != nil {
		return m.getProjectChildrenFn(ctx, projectID)
	}
	return ProjectChildrenResponse{}, nil
}

func (m *mockService) GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
	if m.getTaskTimeEntriesFn != nil {
		return m.getTaskTimeEntriesFn(ctx, taskID)
	}
	return TaskTimeEntriesResponse{}, nil
}

// --- Handler Tests ---

func TestHandler_CreateProject(t *testing.T) {
	t.Run("returns 201 with created project", func(t *testing.T) {
		desc := "Test description"
		mock := &mockService{
			createProjectFn: func(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{ID: 1, Name: req.Name, Description: req.Description}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"name": "My Project", "description": "Test description"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateProject(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "My Project", got.Name)
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
			body: `{"name": "My Project"}`,
			setupMock: func() *mockService {
				return &mockService{
					createProjectFn: func(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error) {
						return ProjectResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create project",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/tasks/projects", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateProject(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_CreateTask(t *testing.T) {
	t.Run("returns 201 with created task", func(t *testing.T) {
		projectID := int32(1)
		mock := &mockService{
			createTaskFn: func(ctx context.Context, req CreateTaskRequest) (TaskResponse, error) {
				return TaskResponse{ID: 1, ProjectID: req.ProjectID, Name: req.Name}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"project_id": 1, "name": "My Task"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "My Task", got.Name)
		assert.Equal(t, &projectID, got.ProjectID)
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
			body:       `{"project_id": 1}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
		{
			name: "returns 500 when service fails",
			body: `{"name": "My Task"}`,
			setupMock: func() *mockService {
				return &mockService{
					createTaskFn: func(ctx context.Context, req CreateTaskRequest) (TaskResponse, error) {
						return TaskResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create task",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateTask(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_CreateTodo(t *testing.T) {
	t.Run("returns 201 with created todo", func(t *testing.T) {
		mock := &mockService{
			createTodoFn: func(ctx context.Context, req CreateTodoRequest) (TodoResponse, error) {
				return TodoResponse{ID: 1, TaskID: req.TaskID, Name: req.Name}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"task_id": 5, "name": "My Todo"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTodo(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got TodoResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(5), got.TaskID)
		assert.Equal(t, "My Todo", got.Name)
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
			name:       "returns 400 when task_id is missing",
			body:       `{"name": "My Todo"}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "task_id is required",
		},
		{
			name:       "returns 400 when name is missing",
			body:       `{"task_id": 5}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "name is required",
		},
		{
			name: "returns 500 when service fails",
			body: `{"task_id": 5, "name": "My Todo"}`,
			setupMock: func() *mockService {
				return &mockService{
					createTodoFn: func(ctx context.Context, req CreateTodoRequest) (TodoResponse, error) {
						return TodoResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create todo",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateTodo(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_CreateTimeEntry(t *testing.T) {
	t.Run("returns 201 with created time entry", func(t *testing.T) {
		mock := &mockService{
			createTimeEntryFn: func(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{ID: 1, TaskID: req.TaskID, StartedAt: req.StartedAt}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTimeEntry(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
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
			name:       "returns 400 when task_id is missing",
			body:       `{"started_at": "2026-03-01T09:00:00Z"}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "task_id is required",
		},
		{
			name:       "returns 400 when started_at is missing",
			body:       `{"task_id": 3}`,
			setupMock:  func() *mockService { return &mockService{} },
			wantStatus: http.StatusBadRequest,
			wantBody:   "started_at is required",
		},
		{
			name: "returns 500 when service fails",
			body: `{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`,
			setupMock: func() *mockService {
				return &mockService{
					createTimeEntryFn: func(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error) {
						return TimeEntryResponse{}, errors.New("db error")
					},
				}
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to create time entry",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(tc.setupMock())
			req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()

			handler.CreateTimeEntry(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tc.wantBody)
		})
	}
}

func TestHandler_UpdateProject(t *testing.T) {
	t.Run("returns 200 with updated project", func(t *testing.T) {
		now := time.Now()
		name := "updated name"
		mock := &mockService{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				assert.Equal(t, int32(5), req.ID)
				assert.Equal(t, &name, req.Name)
				return ProjectResponse{ID: 5, Name: name, FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"name": "updated name"}`
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/5", strings.NewReader(body))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "5")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.ID)
		assert.Equal(t, "updated name", got.Name)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/abc", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 400 for invalid body", func(t *testing.T) {
		handler := NewHandler(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/1", strings.NewReader("not json"))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "Invalid Body")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/999", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/1", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateProject(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update project")
	})
}

func TestHandler_UpdateTask(t *testing.T) {
	t.Run("returns 200 with updated task", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			updateTaskFn: func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
				return TaskResponse{ID: req.ID, Name: "test task", FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7", strings.NewReader(`{"finished_at": "2026-03-01T17:00:00Z"}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "7")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/abc", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			updateTaskFn: func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
				return TaskResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/999", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			updateTaskFn: func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
				return TaskResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/1", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTask(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update task")
	})
}

func TestHandler_UpdateTodo(t *testing.T) {
	t.Run("returns 200 with updated todo", func(t *testing.T) {
		isDone := true
		mock := &mockService{
			updateTodoFn: func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
				assert.Equal(t, int32(3), req.ID)
				assert.Equal(t, &isDone, req.IsDone)
				return TodoResponse{ID: 3, TaskID: 5, Name: "My Todo", IsDone: true}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/3", strings.NewReader(`{"is_done": true}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "3")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got TodoResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(3), got.ID)
		assert.True(t, got.IsDone)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/abc", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid todo id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			updateTodoFn: func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
				return TodoResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/999", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "todo not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			updateTodoFn: func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
				return TodoResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/todos/1", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTodo(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update todo")
	})
}

func TestHandler_UpdateTimeEntry(t *testing.T) {
	t.Run("returns 200 with updated time entry", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			updateTimeEntryFn: func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{ID: req.ID, TaskID: 3, StartedAt: now, FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/7", strings.NewReader(`{"finished_at": "2026-03-01T17:00:00Z"}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "7")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var got TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/abc", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid time entry id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			updateTimeEntryFn: func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/999", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "time entry not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			updateTimeEntryFn: func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)
		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/1", strings.NewReader(`{}`))
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.UpdateTimeEntry(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to update time entry")
	})
}

func TestHandler_GetRootProjects(t *testing.T) {
	t.Run("returns 200 with projects", func(t *testing.T) {
		mock := &mockService{
			getRootProjectsFn: func(ctx context.Context) ([]ProjectResponse, error) {
				return []ProjectResponse{
					{ID: 1, Name: "Project A"},
					{ID: 2, Name: "Project B"},
				}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()

		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got []ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Len(t, got, 2)
		assert.Equal(t, "Project A", got[0].Name)
		assert.Equal(t, "Project B", got[1].Name)
	})

	t.Run("returns 200 with empty list", func(t *testing.T) {
		mock := &mockService{
			getRootProjectsFn: func(ctx context.Context) ([]ProjectResponse, error) {
				return []ProjectResponse{}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()

		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got []ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			getRootProjectsFn: func(ctx context.Context) ([]ProjectResponse, error) {
				return nil, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects", nil)
		rec := httptest.NewRecorder()

		handler.GetRootProjects(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get projects")
	})
}

func TestHandler_GetActiveTree(t *testing.T) {
	t.Run("returns 200 with tree nodes", func(t *testing.T) {
		mock := &mockService{
			getActiveTreeFn: func(ctx context.Context) ([]ActiveTreeNode, error) {
				return []ActiveTreeNode{
					{ID: 1, Type: "project", Name: "Project A", Children: []ActiveTreeNode{
						{ID: 1, Type: "task", Name: "Task 1"},
					}},
					{ID: 2, Type: "task", Name: "Orphan Task"},
				}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()

		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got []ActiveTreeNode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Len(t, got, 2)
		assert.Equal(t, "project", got[0].Type)
		assert.Len(t, got[0].Children, 1)
		assert.Equal(t, "task", got[1].Type)
		assert.Nil(t, got[1].Children)
	})

	t.Run("returns 200 with empty array", func(t *testing.T) {
		mock := &mockService{
			getActiveTreeFn: func(ctx context.Context) ([]ActiveTreeNode, error) {
				return []ActiveTreeNode{}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()

		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got []ActiveTreeNode
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got)
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			getActiveTreeFn: func(ctx context.Context) ([]ActiveTreeNode, error) {
				return nil, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/tree", nil)
		rec := httptest.NewRecorder()

		handler.GetActiveTree(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get active tree")
	})
}

func TestHandler_GetProjectChildren(t *testing.T) {
	t.Run("returns 200 with project children", func(t *testing.T) {
		mock := &mockService{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				return ProjectChildrenResponse{
					Project:  ProjectDetailResponse{ID: projectID, Name: "Root"},
					Children: []ProjectChildNode{{ID: 1, Type: "task", Name: "Task 1"}},
				}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/5/children", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "5")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got ProjectChildrenResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.Project.ID)
		assert.Len(t, got.Children, 1)
	})

	t.Run("returns 200 with empty children", func(t *testing.T) {
		mock := &mockService{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				return ProjectChildrenResponse{
					Project:  ProjectDetailResponse{ID: 1, Name: "Empty"},
					Children: []ProjectChildNode{},
				}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/1/children", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got ProjectChildrenResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Empty(t, got.Children)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/abc/children", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				return ProjectChildrenResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/999/children", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				return ProjectChildrenResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/projects/1/children", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetProjectChildren(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get project children")
	})
}

func TestHandler_GetTaskTimeEntries(t *testing.T) {
	t.Run("returns 200 with task time entries", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			getTaskTimeEntriesFn: func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
				return TaskTimeEntriesResponse{
					Task: TaskDetailResponse{ID: taskID, Name: "My Task", TimeSpent: 3600},
					TimeEntries: []TimeEntryResponse{
						{ID: 1, TaskID: taskID, StartedAt: now},
					},
				}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/7/time-entries", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "7")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got TaskTimeEntriesResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.Task.ID)
		assert.Equal(t, int64(3600), got.Task.TimeSpent)
		assert.Len(t, got.TimeEntries, 1)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodGet, "/tasks/abc/time-entries", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			getTaskTimeEntriesFn: func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
				return TaskTimeEntriesResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/999/time-entries", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			getTaskTimeEntriesFn: func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
				return TaskTimeEntriesResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodGet, "/tasks/1/time-entries", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.GetTaskTimeEntries(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to get task time entries")
	})
}
