package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuerier struct {
	createProjectFn   func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error)
	createTaskFn      func(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error)
	createTodoFn      func(ctx context.Context, arg sqlc.CreateTodoParams) (sqlc.CreateTodoRow, error)
	createTimeEntryFn func(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error)
	finishTimeEntryFn func(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error)
	finishTaskFn      func(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error)
	finishProjectFn   func(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error)
	getRootProjectsFn    func(ctx context.Context) ([]sqlc.GetRootProjectsRow, error)
	getActiveProjectsFn  func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error)
	getUnfinishedTasksFn func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error)
}

func (m *mockQuerier) CreateProject(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
	if m.createProjectFn != nil {
		return m.createProjectFn(ctx, arg)
	}
	return sqlc.CreateProjectRow{}, nil
}

func (m *mockQuerier) CreateTask(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(ctx, arg)
	}
	return sqlc.CreateTaskRow{}, nil
}

func (m *mockQuerier) CreateTodo(ctx context.Context, arg sqlc.CreateTodoParams) (sqlc.CreateTodoRow, error) {
	if m.createTodoFn != nil {
		return m.createTodoFn(ctx, arg)
	}
	return sqlc.CreateTodoRow{}, nil
}

func (m *mockQuerier) CreateTimeEntry(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error) {
	if m.createTimeEntryFn != nil {
		return m.createTimeEntryFn(ctx, arg)
	}
	return sqlc.TimeEntry{}, nil
}

func (m *mockQuerier) FinishTimeEntry(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error) {
	if m.finishTimeEntryFn != nil {
		return m.finishTimeEntryFn(ctx, arg)
	}
	return sqlc.TimeEntry{}, nil
}

func (m *mockQuerier) FinishTask(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error) {
	if m.finishTaskFn != nil {
		return m.finishTaskFn(ctx, arg)
	}
	return sqlc.Task{}, nil
}

func (m *mockQuerier) FinishProject(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error) {
	if m.finishProjectFn != nil {
		return m.finishProjectFn(ctx, arg)
	}
	return sqlc.Project{}, nil
}

