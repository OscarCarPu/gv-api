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
	createTaskFn      func(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error)
	createTodoFn      func(ctx context.Context, taskID int32, name string) (TodoResponse, error)
	createTimeEntryFn func(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error)
	updateProjectFn   func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error)
	updateTaskFn      func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error)
	updateTodoFn      func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error)
	updateTimeEntryFn func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error)
	getRootProjectsFn      func(ctx context.Context) ([]ProjectResponse, error)
	getActiveProjectsFn    func(ctx context.Context) ([]ActiveProject, error)
	getUnfinishedTasksFn   func(ctx context.Context) ([]UnfinishedTask, error)
	getProjectFn           func(ctx context.Context, id int32) (ProjectDetailResponse, error)
	getTaskFn              func(ctx context.Context, id int32) (TaskFullResponse, error)
	getProjectChildrenFn   func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	getTaskTimeEntriesFn   func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
	getTasksByDueDateFn    func(ctx context.Context) ([]TaskByDueDateResponse, error)
	finishDescendantProjectsFn func(ctx context.Context, projectID int32) error
	finishTasksByProjectTreeFn func(ctx context.Context, projectID int32) error
	deleteProjectFn            func(ctx context.Context, id int32) error
	deleteTaskFn               func(ctx context.Context, id int32) error
	deleteTodoFn               func(ctx context.Context, id int32) error
	deleteTimeEntryFn          func(ctx context.Context, id int32) error
}

func (m *mockRepo) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, name, description, dueAt, parentID)
	}
	return ProjectResponse{}, nil
}

func (m *mockRepo) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, projectID, name, description, dueAt)
	}
	return TaskResponse{}, nil
}

func (m *mockRepo) CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, taskID, name)
	}
	return TodoResponse{}, nil
}

func (m *mockRepo) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, taskID, startedAt, finishedAt, comment)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockRepo) UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, req)
	}
	return ProjectResponse{}, nil
}

func (m *mockRepo) UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, req)
	}
	return TaskResponse{}, nil
}

func (m *mockRepo) UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
	if m.updateTodoFn != nil {
		return m.updateTodoFn(ctx, req)
	}
	return TodoResponse{}, nil
}

func (m *mockRepo) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	if m.updateTimeEntryFn != nil {
		return m.updateTimeEntryFn(ctx, req)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockRepo) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) GetActiveProjects(ctx context.Context) ([]ActiveProject, error) {
	if m.getActiveProjectsFn != nil {
		return m.getActiveProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) GetUnfinishedTasks(ctx context.Context) ([]UnfinishedTask, error) {
	if m.getUnfinishedTasksFn != nil {
		return m.getUnfinishedTasksFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error) {
	if m.getProjectFn != nil {
		return m.getProjectFn(ctx, id)
	}
	return ProjectDetailResponse{}, nil
}

func (m *mockRepo) GetTask(ctx context.Context, id int32) (TaskFullResponse, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(ctx, id)
	}
	return TaskFullResponse{}, nil
}

func (m *mockRepo) GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
	if m.getProjectChildrenFn != nil {
		return m.getProjectChildrenFn(ctx, projectID)
	}
	return ProjectChildrenResponse{}, nil
}

func (m *mockRepo) GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
	if m.getTaskTimeEntriesFn != nil {
		return m.getTaskTimeEntriesFn(ctx, taskID)
	}
	return TaskTimeEntriesResponse{}, nil
}

func (m *mockRepo) GetTasksByDueDate(ctx context.Context) ([]TaskByDueDateResponse, error) {
	if m.getTasksByDueDateFn != nil {
		return m.getTasksByDueDateFn(ctx)
	}
	return nil, nil
}

func (m *mockRepo) FinishDescendantProjects(ctx context.Context, projectID int32) error {
	if m.finishDescendantProjectsFn != nil {
		return m.finishDescendantProjectsFn(ctx, projectID)
	}
	return nil
}

func (m *mockRepo) FinishTasksByProjectTree(ctx context.Context, projectID int32) error {
	if m.finishTasksByProjectTreeFn != nil {
		return m.finishTasksByProjectTreeFn(ctx, projectID)
	}
	return nil
}

