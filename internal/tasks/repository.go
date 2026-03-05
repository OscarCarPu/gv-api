package tasks

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error)
	CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error)
	CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error)
	CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error)
	FinishTimeEntry(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error)
	FinishTask(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error)
	FinishProject(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error)
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

func (r *PostgresRepository) CreateTask(ctx context.Context, projectID *int32, name string, description *string, dueAt *time.Time) (TaskResponse, error) {
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
		return TaskResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	return TaskResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
	}, nil
}

func (r *PostgresRepository) CreateTodo(ctx context.Context, taskID int32, name string) (TodoResponse, error) {
	row, err := r.q.CreateTodo(ctx, sqlc.CreateTodoParams{
		TaskID: taskID,
		Name:   name,
	})
	if err != nil {
		return TodoResponse{}, err
	}

	return TodoResponse{
		ID:     row.ID,
		TaskID: row.TaskID,
		Name:   row.Name,
	}, nil
}

func (r *PostgresRepository) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
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
		return TimeEntryResponse{}, err
	}

	var respFinishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		respFinishedAt = &t
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: respFinishedAt,
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) FinishTimeEntry(ctx context.Context, id int32, finishedAt time.Time) (TimeEntryResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishTimeEntry(ctx, sqlc.FinishTimeEntryParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimeEntryResponse{}, ErrNotFound
		}
		return TimeEntryResponse{}, err
	}

	var respFinishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		respFinishedAt = &t
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: respFinishedAt,
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) FinishTask(ctx context.Context, id int32, finishedAt time.Time) (TaskResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishTask(ctx, sqlc.FinishTaskParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskResponse{}, ErrNotFound
		}
		return TaskResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	var respStartedAt *time.Time
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		respStartedAt = &t
	}

	var respFinishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		respFinishedAt = &t
	}

	return TaskResponse{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
		StartedAt:   respStartedAt,
		FinishedAt:  respFinishedAt,
	}, nil
}

func (r *PostgresRepository) FinishProject(ctx context.Context, id int32, finishedAt time.Time) (ProjectResponse, error) {
	pgFinishedAt := pgtype.Timestamp{Time: finishedAt, Valid: true}

	row, err := r.q.FinishProject(ctx, sqlc.FinishProjectParams{
		ID:         id,
		FinishedAt: pgFinishedAt,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectResponse{}, ErrNotFound
		}
		return ProjectResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	var respStartedAt *time.Time
	if row.StartedAt.Valid {
		t := row.StartedAt.Time
		respStartedAt = &t
	}

	var respFinishedAt *time.Time
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		respFinishedAt = &t
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
		ParentID:    row.ParentID,
		StartedAt:   respStartedAt,
		FinishedAt:  respFinishedAt,
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
