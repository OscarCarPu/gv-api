package tasks

import (
	"context"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (CreateTaskResponse, error)
	CreateTodo(ctx context.Context, taskID int32, name string) (CreateTodoResponse, error)
	CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (CreateTimeEntryResponse, error)
	GetRootProjects(ctx context.Context) ([]ProjectResponse, error)
}

type PostgresRepository struct {
	q sqlc.Querier
}

func NewRepository(q sqlc.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateProject(ctx, sqlc.CreateProjectParams{
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
		ParentID:    parentID,
	})
	if err != nil {
		return ProjectResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
		ParentID:    row.ParentID,
	}, nil
}

func (r *PostgresRepository) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (CreateTaskResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateTask(ctx, sqlc.CreateTaskParams{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
	})
	if err != nil {
		return CreateTaskResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	return CreateTaskResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
	}, nil
}

func (r *PostgresRepository) CreateTodo(ctx context.Context, taskID int32, name string) (CreateTodoResponse, error) {
	row, err := r.q.CreateTodo(ctx, sqlc.CreateTodoParams{
		TaskID: taskID,
		Name:   name,
	})
	if err != nil {
		return CreateTodoResponse{}, err
	}

	return CreateTodoResponse{
		ID:     row.ID,
		TaskID: row.TaskID,
		Name:   row.Name,
	}, nil
}

func (r *PostgresRepository) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (CreateTimeEntryResponse, error) {
	pgStartedAt := pgtype.Timestamp{Time: startedAt, Valid: true}

	var pgFinishedAt pgtype.Timestamp
	if finishedAt != nil {
		pgFinishedAt = pgtype.Timestamp{Time: *finishedAt, Valid: true}
	}

	row, err := r.q.CreateTimeEntry(ctx, sqlc.CreateTimeEntryParams{
		TaskID:     taskID,
		StartedAt:  pgStartedAt,
		FinishedAt: pgFinishedAt,
		Comment:    comment,
	})
	if err != nil {
		return CreateTimeEntryResponse{}, err
	}

	var respFinishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		respFinishedAt = &t
	}

	return CreateTimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: respFinishedAt,
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	rows, err := r.q.GetRootProjects(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectResponse, len(rows))
	for i, row := range rows {
		var respDueAt *time.Time
		if row.DueAt.Valid {
			t := row.DueAt.Time
			respDueAt = &t
		}

		projects[i] = ProjectResponse{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			DueAt:       respDueAt,
			ParentID:    row.ParentID,
		}
	}

	return projects, nil
}