func (m *mockRepo) DeleteProject(ctx context.Context, id int32) error {
	if m.deleteProjectFn != nil {
		return m.deleteProjectFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) DeleteTask(ctx context.Context, id int32) error {
	if m.deleteTaskFn != nil {
		return m.deleteTaskFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) DeleteTodo(ctx context.Context, id int32) error {
	if m.deleteTodoFn != nil {
		return m.deleteTodoFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) DeleteTimeEntry(ctx context.Context, id int32) error {
	if m.deleteTimeEntryFn != nil {
		return m.deleteTimeEntryFn(ctx, id)
	}
	return nil
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
		mockRes TaskResponse
		mockErr error
		wantRes TaskResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateTaskRequest{ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
			mockRes: TaskResponse{ID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
			wantRes: TaskResponse{ID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc, DueAt: &now},
		},
		{
			name:    "name only",
			req:     CreateTaskRequest{Name: "Minimal Task"},
			mockRes: TaskResponse{ID: 2, Name: "Minimal Task"},
			wantRes: TaskResponse{ID: 2, Name: "Minimal Task"},
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
				createTaskFn: func(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error) {
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
		mockRes TodoResponse
		mockErr error
		wantRes TodoResponse
		wantErr bool
	}{
		{
			name:    "success",
			req:     CreateTodoRequest{TaskID: 1, Name: "My Todo"},
			mockRes: TodoResponse{ID: 1, TaskID: 1, Name: "My Todo"},
			wantRes: TodoResponse{ID: 1, TaskID: 1, Name: "My Todo"},
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
				createTodoFn: func(ctx context.Context, taskID int32, name string) (TodoResponse, error) {
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
		mockRes TimeEntryResponse
		mockErr error
		wantRes TimeEntryResponse
		wantErr bool
	}{
		{
			name:    "all fields provided",
			req:     CreateTimeEntryRequest{TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
			mockRes: TimeEntryResponse{ID: 1, TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
			wantRes: TimeEntryResponse{ID: 1, TaskID: 1, StartedAt: now, FinishedAt: &later, Comment: &comment},
		},
		{
			name:    "required fields only",
			req:     CreateTimeEntryRequest{TaskID: 1, StartedAt: now},
			mockRes: TimeEntryResponse{ID: 2, TaskID: 1, StartedAt: now},
			wantRes: TimeEntryResponse{ID: 2, TaskID: 1, StartedAt: now},
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
				createTimeEntryFn: func(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
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

func TestService_UpdateProject(t *testing.T) {
	now := time.Now()
	name := "updated"

	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				assert.Equal(t, int32(1), req.ID)
				assert.Equal(t, &name, req.Name)
				return ProjectResponse{ID: 1, Name: name, FinishedAt: &now}, nil
			},
		}
		svc := NewService(mock, nil)
		got, err := svc.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1, Name: &name})
		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, name, got.Name)
	})

	t.Run("cascades finish to descendants when finished_at is set", func(t *testing.T) {
		var finishProjectsCalled, finishTasksCalled bool
		mock := &mockRepo{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{ID: 1, Name: name, FinishedAt: req.FinishedAt}, nil
			},
			finishDescendantProjectsFn: func(ctx context.Context, projectID int32) error {
				assert.Equal(t, int32(1), projectID)
				finishProjectsCalled = true
				return nil
			},
			finishTasksByProjectTreeFn: func(ctx context.Context, projectID int32) error {
				assert.Equal(t, int32(1), projectID)
				finishTasksCalled = true
				return nil
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1, FinishedAt: &now})
		require.NoError(t, err)
		assert.True(t, finishProjectsCalled)
		assert.True(t, finishTasksCalled)
	})

	t.Run("does not cascade when finished_at is not set", func(t *testing.T) {
		var finishProjectsCalled bool
		mock := &mockRepo{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{ID: 1, Name: name}, nil
			},
			finishDescendantProjectsFn: func(ctx context.Context, projectID int32) error {
				finishProjectsCalled = true
				return nil
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1, Name: &name})
		require.NoError(t, err)
		assert.False(t, finishProjectsCalled)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{}, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1})
		require.Error(t, err)
	})

	t.Run("propagates error from finish descendants", func(t *testing.T) {
		mock := &mockRepo{
			updateProjectFn: func(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
				return ProjectResponse{ID: 1, Name: "p"}, nil
			},
			finishDescendantProjectsFn: func(ctx context.Context, projectID int32) error {
				return errors.New("cascade error")
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1, FinishedAt: &now})
		require.Error(t, err)
	})
}

func TestService_UpdateTask(t *testing.T) {
	now := time.Now()

	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			updateTaskFn: func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
				assert.Equal(t, int32(1), req.ID)
				return TaskResponse{ID: 1, Name: "task", FinishedAt: &now}, nil
			},
		}
		svc := NewService(mock, nil)
		got, err := svc.UpdateTask(context.Background(), UpdateTaskRequest{ID: 1, FinishedAt: &now})
		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			updateTaskFn: func(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error) {
				return TaskResponse{}, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateTask(context.Background(), UpdateTaskRequest{ID: 1})
		require.Error(t, err)
	})
}

