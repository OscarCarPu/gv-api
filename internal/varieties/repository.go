package varieties

import (
	"context"
	"errors"
	"fmt"

	"gv-api/internal/actor"
	"gv-api/internal/database/varietiesdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool *pgxpool.Pool
	q    *varietiesdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: varietiesdb.New(pool)}
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
		Judge:    v.Judge,
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

// withActorTx runs fn inside a transaction with the actor identifiers from
// ctx pinned via SET LOCAL, so the audit trigger can stamp them onto the
// history row.
func (r *PostgresRepository) withActorTx(ctx context.Context, fn func(*varietiesdb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	info := actor.FromContext(ctx)
	settings := []struct{ key, val string }{
		{"app.actor_ip", info.IP},
		{"app.actor_user_agent", info.UserAgent},
		{"app.actor_device_id", info.DeviceID},
		{"app.actor_token_kind", info.TokenKind},
	}
	for _, s := range settings {
		if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", s.key, s.val); err != nil {
			return fmt.Errorf("set_config %s: %w", s.key, err)
		}
	}

	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) Create(ctx context.Context, req CreateVarietyRequest) (Variety, error) {
	var out Variety
	err := r.withActorTx(ctx, func(q *varietiesdb.Queries) error {
		row, err := q.CreateVariety(ctx, varietiesdb.CreateVarietyParams{
			Name:     req.Name,
			Scent:    req.Scent,
			Flavor:   req.Flavor,
			Power:    req.Power,
			Quality:  req.Quality,
			Price:    req.Price,
			Comments: req.Comments,
			Judge:    req.Judge,
		})
		if err != nil {
			return err
		}
		out = toDTO(row)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) Update(ctx context.Context, req UpdateVarietyRequest) (Variety, error) {
	var out Variety
	err := r.withActorTx(ctx, func(q *varietiesdb.Queries) error {
		row, err := q.UpdateVariety(ctx, varietiesdb.UpdateVarietyParams{
			ID:       req.ID,
			Name:     req.Name,
			Scent:    req.Scent,
			Flavor:   req.Flavor,
			Power:    req.Power,
			Quality:  req.Quality,
			Price:    req.Price,
			Comments: req.Comments,
			Judge:    req.Judge,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		out = toDTO(row)
		return nil
	})
	return out, err
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	return r.withActorTx(ctx, func(q *varietiesdb.Queries) error {
		return q.DeleteVariety(ctx, id)
	})
}
