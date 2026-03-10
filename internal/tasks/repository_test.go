package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/tasksdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuerier struct {
	createProjectFn              func(ctx context.Context, arg tasksdb.CreateProjectParams) (tasksdb.CreateProjectRow, error)
	createTaskFn                 func(ctx context.Context, arg tasksdb.CreateTaskParams) (tasksdb.CreateTaskRow, error)
	createTodoFn                 func(ctx context.Context, arg tasksdb.CreateTodoParams) (tasksdb.CreateTodoRow, error)
	createTimeEntryFn            func(ctx context.Context, arg tasksdb.CreateTimeEntryParams) (tasksdb.TimeEntry, error)
	updateProjectFn              func(ctx context.Context, arg tasksdb.UpdateProjectParams) (tasksdb.Project, error)
	updateTaskFn                 func(ctx context.Context, arg tasksdb.UpdateTaskParams) (tasksdb.Task, error)
	updateTodoFn                 func(ctx context.Context, arg tasksdb.UpdateTodoParams) (tasksdb.Todo, error)
	updateTimeEntryFn            func(ctx context.Context, arg tasksdb.UpdateTimeEntryParams) (tasksdb.TimeEntry, error)
	getRootProjectsFn            func(ctx context.Context) ([]tasksdb.GetRootProjectsRow, error)
	getActiveProjectsFn          func(ctx context.Context) ([]tasksdb.GetActiveProjectsRow, error)
	getUnfinishedTasksFn         func(ctx context.Context) ([]tasksdb.GetUnfinishedTasksRow, error)
	getProjectWithDescendantsFn  func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error)
	getTasksByProjectIDsFn       func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error)
	getTaskByIDFn                func(ctx context.Context, id int32) ([]tasksdb.GetTaskByIDRow, error)
	getTimeEntriesByTaskIDFn     func(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error)
	getTasksByDueDateFn          func(ctx context.Context) ([]tasksdb.GetTasksByDueDateRow, error)
	finishDescendantProjectsFn   func(ctx context.Context, id int32) error
	finishTasksByProjectTreeFn   func(ctx context.Context, id int32) error
	deleteProjectFn              func(ctx context.Context, id int32) error
	deleteTaskFn                 func(ctx context.Context, id int32) error
	deleteTodoFn                 func(ctx context.Context, id int32) error
	deleteTimeEntryFn            func(ctx context.Context, id int32) error
}

func (m *mockQuerier) CreateProject(ctx context.Context, arg tasksdb.CreateProjectParams) (tasksdb.CreateProjectRow, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, arg)
	}
	return tasksdb.CreateProjectRow{}, nil
}

func (m *mockQuerier) CreateTask(ctx context.Context, arg tasksdb.CreateTaskParams) (tasksdb.CreateTaskRow, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, arg)
	}
	return tasksdb.CreateTaskRow{}, nil
}

func (m *mockQuerier) CreateTodo(ctx context.Context, arg tasksdb.CreateTodoParams) (tasksdb.CreateTodoRow, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, arg)
	}
	return tasksdb.CreateTodoRow{}, nil
}

func (m *mockQuerier) CreateTimeEntry(ctx context.Context, arg tasksdb.CreateTimeEntryParams) (tasksdb.TimeEntry, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, arg)
	}
	return tasksdb.TimeEntry{}, nil
}

func (m *mockQuerier) UpdateProject(ctx context.Context, arg tasksdb.UpdateProjectParams) (tasksdb.Project, error) {
	if m.updateProjectFn != nil {
		return m.updateProjectFn(ctx, arg)
	}
	return tasksdb.Project{}, nil
}

func (m *mockQuerier) UpdateTask(ctx context.Context, arg tasksdb.UpdateTaskParams) (tasksdb.Task, error) {
	if m.updateTaskFn != nil {
		return m.updateTaskFn(ctx, arg)
	}
	return tasksdb.Task{}, nil
}

func (m *mockQuerier) UpdateTodo(ctx context.Context, arg tasksdb.UpdateTodoParams) (tasksdb.Todo, error) {
	if m.updateTodoFn != nil {
		return m.updateTodoFn(ctx, arg)
	}
	return tasksdb.Todo{}, nil
}

func (m *mockQuerier) UpdateTimeEntry(ctx context.Context, arg tasksdb.UpdateTimeEntryParams) (tasksdb.TimeEntry, error) {
	if m.updateTimeEntryFn != nil {
		return m.updateTimeEntryFn(ctx, arg)
	}
	return tasksdb.TimeEntry{}, nil
}