func TestService_UpdateTodo(t *testing.T) {
	isDone := true

	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			updateTodoFn: func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
				assert.Equal(t, int32(3), req.ID)
				assert.Equal(t, &isDone, req.IsDone)
				return TodoResponse{ID: 3, TaskID: 5, Name: "My Todo", IsDone: true}, nil
			},
		}
		svc := NewService(mock, nil)
		got, err := svc.UpdateTodo(context.Background(), UpdateTodoRequest{ID: 3, IsDone: &isDone})
		require.NoError(t, err)
		assert.Equal(t, int32(3), got.ID)
		assert.True(t, got.IsDone)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			updateTodoFn: func(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error) {
				return TodoResponse{}, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateTodo(context.Background(), UpdateTodoRequest{ID: 1})
		require.Error(t, err)
	})
}

func TestService_UpdateTimeEntry(t *testing.T) {
	now := time.Now()

	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			updateTimeEntryFn: func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
				assert.Equal(t, int32(1), req.ID)
				return TimeEntryResponse{ID: 1, TaskID: 3, StartedAt: now, FinishedAt: &now}, nil
			},
		}
		svc := NewService(mock, nil)
		got, err := svc.UpdateTimeEntry(context.Background(), UpdateTimeEntryRequest{ID: 1, FinishedAt: &now})
		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			updateTimeEntryFn: func(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		_, err := svc.UpdateTimeEntry(context.Background(), UpdateTimeEntryRequest{ID: 1})
		require.Error(t, err)
	})
}

func TestService_GetActiveTree(t *testing.T) {
	parentID1 := int32(1)
	projectID1 := int32(1)
	projectID2 := int32(2)
	projDue := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	taskDesc := "important task"
	taskDue := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	taskStarted := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	t.Run("projects with nested sub-projects and tasks", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{
					{ID: 1, Name: "Parent Project", DueAt: &projDue},
					{ID: 2, ParentID: &parentID1, Name: "Child Project"},
				}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{
					{ID: 1, ProjectID: &projectID1, Name: "Task A", Description: &taskDesc, DueAt: &taskDue, Started: true, StartedAt: &taskStarted},
					{ID: 2, ProjectID: &projectID2, Name: "Task B"},
				}, nil
			},
		}
		svc := NewService(mock, nil)

		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Parent Project", got[0].Name)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, &projDue, got[0].DueAt)

		require.Len(t, got[0].Children, 2)
		child := got[0].Children[0]
		assert.Equal(t, "Child Project", child.Name)
		assert.Equal(t, "project", child.Type)
		assert.Nil(t, child.DueAt)

		taskA := got[0].Children[1]
		assert.Equal(t, "Task A", taskA.Name)
		assert.Equal(t, "task", taskA.Type)
		assert.Equal(t, &taskDesc, taskA.Description)
		assert.Equal(t, &taskDue, taskA.DueAt)
		assert.Equal(t, &taskStarted, taskA.StartedAt)

		require.Len(t, got[0].Children[0].Children, 1)
		taskB := got[0].Children[0].Children[0]
		assert.Equal(t, "Task B", taskB.Name)
		assert.Nil(t, taskB.Description)
		assert.Nil(t, taskB.DueAt)
		assert.Nil(t, taskB.StartedAt)
	})

	t.Run("orphan tasks at root level", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{
					{ID: 1, Name: "Orphan Started", Started: true},
					{ID: 2, Name: "Orphan Unstarted"},
				}, nil
			},
		}
		svc := NewService(mock, nil)

		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, "Orphan Started", got[0].Name)
		assert.Equal(t, "Orphan Unstarted", got[1].Name)
	})

	t.Run("tasks with inactive project are excluded from root", func(t *testing.T) {
		inactiveProjectID := int32(99)
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{
					{ID: 1, ProjectID: &inactiveProjectID, Name: "Task with inactive project", Started: true},
					{ID: 2, Name: "Root task"},
				}, nil
			},
		}
		svc := NewService(mock, nil)

		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Root task", got[0].Name)
	})

	t.Run("empty tree", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{}, nil
			},
		}
		svc := NewService(mock, nil)

		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got)
	})

	t.Run("ordering: projects before started tasks before unstarted tasks", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{
					{ID: 1, Name: "Project"},
				}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{
					{ID: 1, Name: "Unstarted Orphan"},
					{ID: 2, Name: "Started Orphan", Started: true},
					{ID: 3, ProjectID: &projectID1, Name: "Unstarted Child"},
					{ID: 4, ProjectID: &projectID1, Name: "Started Child", Started: true},
				}, nil
			},
		}
		svc := NewService(mock, nil)

		got, err := svc.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 3)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, "Started Orphan", got[1].Name)
		assert.Equal(t, "Unstarted Orphan", got[2].Name)

		require.Len(t, got[0].Children, 2)
		assert.Equal(t, "Started Child", got[0].Children[0].Name)
		assert.Equal(t, "Unstarted Child", got[0].Children[1].Name)
	})

	t.Run("error from GetActiveProjects propagates", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return nil, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)

		_, err := svc.GetActiveTree(context.Background())
		assert.Error(t, err)
	})

	t.Run("error from GetUnfinishedTasks propagates", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return nil, errors.New("db error")
			},
		}
		svc := NewService(mock, nil)

		_, err := svc.GetActiveTree(context.Background())
		assert.Error(t, err)
	})
}

