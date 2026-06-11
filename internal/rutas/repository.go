package rutas

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gv-api/internal/database/rutasdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: must be YYYY-MM-DD", s)
	}
	return t, nil
}

type Repository interface {
	List(ctx context.Context) ([]ConcelloMark, error)
	Get(ctx context.Context, id int32) (ConcelloMark, error)
	Create(ctx context.Context, req CreateMarkRequest) (ConcelloMark, error)
	Update(ctx context.Context, req UpdateMarkRequest) (ConcelloMark, error)
	Delete(ctx context.Context, id int32) error
}

type PostgresRepository struct {
	q *rutasdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: rutasdb.New(pool)}
}

func toDTO(m rutasdb.ConcelloMark) ConcelloMark {
	return ConcelloMark{
		ID:          m.ID,
		Name:        m.Name,
		VisitedOn:   m.VisitedOn,
		Description: m.Description,
	}
}

func (r *PostgresRepository) List(ctx context.Context) ([]ConcelloMark, error) {
	rows, err := r.q.ListConcelloMarks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ConcelloMark, len(rows))
	for i, row := range rows {
		out[i] = toDTO(row)
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id int32) (ConcelloMark, error) {
	row, err := r.q.GetConcelloMark(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConcelloMark{}, ErrNotFound
		}
		return ConcelloMark{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) Create(ctx context.Context, req CreateMarkRequest) (ConcelloMark, error) {
	date, err := parseDate(req.VisitedOn)
	if err != nil {
		return ConcelloMark{}, err
	}
	row, err := r.q.CreateConcelloMark(ctx, rutasdb.CreateConcelloMarkParams{
		Name:        req.Name,
		VisitedOn:   date,
		Description: req.Description,
	})
	if err != nil {
		return ConcelloMark{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, req UpdateMarkRequest) (ConcelloMark, error) {
	date, err := parseDate(req.VisitedOn)
	if err != nil {
		return ConcelloMark{}, err
	}
	row, err := r.q.UpdateConcelloMark(ctx, rutasdb.UpdateConcelloMarkParams{
		ID:          req.ID,
		VisitedOn:   date,
		Description: req.Description,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConcelloMark{}, ErrNotFound
		}
		return ConcelloMark{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	return r.q.DeleteConcelloMark(ctx, id)
}
