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
	finishTimeEntryFn func(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error)
	finishTaskFn      func(ctx context.Context, req FinishTaskRequest) (TaskResponse, error)
	finishProjectFn   func(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error)
	getRootProjectsFn    func(ctx context.Context) ([]ProjectResponse, error)
	getActiveTreeFn      func(ctx context.Context) ([]ActiveTreeNode, error)
	getProjectChildrenFn func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
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

func (m *mockService) FinishTimeEntry(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error) {
	if m.finishTimeEntryFn != nil {
		return m.finishTimeEntryFn(ctx, req)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockService) FinishTask(ctx context.Context, req FinishTaskRequest) (TaskResponse, error) {
	if m.finishTaskFn != nil {
		return m.finishTaskFn(ctx, req)
	}
	return TaskResponse{}, nil
}

func (m *mockService) FinishProject(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error) {
	if m.finishProjectFn != nil {
		return m.finishProjectFn(ctx, req)
	}
	return ProjectResponse{}, nil
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

func TestHandler_FinishTask(t *testing.T) {
	t.Run("returns 200 with finished task", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			finishTaskFn: func(ctx context.Context, req FinishTaskRequest) (TaskResponse, error) {
				return TaskResponse{ID: req.ID, Name: "test task", FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/7/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "7")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTask(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got TaskResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/abc/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTask(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid task id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			finishTaskFn: func(ctx context.Context, req FinishTaskRequest) (TaskResponse, error) {
				return TaskResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/999/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTask(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "task not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			finishTaskFn: func(ctx context.Context, req FinishTaskRequest) (TaskResponse, error) {
				return TaskResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/tasks/1/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTask(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to finish task")
	})
}

func TestHandler_FinishProject(t *testing.T) {
	t.Run("returns 200 with finished project", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			finishProjectFn: func(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{ID: req.ID, Name: "test project", FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/5/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "5")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishProject(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got ProjectResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(5), got.ID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/abc/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishProject(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid project id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			finishProjectFn: func(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/999/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishProject(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "project not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			finishProjectFn: func(ctx context.Context, req FinishProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/projects/1/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishProject(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to finish project")
	})
}

func TestHandler_FinishTimeEntry(t *testing.T) {
	t.Run("returns 200 with finished time entry", func(t *testing.T) {
		now := time.Now()
		mock := &mockService{
			finishTimeEntryFn: func(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{ID: req.ID, TaskID: 3, StartedAt: now, FinishedAt: &now}, nil
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/7/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "7")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTimeEntry(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var got TimeEntryResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
		assert.Equal(t, int32(7), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
		assert.NotNil(t, got.FinishedAt)
	})

	t.Run("returns 400 for invalid id", func(t *testing.T) {
		handler := NewHandler(&mockService{})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/abc/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "abc")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTimeEntry(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid time entry id")
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		mock := &mockService{
			finishTimeEntryFn: func(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, ErrNotFound
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/999/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "999")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTimeEntry(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Contains(t, rec.Body.String(), "time entry not found")
	})

	t.Run("returns 500 when service fails", func(t *testing.T) {
		mock := &mockService{
			finishTimeEntryFn: func(ctx context.Context, req FinishTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, errors.New("db error")
			},
		}
		handler := NewHandler(mock)

		req := httptest.NewRequest(http.MethodPatch, "/tasks/time-entries/1/finish", nil)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.FinishTimeEntry(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, rec.Body.String(), "Failed to finish time entry")
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