func TestService_GetProjectChildren(t *testing.T) {
	t.Run("delegates to repo and returns result", func(t *testing.T) {
		expected := ProjectChildrenResponse{
			Project:  ProjectDetailResponse{ID: 1, Name: "Root"},
			Children: []ProjectChildNode{},
		}
		mock := &mockRepo{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				assert.Equal(t, int32(5), projectID)
				return expected, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.GetProjectChildren(context.Background(), 5)

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			getProjectChildrenFn: func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
				return ProjectChildrenResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.GetProjectChildren(context.Background(), 1)

		require.Error(t, err)
	})
}

func TestService_GetTaskTimeEntries(t *testing.T) {
	t.Run("delegates to repo and returns result", func(t *testing.T) {
		expected := TaskTimeEntriesResponse{
			Task:        TaskDetailResponse{ID: 1, Name: "Task", TimeSpent: 3600},
			TimeEntries: []TimeEntryResponse{},
		}
		mock := &mockRepo{
			getTaskTimeEntriesFn: func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
				assert.Equal(t, int32(7), taskID)
				return expected, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.GetTaskTimeEntries(context.Background(), 7)

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			getTaskTimeEntriesFn: func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
				return TaskTimeEntriesResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.GetTaskTimeEntries(context.Background(), 1)

		require.Error(t, err)
	})
}

func TestService_GetTasksByDueDate(t *testing.T) {
	t.Run("delegates to repo and returns result", func(t *testing.T) {
		now := time.Now()
		expected := []TaskByDueDateResponse{
			{ID: 1, Name: "Task A", DueAt: &now, TimeSpent: 3600},
			{ID: 2, Name: "Task B"},
		}
		mock := &mockRepo{
			getTasksByDueDateFn: func(ctx context.Context) ([]TaskByDueDateResponse, error) {
				return expected, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.GetTasksByDueDate(context.Background())

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := &mockRepo{
			getTasksByDueDateFn: func(ctx context.Context) ([]TaskByDueDateResponse, error) {
				return []TaskByDueDateResponse{}, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.GetTasksByDueDate(context.Background())

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			getTasksByDueDateFn: func(ctx context.Context) ([]TaskByDueDateResponse, error) {
				return nil, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.GetTasksByDueDate(context.Background())

		require.Error(t, err)
	})
}

func TestService_DeleteProject(t *testing.T) {
	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			deleteProjectFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(5), id)
				return nil
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteProject(context.Background(), 5)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			deleteProjectFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteProject(context.Background(), 1)
		require.Error(t, err)
	})
}

func TestService_DeleteTask(t *testing.T) {
	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			deleteTaskFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(3), id)
				return nil
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTask(context.Background(), 3)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			deleteTaskFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTask(context.Background(), 1)
		require.Error(t, err)
	})
}

func TestService_DeleteTodo(t *testing.T) {
	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			deleteTodoFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(7), id)
				return nil
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTodo(context.Background(), 7)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			deleteTodoFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTodo(context.Background(), 1)
		require.Error(t, err)
	})
}

func TestService_DeleteTimeEntry(t *testing.T) {
	t.Run("delegates to repo", func(t *testing.T) {
		mock := &mockRepo{
			deleteTimeEntryFn: func(ctx context.Context, id int32) error {
				assert.Equal(t, int32(9), id)
				return nil
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTimeEntry(context.Background(), 9)
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			deleteTimeEntryFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		svc := NewService(mock, nil)
		err := svc.DeleteTimeEntry(context.Background(), 1)
		require.Error(t, err)
	})
}
