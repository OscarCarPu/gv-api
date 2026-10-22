package uptime

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	TrackingStart(ctx context.Context) (time.Time, error)
	Ranges(ctx context.Context, from, to time.Time) ([]Range, error)
	Beat(ctx context.Context, now time.Time, gap time.Duration) error
	RecordFailover(ctx context.Context, lastAlive *time.Time, ec2UpAt, homeAt time.Time) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
	q    *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: gvdb.New(pool)}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func (r *PostgresRepository) TrackingStart(ctx context.Context) (time.Time, error) {
	t, err := r.q.OutagesStart(ctx)
	return t.Time, err
}

func (r *PostgresRepository) Ranges(ctx context.Context, from, to time.Time) ([]Range, error) {
	rows, err := r.q.ListOutageRanges(ctx, gvdb.ListOutageRangesParams{FromAt: ts(from), ToAt: ts(to)})
	if err != nil {
		return nil, err
	}
	out := make([]Range, 0, len(rows))
	for _, row := range rows {
		if !row.StartedAt.Valid {
			continue
		}
		out = append(out, Range{Kind: Kind(row.Kind), Start: row.StartedAt.Time, End: row.EndsAt.Time})
	}
	return out, nil
}

func (r *PostgresRepository) Beat(ctx context.Context, now time.Time, gap time.Duration) error {
	return r.inTx(ctx, func(q *gvdb.Queries) error {
		last, err := q.LastOutage(ctx)
		if err != nil {
			return err
		}
		if !now.After(last.EndsAt.Time) {
			return nil
		}
		switch {
		case last.Kind == gvdb.OutageKindUp && now.Sub(last.EndsAt.Time) <= gap:
			return q.ExtendOutage(ctx, gvdb.ExtendOutageParams{ID: last.ID, EndsAt: ts(now)})
		case last.Kind == gvdb.OutageKindUp:
			return q.AddOutage(ctx, gvdb.AddOutageParams{EndsAt: ts(now), Kind: gvdb.OutageKindDown})
		}
		return q.AddOutage(ctx, gvdb.AddOutageParams{EndsAt: ts(now), Kind: gvdb.OutageKindUp})
	})
}

func (r *PostgresRepository) RecordFailover(ctx context.Context, lastAlive *time.Time, ec2UpAt, homeAt time.Time) error {
	return r.inTx(ctx, func(q *gvdb.Queries) error {
		last, err := q.LastOutage(ctx)
		if err != nil {
			return err
		}
		if !last.EndsAt.Time.Before(homeAt) {
			return nil
		}

		start := last.EndsAt.Time
		if last.Kind == gvdb.OutageKindUp && lastAlive != nil && lastAlive.After(start) && lastAlive.Before(homeAt) {
			start = *lastAlive
			if err := q.ExtendOutage(ctx, gvdb.ExtendOutageParams{ID: last.ID, EndsAt: ts(start)}); err != nil {
				return err
			}
		}
		if start.Before(ec2UpAt) {
			if err := q.AddOutage(ctx, gvdb.AddOutageParams{EndsAt: ts(ec2UpAt), Kind: gvdb.OutageKindDown}); err != nil {
				return err
			}
		}
		return q.AddOutage(ctx, gvdb.AddOutageParams{EndsAt: ts(homeAt), Kind: gvdb.OutageKindFailover})
	})
}

func (r *PostgresRepository) inTx(ctx context.Context, fn func(*gvdb.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	//nolint:errcheck // nothing left to undo after a failed commit
	defer tx.Rollback(ctx)
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func LastAlive(ctx context.Context, pool *pgxpool.Pool) (*time.Time, error) {
	var t time.Time
	err := pool.QueryRow(ctx, "SELECT ends_at FROM outages WHERE kind = 'up' ORDER BY ends_at DESC LIMIT 1").Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
