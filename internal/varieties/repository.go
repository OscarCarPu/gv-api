package varieties

import (
	"context"
	"errors"

	"gv-api/internal/database/varietiesdb"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Get(ctx context.Context, id int32) (Variety, error)
	List(ctx context.Context) ([]Variety, error)
	Create(ctx context.Context, req CreateVarietyRequest) (Variety, error)
	Update(ctx context.Context, req UpdateVarietyRequest) (Variety, error)
	Delete(ctx context.Context, id int32) error
}

type PostgresRepository struct {
	q varietiesdb.Querier
}

func NewRepository(q varietiesdb.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func toDTO(v varietiesdb.WeedVariety) Variety {
	return Variety{
		ID:       v.ID,
		Name:     v.Name,
		Scent:    v.Scent,
		Flavor:   v.Flavor,
		Power:    v.Power,
		Quality:  v.Quality,
		Score:    v.Score,
		Price:    v.Price,
		Comments: v.Comments,
	}
}

func (r *PostgresRepository) Get(ctx context.Context, id int32) (Variety, error) {
	row, err := r.q.GetVariety(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Variety{}, ErrNotFound
		}
		return Variety{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Variety, error) {
	rows, err := r.q.ListVarieties(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]Variety, len(rows))
	for i, row := range rows {
		results[i] = toDTO(row)
	}
	return results, nil
}

func (r *PostgresRepository) Create(ctx context.Context, req CreateVarietyRequest) (Variety, error) {
	row, err := r.q.CreateVariety(ctx, varietiesdb.CreateVarietyParams{
		Name:     req.Name,
		Scent:    req.Scent,
		Flavor:   req.Flavor,
		Power:    req.Power,
		Quality:  req.Quality,
		Price:    req.Price,
		Comments: req.Comments,
	})
	if err != nil {
		return Variety{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, req UpdateVarietyRequest) (Variety, error) {
	row, err := r.q.UpdateVariety(ctx, varietiesdb.UpdateVarietyParams{
		ID:       req.ID,
		Name:     req.Name,
		Scent:    req.Scent,
		Flavor:   req.Flavor,
		Power:    req.Power,
		Quality:  req.Quality,
		Price:    req.Price,
		Comments: req.Comments,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Variety{}, ErrNotFound
		}
		return Variety{}, err
	}
	return toDTO(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	return r.q.DeleteVariety(ctx, id)
}
