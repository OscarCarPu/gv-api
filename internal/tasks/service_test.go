package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	createProjectFn   func(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	createTaskFn      func(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (CreateTaskResponse, error)
	createTodoFn      func(ctx context.Context, taskID int32, name string) (CreateTodoResponse, error)
	createTimeEntryFn func(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (CreateTimeEntryResponse, error)
	getRootProjectsFn func(ctx context.Context) ([]ProjectResponse, error)
}

func (m *mockRepo) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, name, description, dueAt, parentID)
	}
	return ProjectResponse{}, nil
}

func (m *mockRepo) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (CreateTaskResponse, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, projectID, name, description, dueAt)
	}
	return CreateTaskResponse{}, nil
}

func (m *mockRepo) CreateTodo(ctx context.Context, taskID int32, name string) (CreateTodoResponse, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, taskID, name)
	}
	return CreateTodoResponse{}, nil
}

func (m *mockRepo) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (CreateTimeEntryResponse, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, taskID, startedAt, finishedAt, comment)
	}
	return CreateTimeEntryResponse{}, nil
}

func (m *mockRepo) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
}

func TestService_CreateProject(t *testing.T) {
	now := time.Now()
	desc := "test description"
	desc2 := "read more books"
	parentID1 := int32(1)
	parentID5 := int32(5)

	tests := []struct {
		name    string
		req     CreateProjectRequest
		mockRes ProjectResponse
		mockErr error
		wantRes ProjectResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateProjectRequest{Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
			mockRes: ProjectResponse{ID: 1, Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
			wantRes: ProjectResponse{ID: 1, Name: "Exercise", Description: &desc, DueAt: &now, ParentID: &parentID1},
		},
		{
			name:    "name only",
			req:     CreateProjectRequest{Name: "Minimal"},
			mockRes: ProjectResponse{ID: 2, Name: "Minimal"},
			wantRes: ProjectResponse{ID: 2, Name: "Minimal"},
		},
		{
			name:    "with description no due date",
			req:     CreateProjectRequest{Name: "Reading", Description: &desc2},
			mockRes: ProjectResponse{ID: 3, Name: "Reading", Description: &desc2},
			wantRes: ProjectResponse{ID: 3, Name: "Reading", Description: &desc2},
		},
		{
			name:    "with parent ID only",
			req:     CreateProjectRequest{Name: "Sub-project", ParentID: &parentID5},
			mockRes: ProjectResponse{ID: 4, Name: "Sub-project", ParentID: &parentID5},
			wantRes: ProjectResponse{ID: 4, Name: "Sub-project", ParentID: &parentID5},
		},
		{
			name:    "repository error",
			req:     CreateProjectRequest{Name: "Fail"},
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				createProjectFn: func(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.CreateProject(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes.ID, got.ID)
			assert.Equal(t, tt.wantRes.Name, got.Name)
		})
	}
}

func TestService_CreateTask(t *testing.T) {
	now := time.Now()
	desc := "task description"
	projectID := int32(1)

	tests := []struct {
		name    string
		req     CreateTaskRequest
		mockRes CreateTaskResponse
		mockErr error
		wantRes CreateTaskResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateTaskRequest{ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
			mockRes: CreateTaskResponse{ID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
			wantRes: CreateTaskResponse{ID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
		},
		{
			name:    "name only",
			req:     CreateTaskRequest{Name: "Minimal Task"},
			mockRes: CreateTaskResponse{ID: 2, Name: "Minimal Task"},
			wantRes: CreateTaskResponse{ID: 2, Name: "Minimal Task"},
		},
		{
			name:    "repository error",
			req:     CreateTaskRequest{Name: "Fail"},
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				createTaskFn: func(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (CreateTaskResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.CreateTask(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes.ID, got.ID)
			assert.Equal(t, tt.wantRes.Name, got.Name)
		})
	}
}

func TestService_CreateTodo(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateTodoRequest
		mockRes CreateTodoResponse
		mockErr error
		wantRes CreateTodoResponse
		wantErr bool
	}{
		{
			name:    "success",
			req:     CreateTodoRequest{TaskID: 1, Name: "My Todo"},
			mockRes: CreateTodoResponse{ID: 1, TaskID: 1, Name: "My Todo"},
			wantRes: CreateTodoResponse{ID: 1, TaskID: 1, Name: "My Todo"},
		},
		{
			name:    "repository error",
			req:     CreateTodoRequest{TaskID: 1, Name: "Fail"},
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				createTodoFn: func(ctx context.Context, taskID int32, name string) (CreateTodoResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.CreateTodo(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes.ID, got.ID)
			assert.Equal(t, tt.wantRes.Name, got.Name)
		})
	}
}

func TestService_CreateTimeEntry(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	comment := "worked on feature"

	tests := []struct {
		name    string
		req     CreateTimeEntryRequest
		mockRes CreateTimeEntryResponse
		mockErr error
		wantRes CreateTimeEntryResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateTimeEntryRequest{TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
			mockRes: CreateTimeEntryResponse{ID: 1, TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
			wantRes: CreateTimeEntryResponse{ID: 1, TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
		},
		{
			name:    "required fields only",
			req:     CreateTimeEntryRequest{TaskID: 1, StartedAt: now},
			mockRes: CreateTimeEntryResponse{ID: 2, TaskID: 1, StartedAt: now},
			wantRes: CreateTimeEntryResponse{ID: 2, TaskID: 1, StartedAt: now},
		},
		{
			name:    "repository error",
			req:     CreateTimeEntryRequest{TaskID: 1, StartedAt: now},
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				createTimeEntryFn: func(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (CreateTimeEntryResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.CreateTimeEntry(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRes.ID, got.ID)
			assert.Equal(t, tt.wantRes.TaskID, got.TaskID)
		})
	}
}

func TestService_GetRootProjects(t *testing.T) {
	tests := []struct {
		name    string
		mockRes []ProjectResponse
		mockErr error
		wantLen int
		wantErr bool
	}{
		{
			name: "returns list",
			mockRes: []ProjectResponse{
				{ID: 1, Name: "Project A"},
				{ID: 2, Name: "Project B"},
			},
			wantLen: 2,
		},
		{
			name:    "returns empty list",
			mockRes: []ProjectResponse{},
			wantLen: 0,
		},
		{
			name:    "repository error",
			mockErr: errors.New("db error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockRepo{
				getRootProjectsFn: func(ctx context.Context) ([]ProjectResponse, error) {
					return tt.mockRes, tt.mockErr
				},
			}

			svc := NewService(mock, nil)
			got, err := svc.GetRootProjects(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)
		})
	}
}
