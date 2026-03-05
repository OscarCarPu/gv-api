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
	finishTimeEntryFn func(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error)
	finishTaskFn      func(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error)
	finishProjectFn   func(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error)
	getRootProjectsFn      func(ctx context.Context) ([]ProjectResponse, error)
	getActiveProjectsFn    func(ctx context.Context) ([]ActiveProject, error)
	getUnfinishedTasksFn   func(ctx context.Context) ([]UnfinishedTask, error)
	getProjectChildrenFn   func(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	getTaskTimeEntriesFn   func(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
	toggleTodoFn           func(ctx context.Context, id int32) (TodoResponse, error)
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

func (m *mockRepo) FinishTimeEntry(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
	if m.finishTimeEntryFn != nil {
		return m.finishTimeEntryFn(ctx, id, finishedAt)
	}
	return TimeEntryResponse{}, nil
}

func (m *mockRepo) FinishTask(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
	if m.finishTaskFn != nil {
		return m.finishTaskFn(ctx, id, finishedAt)
	}
	return TaskResponse{}, nil
}

func (m *mockRepo) FinishProject(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
	if m.finishProjectFn != nil {
		return m.finishProjectFn(ctx, id, finishedAt)
	}
	return ProjectResponse{}, nil
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

func (m *mockRepo) ToggleTodo(ctx context.Context, id int32) (TodoResponse, error) {
	if m.toggleTodoFn != nil {
		return m.toggleTodoFn(ctx, id)
	}
	return TodoResponse{}, nil
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

func TestService_FinishTask(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	t.Run("with explicit finished_at", func(t *testing.T) {
		mock := &mockRepo{
			finishTaskFn: func(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
				assert.Equal(t, int32(1), id)
				assert.Equal(t, now, finishedAt)
				return TaskResponse{ID: 1, Name: "task", FinishedAt: &now}, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.FinishTask(context.Background(), FinishTaskRequest{ID: 1, FinishedAt: &now})

		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &now, got.FinishedAt)
	})

	t.Run("without finished_at defaults to now", func(t *testing.T) {
		var capturedFinishedAt time.Time
		mock := &mockRepo{
			finishTaskFn: func(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
				capturedFinishedAt = finishedAt
				return TaskResponse{ID: 1, Name: "task", FinishedAt: &finishedAt}, nil
			},
		}

		svc := NewService(mock, time.UTC)
		before := time.Now().In(time.UTC)
		_, err := svc.FinishTask(context.Background(), FinishTaskRequest{ID: 1})
		after := time.Now().In(time.UTC)

		require.NoError(t, err)
		assert.False(t, capturedFinishedAt.Before(before))
		assert.False(t, capturedFinishedAt.After(after))
	})

	t.Run("repository error", func(t *testing.T) {
		mock := &mockRepo{
			finishTaskFn: func(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
				return TaskResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.FinishTask(context.Background(), FinishTaskRequest{ID: 1})

		require.Error(t, err)
	})
}

func TestService_FinishProject(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	t.Run("with explicit finished_at", func(t *testing.T) {
		mock := &mockRepo{
			finishProjectFn: func(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
				assert.Equal(t, int32(1), id)
				assert.Equal(t, now, finishedAt)
				return ProjectResponse{ID: 1, Name: "project", FinishedAt: &now}, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.FinishProject(context.Background(), FinishProjectRequest{ID: 1, FinishedAt: &now})

		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &now, got.FinishedAt)
	})

	t.Run("without finished_at defaults to now", func(t *testing.T) {
		var capturedFinishedAt time.Time
		mock := &mockRepo{
			finishProjectFn: func(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
				capturedFinishedAt = finishedAt
				return ProjectResponse{ID: 1, Name: "project", FinishedAt: &finishedAt}, nil
			},
		}

		svc := NewService(mock, time.UTC)
		before := time.Now().In(time.UTC)
		_, err := svc.FinishProject(context.Background(), FinishProjectRequest{ID: 1})
		after := time.Now().In(time.UTC)

		require.NoError(t, err)
		assert.False(t, capturedFinishedAt.Before(before))
		assert.False(t, capturedFinishedAt.After(after))
	})

	t.Run("repository error", func(t *testing.T) {
		mock := &mockRepo{
			finishProjectFn: func(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
				return ProjectResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.FinishProject(context.Background(), FinishProjectRequest{ID: 1})

		require.Error(t, err)
	})
}

func TestService_FinishTimeEntry(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	t.Run("with explicit finished_at", func(t *testing.T) {
		mock := &mockRepo{
			finishTimeEntryFn: func(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
				assert.Equal(t, int32(1), id)
				assert.Equal(t, now, finishedAt)
				return TimeEntryResponse{ID: 1, TaskID: 3, StartedAt: startedAt, FinishedAt: &now}, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.FinishTimeEntry(context.Background(), FinishTimeEntryRequest{ID: 1, FinishedAt: &now})

		require.NoError(t, err)
		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &now, got.FinishedAt)
	})

	t.Run("without finished_at defaults to now", func(t *testing.T) {
		var capturedFinishedAt time.Time
		mock := &mockRepo{
			finishTimeEntryFn: func(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
				capturedFinishedAt = finishedAt
				return TimeEntryResponse{ID: 1, TaskID: 3, StartedAt: startedAt, FinishedAt: &finishedAt}, nil
			},
		}

		svc := NewService(mock, time.UTC)
		before := time.Now().In(time.UTC)
		_, err := svc.FinishTimeEntry(context.Background(), FinishTimeEntryRequest{ID: 1})
		after := time.Now().In(time.UTC)

		require.NoError(t, err)
		assert.False(t, capturedFinishedAt.Before(before))
		assert.False(t, capturedFinishedAt.After(after))
	})

	t.Run("repository error", func(t *testing.T) {
		mock := &mockRepo{
			finishTimeEntryFn: func(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
				return TimeEntryResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.FinishTimeEntry(context.Background(), FinishTimeEntryRequest{ID: 1})

		require.Error(t, err)
	})
}

func TestService_GetActiveTree(t *testing.T) {
	parentID1 := int32(1)
	projectID1 := int32(1)
	projectID2 := int32(2)

	t.Run("projects with nested sub-projects and tasks", func(t *testing.T) {
		mock := &mockRepo{
			getActiveProjectsFn: func(ctx context.Context) ([]ActiveProject, error) {
				return []ActiveProject{
					{ID: 1, Name: "Parent Project"},
					{ID: 2, ParentID: &parentID1, Name: "Child Project"},
				}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]UnfinishedTask, error) {
				return []UnfinishedTask{
					{ID: 1, ProjectID: &projectID1, Name: "Task A", Started: true},
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

		require.Len(t, got[0].Children, 2)
		assert.Equal(t, "Child Project", got[0].Children[0].Name)
		assert.Equal(t, "project", got[0].Children[0].Type)
		assert.Equal(t, "Task A", got[0].Children[1].Name)
		assert.Equal(t, "task", got[0].Children[1].Type)

		require.Len(t, got[0].Children[0].Children, 1)
		assert.Equal(t, "Task B", got[0].Children[0].Children[0].Name)
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

func TestService_ToggleTodo(t *testing.T) {
	t.Run("delegates to repo and returns result", func(t *testing.T) {
		expected := TodoResponse{ID: 1, TaskID: 5, Name: "My Todo", IsDone: true}
		mock := &mockRepo{
			toggleTodoFn: func(ctx context.Context, id int32) (TodoResponse, error) {
				assert.Equal(t, int32(3), id)
				return expected, nil
			},
		}

		svc := NewService(mock, nil)
		got, err := svc.ToggleTodo(context.Background(), 3)

		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})

	t.Run("propagates error", func(t *testing.T) {
		mock := &mockRepo{
			toggleTodoFn: func(ctx context.Context, id int32) (TodoResponse, error) {
				return TodoResponse{}, errors.New("db error")
			},
		}

		svc := NewService(mock, nil)
		_, err := svc.ToggleTodo(context.Background(), 1)

		require.Error(t, err)
	})
}
