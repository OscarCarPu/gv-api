package tasks

import (
	"context"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

type Repository interface {
	CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (CreateProjectResponse, error)
}

type PostgresRepository struct {
	q sqlc.Querier
}

func NewRepository(q sqlc.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (CreateProjectResponse, error) {
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
		return CreateProjectResponse{}, err
	}

	var respDueAt *time.Time
	if row.DueAt.Valid {
		t := row.DueAt.Time
		respDueAt = &t
	}

	return CreateProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       respDueAt,
		ParentID:    row.ParentID,
	}, nil
}