func (m *mockQuerier) GetRootProjects(ctx context.Context) ([]sqlc.GetRootProjectsRow, error) {
	if m.getRootProjectsFn != nil {
		return m.getRootProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) GetActiveProjects(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
	if m.getActiveProjectsFn != nil {
		return m.getActiveProjectsFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) GetUnfinishedTasks(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
	if m.getUnfinishedTasksFn != nil {
		return m.getUnfinishedTasksFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) CreateHabit(ctx context.Context, arg sqlc.CreateHabitParams) (sqlc.Habit, error) {
	return sqlc.Habit{}, nil
}

func (m *mockQuerier) GetHabitsWithLogs(ctx context.Context, logDate time.Time) ([]sqlc.GetHabitsWithLogsRow, error) {
	return nil, nil
}

func (m *mockQuerier) UpsertLog(ctx context.Context, arg sqlc.UpsertLogParams) error {
	return nil
}

func TestRepository_CreateProject(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "test desc"
		parentID := int32(2)
		dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{
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
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{
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
			createProjectFn: func(ctx context.Context, arg sqlc.CreateProjectParams) (sqlc.CreateProjectRow, error) {
				return sqlc.CreateProjectRow{}, errors.New("db error")
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
			createTaskFn: func(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error) {
				return sqlc.CreateTaskRow{
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
			createTaskFn: func(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error) {
				return sqlc.CreateTaskRow{
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
			createTaskFn: func(ctx context.Context, arg sqlc.CreateTaskParams) (sqlc.CreateTaskRow, error) {
				return sqlc.CreateTaskRow{}, errors.New("db error")
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
			createTodoFn: func(ctx context.Context, arg sqlc.CreateTodoParams) (sqlc.CreateTodoRow, error) {
				return sqlc.CreateTodoRow{
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
			createTodoFn: func(ctx context.Context, arg sqlc.CreateTodoParams) (sqlc.CreateTodoRow, error) {
				return sqlc.CreateTodoRow{}, errors.New("db error")
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
			createTimeEntryFn: func(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{
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
			createTimeEntryFn: func(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{
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
			createTimeEntryFn: func(ctx context.Context, arg sqlc.CreateTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{}, errors.New("db error")
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
			getRootProjectsFn: func(ctx context.Context) ([]sqlc.GetRootProjectsRow, error) {
				return []sqlc.GetRootProjectsRow{
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
			getRootProjectsFn: func(ctx context.Context) ([]sqlc.GetRootProjectsRow, error) {
				return []sqlc.GetRootProjectsRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetRootProjects(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			getRootProjectsFn: func(ctx context.Context) ([]sqlc.GetRootProjectsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetRootProjects(context.Background())
		assert.Error(t, err)
	})
}

func TestRepository_FinishTask(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "task desc"
		projectID := int32(5)
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)
		dueDate := pgtype.Date{Time: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			finishTaskFn: func(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error) {
				return sqlc.Task{
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

		got, err := repo.FinishTask(context.Background(), 1, finishedAt)
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
			finishTaskFn: func(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error) {
				return sqlc.Task{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishTask(context.Background(), 999, time.Now())
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			finishTaskFn: func(ctx context.Context, arg sqlc.FinishTaskParams) (sqlc.Task, error) {
				return sqlc.Task{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishTask(context.Background(), 1, time.Now())
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_FinishProject(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		desc := "project desc"
		parentID := int32(2)
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)
		dueDate := pgtype.Date{Time: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), Valid: true}

		mock := &mockQuerier{
			finishProjectFn: func(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error) {
				return sqlc.Project{
					ID:          1,
					ParentID:    &parentID,
					Name:        "test project",
					Description: &desc,
					DueAt:       dueDate,
					StartedAt:   pgtype.Timestamp{Time: startedAt, Valid: true},
					FinishedAt:  pgtype.Timestamp{Time: finishedAt, Valid: true},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.FinishProject(context.Background(), 1, finishedAt)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, &parentID, got.ParentID)
		assert.Equal(t, "test project", got.Name)
		assert.Equal(t, &desc, got.Description)
		dueAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, &dueAt, got.DueAt)
		assert.Equal(t, &startedAt, got.StartedAt)
		assert.Equal(t, &finishedAt, got.FinishedAt)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			finishProjectFn: func(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error) {
				return sqlc.Project{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishProject(context.Background(), 999, time.Now())
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			finishProjectFn: func(ctx context.Context, arg sqlc.FinishProjectParams) (sqlc.Project, error) {
				return sqlc.Project{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishProject(context.Background(), 1, time.Now())
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_FinishTimeEntry(t *testing.T) {
	t.Run("maps response correctly", func(t *testing.T) {
		startedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
		finishedAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		comment := "done"

		mock := &mockQuerier{
			finishTimeEntryFn: func(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{
					ID:         1,
					TaskID:     3,
					StartedAt:  pgtype.Timestamp{Time: startedAt, Valid: true},
					FinishedAt: pgtype.Timestamp{Time: finishedAt, Valid: true},
					Comment:    &comment,
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.FinishTimeEntry(context.Background(), 1, finishedAt)
		require.NoError(t, err)

		assert.Equal(t, int32(1), got.ID)
		assert.Equal(t, int32(3), got.TaskID)
		assert.Equal(t, startedAt, got.StartedAt)
		assert.Equal(t, &finishedAt, got.FinishedAt)
		assert.Equal(t, &comment, got.Comment)
	})

	t.Run("returns ErrNotFound on pgx.ErrNoRows", func(t *testing.T) {
		mock := &mockQuerier{
			finishTimeEntryFn: func(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{}, pgx.ErrNoRows
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishTimeEntry(context.Background(), 999, time.Now())
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("returns error from querier", func(t *testing.T) {
		mock := &mockQuerier{
			finishTimeEntryFn: func(ctx context.Context, arg sqlc.FinishTimeEntryParams) (sqlc.TimeEntry, error) {
				return sqlc.TimeEntry{}, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.FinishTimeEntry(context.Background(), 1, time.Now())
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrNotFound)
	})
}

func TestRepository_GetActiveTree(t *testing.T) {
	parentID1 := int32(1)
	projectID1 := int32(1)
	projectID2 := int32(2)

	t.Run("projects with nested sub-projects and tasks", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return []sqlc.GetActiveProjectsRow{
					{ID: 1, Name: "Parent Project"},
					{ID: 2, ParentID: &parentID1, Name: "Child Project"},
				}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
				return []sqlc.GetUnfinishedTasksRow{
					{ID: 1, ProjectID: &projectID1, Name: "Task A", StartedAt: pgtype.Timestamp{Time: time.Now(), Valid: true}},
					{ID: 2, ProjectID: &projectID2, Name: "Task B"},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 1)
		assert.Equal(t, "Parent Project", got[0].Name)
		assert.Equal(t, "project", got[0].Type)

		// Children: sub-project first, then started task
		require.Len(t, got[0].Children, 2)
		assert.Equal(t, "Child Project", got[0].Children[0].Name)
		assert.Equal(t, "project", got[0].Children[0].Type)
		assert.Equal(t, "Task A", got[0].Children[1].Name)
		assert.Equal(t, "task", got[0].Children[1].Type)

		// Child project has its own task
		require.Len(t, got[0].Children[0].Children, 1)
		assert.Equal(t, "Task B", got[0].Children[0].Children[0].Name)
	})

	t.Run("orphan tasks at root level", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return []sqlc.GetActiveProjectsRow{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
				return []sqlc.GetUnfinishedTasksRow{
					{ID: 1, Name: "Orphan Started", StartedAt: pgtype.Timestamp{Time: time.Now(), Valid: true}},
					{ID: 2, Name: "Orphan Unstarted"},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveTree(context.Background())
		require.NoError(t, err)

		require.Len(t, got, 2)
		assert.Equal(t, "Orphan Started", got[0].Name)
		assert.Equal(t, "Orphan Unstarted", got[1].Name)
	})

	t.Run("empty tree", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return []sqlc.GetActiveProjectsRow{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
				return []sqlc.GetUnfinishedTasksRow{}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveTree(context.Background())
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.NotNil(t, got)
	})

	t.Run("ordering: projects before started tasks before unstarted tasks", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return []sqlc.GetActiveProjectsRow{
					{ID: 1, Name: "Project"},
				}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
				return []sqlc.GetUnfinishedTasksRow{
					{ID: 1, Name: "Unstarted Orphan"},
					{ID: 2, Name: "Started Orphan", StartedAt: pgtype.Timestamp{Time: time.Now(), Valid: true}},
					{ID: 3, ProjectID: &projectID1, Name: "Unstarted Child"},
					{ID: 4, ProjectID: &projectID1, Name: "Started Child", StartedAt: pgtype.Timestamp{Time: time.Now(), Valid: true}},
				}, nil
			},
		}
		repo := NewRepository(mock)

		got, err := repo.GetActiveTree(context.Background())
		require.NoError(t, err)

		// Root: project first, then started orphan, then unstarted orphan
		require.Len(t, got, 3)
		assert.Equal(t, "project", got[0].Type)
		assert.Equal(t, "Started Orphan", got[1].Name)
		assert.Equal(t, "Unstarted Orphan", got[2].Name)

		// Project children: started task first, then unstarted
		require.Len(t, got[0].Children, 2)
		assert.Equal(t, "Started Child", got[0].Children[0].Name)
		assert.Equal(t, "Unstarted Child", got[0].Children[1].Name)
	})

	t.Run("error from GetActiveProjects propagates", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetActiveTree(context.Background())
		assert.Error(t, err)
	})

	t.Run("error from GetUnfinishedTasks propagates", func(t *testing.T) {
		mock := &mockQuerier{
			getActiveProjectsFn: func(ctx context.Context) ([]sqlc.GetActiveProjectsRow, error) {
				return []sqlc.GetActiveProjectsRow{}, nil
			},
			getUnfinishedTasksFn: func(ctx context.Context) ([]sqlc.GetUnfinishedTasksRow, error) {
				return nil, errors.New("db error")
			},
		}
		repo := NewRepository(mock)

		_, err := repo.GetActiveTree(context.Background())
		assert.Error(t, err)
	})
}
