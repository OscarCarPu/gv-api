package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockService struct {
	createProjectFn   func(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error)
	createTaskFn      func(ctx context.Context, req CreateTaskRequest) (CreateTaskResponse, error)
	createTodoFn      func(ctx context.Context, req CreateTodoRequest) (CreateTodoResponse, error)
	createTimeEntryFn func(ctx context.Context, req CreateTimeEntryRequest) (CreateTimeEntryResponse, error)
	getRootProjectsFn func(ctx context.Context) ([]ProjectResponse, error)
}

func (m *mockService) CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, req)
	}
	return ProjectResponse{}, nil
}

func (m *mockService) CreateTask(ctx context.Context, req CreateTaskRequest) (CreateTaskResponse, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, req)
	}
	return CreateTaskResponse{}, nil
}

func (m *mockService) CreateTodo(ctx context.Context, req CreateTodoRequest) (CreateTodoResponse, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, req)
	}
	return CreateTodoResponse{}, nil
}

func (m *mockService) CreateTimeEntry(ctx context.Context, req CreateTimeEntryRequest) (CreateTimeEntryResponse, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, req)
	}
	return CreateTimeEntryResponse{}, nil
}

func (m *mockService) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
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
			createTaskFn: func(ctx context.Context, req CreateTaskRequest) (CreateTaskResponse, error) {
				return CreateTaskResponse{ID: 1, ProjectID: req.ProjectID, Name: req.Name}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"project_id": 1, "name": "My Task"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/tasks", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTask(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got CreateTaskResponse
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
					createTaskFn: func(ctx context.Context, req CreateTaskRequest) (CreateTaskResponse, error) {
						return CreateTaskResponse{}, errors.New("db error")
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
			createTodoFn: func(ctx context.Context, req CreateTodoRequest) (CreateTodoResponse, error) {
				return CreateTodoResponse{ID: 1, TaskID: req.TaskID, Name: req.Name}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"task_id": 5, "name": "My Todo"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/todos", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTodo(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got CreateTodoResponse
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
					createTodoFn: func(ctx context.Context, req CreateTodoRequest) (CreateTodoResponse, error) {
						return CreateTodoResponse{}, errors.New("db error")
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
			createTimeEntryFn: func(ctx context.Context, req CreateTimeEntryRequest) (CreateTimeEntryResponse, error) {
				return CreateTimeEntryResponse{ID: 1, TaskID: req.TaskID, StartedAt: req.StartedAt}, nil
			},
		}
		handler := NewHandler(mock)

		body := `{"task_id": 3, "started_at": "2026-03-01T09:00:00Z"}`
		req := httptest.NewRequest(http.MethodPost, "/tasks/time-entries", strings.NewReader(body))
		rec := httptest.NewRecorder()

		handler.CreateTimeEntry(rec, req)

		assert.Equal(t, http.StatusCreated, rec.Code)

		var got CreateTimeEntryResponse
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
					createTimeEntryFn: func(ctx context.Context, req CreateTimeEntryRequest) (CreateTimeEntryResponse, error) {
						return CreateTimeEntryResponse{}, errors.New("db error")
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