func (m *mockQuerier) GetRootProjects(ctx context.Context) ([]tasksdb.GetRootProjectsRow, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) GetActiveProjects(ctx context.Context) ([]tasksdb.GetActiveProjectsRow, error) {
	if m.getActiveProjectsFn != nil {
		return m.getActiveProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) GetUnfinishedTasks(ctx context.Context) ([]tasksdb.GetUnfinishedTasksRow, error) {
	if m.getUnfinishedTasksFn != nil {
		return m.getUnfinishedTasksFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) GetProjectWithDescendants(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
	if m.getProjectWithDescendantsFn != nil {
		return m.getProjectWithDescendantsFn(ctx, id)
	}
	return nil, nil
}

func (m *mockQuerier) GetTasksByProjectIDs(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
	if m.getTasksByProjectIDsFn != nil {
		return m.getTasksByProjectIDsFn(ctx, projectIds)
	}
	return nil, nil
}

func (m *mockQuerier) GetTaskByID(ctx context.Context, id int32) ([]tasksdb.GetTaskByIDRow, error) {
	if m.getTaskByIDFn != nil {
		return m.getTaskByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockQuerier) GetTimeEntriesByTaskID(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error) {
	if m.getTimeEntriesByTaskIDFn != nil {
		return m.getTimeEntriesByTaskIDFn(ctx, id)
	}
	return nil, nil
}

func (m *mockQuerier) GetTasksByDueDate(ctx context.Context) ([]tasksdb.GetTasksByDueDateRow, error) {
	if m.getTasksByDueDateFn != nil {
		return m.getTasksByDueDateFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) FinishDescendantProjects(ctx context.Context, id int32) error {
	if m.finishDescendantProjectsFn != nil {
		return m.finishDescendantProjectsFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) FinishTasksByProjectTree(ctx context.Context, id int32) error {
	if m.finishTasksByProjectTreeFn != nil {
		return m.finishTasksByProjectTreeFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) DeleteProject(ctx context.Context, id int32) error {
	if m.deleteProjectFn != nil {
		return m.deleteProjectFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) DeleteTask(ctx context.Context, id int32) error {
	if m.deleteTaskFn != nil {
		return m.deleteTaskFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) DeleteTodo(ctx context.Context, id int32) error {
	if m.deleteTodoFn != nil {
		return m.deleteTodoFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) DeleteTimeEntry(ctx context.Context, id int32) error {
	if m.deleteTimeEntryFn != nil {
		return m.deleteTimeEntryFn(ctx, id)
	}
	return nil
}

func TestRepository_CreateProject(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "test desc"
		parentID := int32(2)
		dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg tasksdb.CreateProjectParams) (tasksdb.CreateProjectRow, error) {
				return tasksdb.CreateProjectRow{
					ID:          1,
					Name:        arg.Name,
					Description: arg.Description,
					DueAt:       dueDate,
					ParentID:    arg.ParentID,
				}, nil
			},
		}
		repo := NewRepository(mock)

		dueAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		got, err := repo.CreateProject(context.Background(), "test", &desc, &dueAt, &parentID)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, "test", got.Name)
		assert.Equal(t, &desc, got.Description)
		assert.Equal(t, &dueAt, got.DueAt)
		assert.Equal(t, &parentID, got.ParentID)
	})

	t.Run("returns nil DueAt when date is invalid", func(t *testing.T) {
		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg tasksdb.CreateProjectParams) (tasksdb.CreateProjectRow, error) {
				return tasksdb.CreateProjectRow{
					ID:   2,
					Name: arg.Name,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateProject(context.Background(), "no date", nil, nil, nil)
		require.NoError(t, err)
		assert.Nil(t, got.DueAt)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg tasksdb.CreateProjectParams) (tasksdb.CreateProjectRow, error) {
				return tasksdb.CreateProjectRow{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateProject(context.Background(), "fail", nil, nil, nil)
		assert.Error(t, err)
	})
}

func TestRepository_CreateTask(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "task desc"
		projectID := int32(5)
		dueDate := pgtype.Date{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			createTaskFn: func(ctx context.Context, arg tasksdb.CreateTaskParams) (tasksdb.CreateTaskRow, error) {
				return tasksdb.CreateTaskRow{
					ID:          1,
					ProjectID:   arg.ProjectID,
					Name:        arg.Name,
					Description: arg.Description,
					DueAt:       dueDate,
				}, nil
			},
		}
		repo := NewRepository(mock)

		dueAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		got, err := repo.CreateTask(context.Background(), &projectID, "test task", &desc, &dueAt)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &projectID, got.ProjectID)
		assert.Equal(t, "test task", got.Name)
		assert.Equal(t, &desc, got.Description)
		assert.Equal(t, &dueAt, got.DueAt)
	})

	t.Run("returns nil DueAt when date is invalid", func(t *testing.T) {
		mock := &mockQuerier{
			createTaskFn: func(ctx context.Context, arg tasksdb.CreateTaskParams) (tasksdb.CreateTaskRow, error) {
				return tasksdb.CreateTaskRow{
					ID:   2,
					Name: arg.Name,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateTask(context.Background(), nil, "no date", nil, nil)
		require.NoError(t, err)
		assert.Nil(t, got.DueAt)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createTaskFn: func(ctx context.Context, arg tasksdb.CreateTaskParams) (tasksdb.CreateTaskRow, error) {
				return tasksdb.CreateTaskRow{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateTask(context.Background(), nil, "fail", nil, nil)
		assert.Error(t, err)
	})
}

func TestRepository_CreateTodo(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		mock := &mockQuerier{
			createTodoFn: func(ctx context.Context, arg tasksdb.CreateTodoParams) (tasksdb.CreateTodoRow, error) {
				return tasksdb.CreateTodoRow{
					ID:     1,
					TaskID: arg.TaskID,
					Name:   arg.Name,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateTodo(context.Background(), 5, "my todo")
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(5), got.TaskID)
		assert.Equal(t, "my todo", got.Name)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			createTodoFn: func(ctx context.Context, arg tasksdb.CreateTodoParams) (tasksdb.CreateTodoRow, error) {
				return tasksdb.CreateTodoRow{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateTodo(context.Background(), 1, "fail")
		assert.Error(t, err)
	})
}

func TestRepository_CreateTimeEntry(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		later := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		comment := "worked on feature"

		mock := &mockQuerier{
			createTimeEntryFn: func(ctx context.Context, arg tasksdb.CreateTimeEntryParams) (tasksdb.TimeEntry, error) {
				return tasksdb.TimeEntry{
					ID:         1,
					TaskID:     arg.TaskID,
					StartedAt:  arg.StartedAt,
					FinishedAt: arg.FinishedAt,
					Comment:    arg.Comment,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateTimeEntry(context.Background(), 3, now, &later, &comment)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
		assert.Equal(t, now, got.StartedAt)
		assert.Equal(t, &later, got.FinishedAt)
		assert.Equal(t, &comment, got.Comment)
	})

	t.Run("returns nil FinishedAt when timestamp is invalid", func(t *testing.T) {
		now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

		mock := &mockQuerier{
			createTimeEntryFn: func(ctx context.Context, arg tasksdb.CreateTimeEntryParams) (tasksdb.TimeEntry, error) {
				return tasksdb.TimeEntry{
					ID:        2,
					TaskID:    arg.TaskID,
					StartedAt: arg.StartedAt,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.CreateTimeEntry(context.Background(), 3, now, nil, nil)
		require.NoError(t, err)
		assert.Nil(t, got.FinishedAt)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		now := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

		mock := &mockQuerier{
			createTimeEntryFn: func(ctx context.Context, arg tasksdb.CreateTimeEntryParams) (tasksdb.TimeEntry, error) {
				return tasksdb.TimeEntry{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.CreateTimeEntry(context.Background(), 1, now, nil, nil)
		assert.Error(t, err)
	})
}

func TestRepository_GetRootProjects(t *testing.T) {
	t.Run("maps list correctly", func(t *testing.T) {
		desc := "project desc"
		dueDate := pgtype.Date{Time: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			getRootProjectsFn: func(ctx context.Context) ([]tasksdb.GetRootProjectsRow, error) {
				return []tasksdb.GetRootProjectsRow{
					{ID: 1, Name: "Alpha", Description: &desc, DueAt: dueDate},
					{ID: 2, Name: "Beta"},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetRootProjects(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, "Alpha", got[0].Name)
		assert.Equal(t, &desc, got[0].Description)
		assert.NotNil(t, got[0].DueAt)
		assert.Equal(t, "Beta", got[1].Name)
		assert.Nil(t, got[1].DueAt)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := &mockQuerier{
			getRootProjectsFn: func(ctx context.Context) ([]tasksdb.GetRootProjectsRow, error) {
				return []tasksdb.GetRootProjectsRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetRootProjects(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getRootProjectsFn: func(ctx context.Context) ([]tasksdb.GetRootProjectsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetRootProjects(context.Background())
		assert.Error(t, err)
	})
}

func TestRepository_UpdateProject(t *testing.T) {
	t.Run("maps response correctly with partial update", func(t *testing.T) {
		desc := "project desc"
		parentID := int32(2)
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)
		dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}
		name := "updated"

		mock := &mockQuerier{
			updateProjectFn: func(ctx context.Context, arg tasksdb.UpdateProjectParams) (tasksdb.Project, error) {
				assert.True(t, arg.SetName)
				assert.Equal(t, "updated", arg.Name)
				assert.False(t, arg.SetDescription)
				assert.True(t, arg.SetFinishedAt)
				return tasksdb.Project{
					ID:          1,
					ParentID:    &parentID,
					Name:        "updated",
					Description: &desc,
					DueAt:       dueDate,
					StartedAt:   pgtype.Timestamp{Time: startedAt, Valid: true},
					FinishedAt:  pgtype.Timestamp{Time: finishedAt, Valid: true},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.UpdateProject(context.Background(), UpdateProjectRequest{
			ID:         1,
			Name:       &name,
			FinishedAt: &finishedAt,
		})
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &parentID, got.ParentID)
		assert.Equal(t, "updated", got.Name)
		assert.Equal(t, &desc, got.Description)
		dueAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, &dueAt, got.DueAt)
		assert.Equal(t, &startedAt, got.StartedAt)
		assert.Equal(t, &finishedAt, got.FinishedAt)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			updateProjectFn: func(ctx context.Context, arg tasksdb.UpdateProjectParams) (tasksdb.Project, error) {
				return tasksdb.Project{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateProject(context.Background(), UpdateProjectRequest{ID: 999})
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			updateProjectFn: func(ctx context.Context, arg tasksdb.UpdateProjectParams) (tasksdb.Project, error) {
				return tasksdb.Project{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateProject(context.Background(), UpdateProjectRequest{ID: 1})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_UpdateTask(t *testing.T) {
	t.Run("maps response correctly with partial update", func(t *testing.T) {
		desc := "task desc"
		projectID := int32(5)
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)
		dueDate := pgtype.Date{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			updateTaskFn: func(ctx context.Context, arg tasksdb.UpdateTaskParams) (tasksdb.Task, error) {
				assert.True(t, arg.SetFinishedAt)
				assert.False(t, arg.SetName)
				return tasksdb.Task{
					ID:          1,
					ProjectID:   &projectID,
					Name:        "test task",
					Description: &desc,
					DueAt:       dueDate,
					StartedAt:   pgtype.Timestamp{Time: startedAt, Valid: true},
					FinishedAt:  pgtype.Timestamp{Time: finishedAt, Valid: true},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.UpdateTask(context.Background(), UpdateTaskRequest{ID: 1, FinishedAt: &finishedAt})
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &projectID, got.ProjectID)
		assert.Equal(t, "test task", got.Name)
		assert.Equal(t, &desc, got.Description)
		dueAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, &dueAt, got.DueAt)
		assert.Equal(t, &startedAt, got.StartedAt)
		assert.Equal(t, &finishedAt, got.FinishedAt)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			updateTaskFn: func(ctx context.Context, arg tasksdb.UpdateTaskParams) (tasksdb.Task, error) {
				return tasksdb.Task{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTask(context.Background(), UpdateTaskRequest{ID: 999})
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			updateTaskFn: func(ctx context.Context, arg tasksdb.UpdateTaskParams) (tasksdb.Task, error) {
				return tasksdb.Task{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTask(context.Background(), UpdateTaskRequest{ID: 1})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_UpdateTodo(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		isDone := true
		mock := &mockQuerier{
			updateTodoFn: func(ctx context.Context, arg tasksdb.UpdateTodoParams) (tasksdb.Todo, error) {
				assert.Equal(t, int32(3), arg.ID)
				assert.True(t, arg.SetIsDone)
				assert.True(t, arg.IsDone)
				assert.False(t, arg.SetName)
				return tasksdb.Todo{
					ID:     3,
					TaskID: 5,
					Name:   "My Todo",
					IsDone: true,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.UpdateTodo(context.Background(), UpdateTodoRequest{ID: 3, IsDone: &isDone})
		require.NoError(t, err)

		assert.Equal(t, int32(3), got.ID)
		assert.Equal(t, int32(5), got.TaskID)
		assert.Equal(t, "My Todo", got.Name)
		assert.True(t, got.IsDone)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			updateTodoFn: func(ctx context.Context, arg tasksdb.UpdateTodoParams) (tasksdb.Todo, error) {
				return tasksdb.Todo{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTodo(context.Background(), UpdateTodoRequest{ID: 999})
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			updateTodoFn: func(ctx context.Context, arg tasksdb.UpdateTodoParams) (tasksdb.Todo, error) {
				return tasksdb.Todo{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTodo(context.Background(), UpdateTodoRequest{ID: 1})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_UpdateTimeEntry(t *testing.T) {
	t.Run("maps response correctly with partial update", func(t *testing.T) {
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		comment := "done"

		mock := &mockQuerier{
			updateTimeEntryFn: func(ctx context.Context, arg tasksdb.UpdateTimeEntryParams) (tasksdb.TimeEntry, error) {
				assert.True(t, arg.SetFinishedAt)
				assert.False(t, arg.SetStartedAt)
				assert.False(t, arg.SetComment)
				return tasksdb.TimeEntry{
					ID:         1,
					TaskID:     3,
					StartedAt:  pgtype.Timestamp{Time: startedAt, Valid: true},
					FinishedAt: pgtype.Timestamp{Time: finishedAt, Valid: true},
					Comment:    &comment,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.UpdateTimeEntry(context.Background(), UpdateTimeEntryRequest{ID: 1, FinishedAt: &finishedAt})
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
		assert.Equal(t, startedAt, got.StartedAt)
		assert.Equal(t, &finishedAt, got.FinishedAt)
		assert.Equal(t, &comment, got.Comment)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			updateTimeEntryFn: func(ctx context.Context, arg tasksdb.UpdateTimeEntryParams) (tasksdb.TimeEntry, error) {
				return tasksdb.TimeEntry{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTimeEntry(context.Background(), UpdateTimeEntryRequest{ID: 999})
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			updateTimeEntryFn: func(ctx context.Context, arg tasksdb.UpdateTimeEntryParams) (tasksdb.TimeEntry, error) {
				return tasksdb.TimeEntry{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		_, err := repo.UpdateTimeEntry(context.Background(), UpdateTimeEntryRequest{ID: 1})
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_GetActiveProjects(t *testing.T) {
	parentID := int32(1)
	dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

	t.Run("maps rows correctly", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]tasksdb.GetActiveProjectsRow, error) {
				return []tasksdb.GetActiveProjectsRow{
					{ID: 1, Name: "Parent Project", DueAt: dueDate},
					{ID: 2, ParentID: &parentID, Name: "Child Project"},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveProjects(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, int32(1), got[0].ID)
		assert.Equal(t, "Parent Project", got[0].Name)
		assert.Nil(t, got[0].ParentID)
		assert.Equal(t, &dueDate.Time, got[0].DueAt)
		assert.Equal(t, int32(2), got[1].ID)
		assert.Equal(t, "Child Project", got[1].Name)
		assert.Equal(t, &parentID, got[1].ParentID)
		assert.Nil(t, got[1].DueAt)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]tasksdb.GetActiveProjectsRow, error) {
				return []tasksdb.GetActiveProjectsRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveProjects(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]tasksdb.GetActiveProjectsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetActiveProjects(context.Background())
		assert.Error(t, err)
	})
}

func TestRepository_GetUnfinishedTasks(t *testing.T) {
	projectID := int32(5)
	desc := "do something"
	dueDate := pgtype.Date{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	startedAt := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), Valid: true}

	t.Run("maps rows correctly", func(t *testing.T) {
		mock := &mockQuerier{
			getUnfinishedTasksFn: func(ctx context.Context) ([]tasksdb.GetUnfinishedTasksRow, error) {
				return []tasksdb.GetUnfinishedTasksRow{
					{ID: 1, ProjectID: &projectID, Name: "Task A", Description: &desc, DueAt: dueDate, StartedAt: startedAt},
					{ID: 2, Name: "Task B"},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetUnfinishedTasks(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, int32(1), got[0].ID)
		assert.Equal(t, &projectID, got[0].ProjectID)
		assert.Equal(t, "Task A", got[0].Name)
		assert.Equal(t, &desc, got[0].Description)
		assert.Equal(t, &dueDate.Time, got[0].DueAt)
		assert.True(t, got[0].Started)
		assert.Equal(t, &startedAt.Time, got[0].StartedAt)
		assert.Equal(t, int32(2), got[1].ID)
		assert.Nil(t, got[1].ProjectID)
		assert.Equal(t, "Task B", got[1].Name)
		assert.Nil(t, got[1].Description)
		assert.Nil(t, got[1].DueAt)
		assert.False(t, got[1].Started)
		assert.Nil(t, got[1].StartedAt)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := &mockQuerier{
			getUnfinishedTasksFn: func(ctx context.Context) ([]tasksdb.GetUnfinishedTasksRow, error) {
				return []tasksdb.GetUnfinishedTasksRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetUnfinishedTasks(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getUnfinishedTasksFn: func(ctx context.Context) ([]tasksdb.GetUnfinishedTasksRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetUnfinishedTasks(context.Background())
		assert.Error(t, err)
	})
}

func TestRepository_GetProjectChildren(t *testing.T) {
	parentID := int32(1)
	finishedAt := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC), Valid: true}

	t.Run("project with sub-projects and tasks", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 1, Name: "Root", Depth: 0},
					{ID: 2, ParentID: &parentID, Name: "Sub-Project", Depth: 1},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				pid1 := int32(1)
				pid2 := int32(2)
				todoID := int32(10)
				todoName := "My Todo"
				todoDone := false
				return []tasksdb.GetTasksByProjectIDsRow{
					{ID: 1, ProjectID: &pid1, Name: "Task A", TimeSpent: 3600, TodoID: &todoID, TodoName: &todoName, TodoIsDone: &todoDone},
					{ID: 2, ProjectID: &pid2, Name: "Task B", TimeSpent: 1800},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetProjectChildren(context.Background(), 1)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.Project.ID)
		assert.Equal(t, "Root", got.Project.Name)
		assert.Equal(t, int64(5400), got.Project.TimeSpent)

		require.Len(t, got.Children, 2)
		assert.Equal(t, "project", got.Children[0].Type)
		assert.Equal(t, "Sub-Project", got.Children[0].Name)
		assert.Equal(t, int64(1800), got.Children[0].TimeSpent)

		assert.Equal(t, "task", got.Children[1].Type)
		assert.Equal(t, "Task A", got.Children[1].Name)
		assert.Equal(t, int64(3600), got.Children[1].TimeSpent)
		require.Len(t, got.Children[1].Todos, 1)
		assert.Equal(t, "My Todo", got.Children[1].Todos[0].Name)
	})

	t.Run("tasks with multiple todos", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 1, Name: "Root", Depth: 0},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				pid := int32(1)
				todoID1, todoID2 := int32(10), int32(11)
				todoName1, todoName2 := "Todo 1", "Todo 2"
				todoDone := false
				return []tasksdb.GetTasksByProjectIDsRow{
					{ID: 1, ProjectID: &pid, Name: "Task", TimeSpent: 100, TodoID: &todoID1, TodoName: &todoName1, TodoIsDone: &todoDone},
					{ID: 1, ProjectID: &pid, Name: "Task", TimeSpent: 100, TodoID: &todoID2, TodoName: &todoName2, TodoIsDone: &todoDone},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetProjectChildren(context.Background(), 1)
		require.NoError(t, err)

		require.Len(t, got.Children, 1)
		assert.Equal(t, "Task", got.Children[0].Name)
		require.Len(t, got.Children[0].Todos, 2)
		assert.Equal(t, "Todo 1", got.Children[0].Todos[0].Name)
		assert.Equal(t, "Todo 2", got.Children[0].Todos[1].Name)
	})

	t.Run("project with no children", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 5, Name: "Empty", Depth: 0},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				return []tasksdb.GetTasksByProjectIDsRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetProjectChildren(context.Background(), 5)
		require.NoError(t, err)

		assert.Equal(t, int32(5), got.Project.ID)
		assert.Empty(t, got.Children)
		assert.NotNil(t, got.Children)
	})

	t.Run("recursive time_spent accumulation", func(t *testing.T) {
		pid1 := int32(1)
		pid2 := int32(2)
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 1, Name: "Root", Depth: 0},
					{ID: 2, ParentID: &pid1, Name: "Child", Depth: 1},
					{ID: 3, ParentID: &pid2, Name: "Grandchild", Depth: 2},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				pid3 := int32(3)
				return []tasksdb.GetTasksByProjectIDsRow{
					{ID: 1, ProjectID: &pid3, Name: "Deep Task", TimeSpent: 500},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetProjectChildren(context.Background(), 1)
		require.NoError(t, err)

		assert.Equal(t, int64(500), got.Project.TimeSpent)
		require.Len(t, got.Children, 1)
		assert.Equal(t, int64(500), got.Children[0].TimeSpent)
	})

	t.Run("ordering: sub-projects first, unfinished tasks, finished tasks", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 1, Name: "Root", Depth: 0},
					{ID: 2, ParentID: &parentID, Name: "Sub", Depth: 1},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				pid := int32(1)
				return []tasksdb.GetTasksByProjectIDsRow{
					{ID: 1, ProjectID: &pid, Name: "Active Task", TimeSpent: 0},
					{ID: 2, ProjectID: &pid, Name: "Done Task", TimeSpent: 100, FinishedAt: finishedAt},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetProjectChildren(context.Background(), 1)
		require.NoError(t, err)

		require.Len(t, got.Children, 3)
		assert.Equal(t, "project", got.Children[0].Type)
		assert.Equal(t, "Sub", got.Children[0].Name)
		assert.Equal(t, "task", got.Children[1].Type)
		assert.Equal(t, "Active Task", got.Children[1].Name)
		assert.Nil(t, got.Children[1].FinishedAt)
		assert.Equal(t, "task", got.Children[2].Type)
		assert.Equal(t, "Done Task", got.Children[2].Name)
		assert.NotNil(t, got.Children[2].FinishedAt)
	})

	t.Run("ErrNotFound when project doesn't exist", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{}, nil
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetProjectChildren(context.Background(), 999)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("error from GetProjectWithDescendants propagates", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetProjectChildren(context.Background(), 1)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})

	t.Run("error from GetTasksByProjectIDs propagates", func(t *testing.T) {
		mock := &mockQuerier{
			getProjectWithDescendantsFn: func(ctx context.Context, id int32) ([]tasksdb.GetProjectWithDescendantsRow, error) {
				return []tasksdb.GetProjectWithDescendantsRow{
					{ID: 1, Name: "Root", Depth: 0},
				}, nil
			},
			getTasksByProjectIDsFn: func(ctx context.Context, projectIds []int32) ([]tasksdb.GetTasksByProjectIDsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetProjectChildren(context.Background(), 1)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_GetTaskTimeEntries(t *testing.T) {
	projectID := int32(3)
	desc := "task desc"
	comment1 := "first"
	comment2 := "second"
	startedAt := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), Valid: true}
	finishedAt := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC), Valid: true}
	dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

	t.Run("maps task and time entries correctly", func(t *testing.T) {
		entryID1, entryID2 := int32(10), int32(11)
		entryStart1 := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), Valid: true}
		entryEnd1 := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), Valid: true}
		entryStart2 := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			getTimeEntriesByTaskIDFn: func(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error) {
				return []tasksdb.GetTimeEntriesByTaskIDRow{
					{
						TaskID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc,
						DueAt: dueDate, TaskStartedAt: startedAt, TaskFinishedAt: finishedAt,
						TimeSpent:   3600,
						TimeEntryID: &entryID1, EntryStartedAt: entryStart1, EntryFinishedAt: entryEnd1, Comment: &comment1,
					},
					{
						TaskID: 1, ProjectID: &projectID, Name: "My Task", Description: &desc,
						DueAt: dueDate, TaskStartedAt: startedAt, TaskFinishedAt: finishedAt,
						TimeSpent:   3600,
						TimeEntryID: &entryID2, EntryStartedAt: entryStart2, Comment: &comment2,
					},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetTaskTimeEntries(context.Background(), 1)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.Task.ID)
		assert.Equal(t, &projectID, got.Task.ProjectID)
		assert.Equal(t, "My Task", got.Task.Name)
		assert.Equal(t, &desc, got.Task.Description)
		assert.NotNil(t, got.Task.DueAt)
		assert.NotNil(t, got.Task.StartedAt)
		assert.NotNil(t, got.Task.FinishedAt)
		assert.Equal(t, int64(3600), got.Task.TimeSpent)

		require.Len(t, got.TimeEntries, 2)
		assert.Equal(t, int32(10), got.TimeEntries[0].ID)
		assert.NotNil(t, got.TimeEntries[0].FinishedAt)
		assert.Equal(t, &comment1, got.TimeEntries[0].Comment)
		assert.Equal(t, int32(11), got.TimeEntries[1].ID)
		assert.Nil(t, got.TimeEntries[1].FinishedAt)
	})

	t.Run("task with no time entries", func(t *testing.T) {
		mock := &mockQuerier{
			getTimeEntriesByTaskIDFn: func(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error) {
				return []tasksdb.GetTimeEntriesByTaskIDRow{
					{
						TaskID: 1, Name: "Solo Task", TimeSpent: 0,
					},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetTaskTimeEntries(context.Background(), 1)
		require.NoError(t, err)

		assert.Equal(t, "Solo Task", got.Task.Name)
		assert.Equal(t, int64(0), got.Task.TimeSpent)
		assert.Empty(t, got.TimeEntries)
		assert.NotNil(t, got.TimeEntries)
	})

	t.Run("ErrNotFound when task doesn't exist", func(t *testing.T) {
		mock := &mockQuerier{
			getTimeEntriesByTaskIDFn: func(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error) {
				return []tasksdb.GetTimeEntriesByTaskIDRow{}, nil
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetTaskTimeEntries(context.Background(), 999)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getTimeEntriesByTaskIDFn: func(ctx context.Context, id int32) ([]tasksdb.GetTimeEntriesByTaskIDRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetTaskTimeEntries(context.Background(), 1)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_GetTasksByDueDate(t *testing.T) {
	desc := "task desc"
	projectID := int32(5)
	projectName := "My Project"
	taskDueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}
	projDueDate := pgtype.Date{Time: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), Valid: true}
	startedAt := pgtype.Timestamp{Time: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC), Valid: true}

	t.Run("maps rows correctly", func(t *testing.T) {
		mock := &mockQuerier{
			getTasksByDueDateFn: func(ctx context.Context) ([]tasksdb.GetTasksByDueDateRow, error) {
				return []tasksdb.GetTasksByDueDateRow{
					{
						ID: 1, Name: "Task A", Description: &desc,
						DueAt: taskDueDate, StartedAt: startedAt,
						ProjectID: &projectID, ProjectName: &projectName, ProjectDueAt: projDueDate,
						TimeSpent: 3600,
					},
					{
						ID: 2, Name: "Task B",
						ProjectID: &projectID, ProjectName: &projectName, ProjectDueAt: projDueDate,
						TimeSpent: 0,
					},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetTasksByDueDate(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)

		assert.Equal(t, int32(1), got[0].ID)
		assert.Equal(t, "Task A", got[0].Name)
		assert.Equal(t, &desc, got[0].Description)
		assert.NotNil(t, got[0].DueAt)
		assert.NotNil(t, got[0].StartedAt)
		assert.Equal(t, &projectID, got[0].ProjectID)
		assert.Equal(t, &projectName, got[0].ProjectName)
		assert.NotNil(t, got[0].ProjectDueAt)
		assert.Equal(t, int64(3600), got[0].TimeSpent)

		assert.Equal(t, int32(2), got[1].ID)
		assert.Equal(t, "Task B", got[1].Name)
		assert.Nil(t, got[1].DueAt)
		assert.Nil(t, got[1].StartedAt)
		assert.Equal(t, int64(0), got[1].TimeSpent)
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock := &mockQuerier{
			getTasksByDueDateFn: func(ctx context.Context) ([]tasksdb.GetTasksByDueDateRow, error) {
				return []tasksdb.GetTasksByDueDateRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetTasksByDueDate(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getTasksByDueDateFn: func(ctx context.Context) ([]tasksdb.GetTasksByDueDateRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetTasksByDueDate(context.Background())
		assert.Error(t, err)
	})
}

func TestRepository_DeleteProject(t *testing.T) {
	t.Run("calls querier with correct ID", func(t *testing.T) {
		var gotID int32
		mock := &mockQuerier{
			deleteProjectFn: func(ctx context.Context, id int32) error {
				gotID = id
				return nil
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteProject(context.Background(), 5)
		require.NoError(t, err)
		assert.Equal(t, int32(5), gotID)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			deleteProjectFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteProject(context.Background(), 1)
		assert.Error(t, err)
	})
}

func TestRepository_DeleteTask(t *testing.T) {
	t.Run("calls querier with correct ID", func(t *testing.T) {
		var gotID int32
		mock := &mockQuerier{
			deleteTaskFn: func(ctx context.Context, id int32) error {
				gotID = id
				return nil
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTask(context.Background(), 3)
		require.NoError(t, err)
		assert.Equal(t, int32(3), gotID)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			deleteTaskFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTask(context.Background(), 1)
		assert.Error(t, err)
	})
}

func TestRepository_DeleteTodo(t *testing.T) {
	t.Run("calls querier with correct ID", func(t *testing.T) {
		var gotID int32
		mock := &mockQuerier{
			deleteTodoFn: func(ctx context.Context, id int32) error {
				gotID = id
				return nil
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTodo(context.Background(), 7)
		require.NoError(t, err)
		assert.Equal(t, int32(7), gotID)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			deleteTodoFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTodo(context.Background(), 1)
		assert.Error(t, err)
	})
}

func TestRepository_DeleteTimeEntry(t *testing.T) {
	t.Run("calls querier with correct ID", func(t *testing.T) {
		var gotID int32
		mock := &mockQuerier{
			deleteTimeEntryFn: func(ctx context.Context, id int32) error {
				gotID = id
				return nil
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTimeEntry(context.Background(), 9)
		require.NoError(t, err)
		assert.Equal(t, int32(9), gotID)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			deleteTimeEntryFn: func(ctx context.Context, id int32) error {
				return errors.New("db error")
			},
		}
		repo := NewRepository(mock)
		err := repo.DeleteTimeEntry(context.Background(), 1)
		assert.Error(t, err)
	})
}
