// Package pipeline is gv-api's read-only connection to central-pipeline's dbt marts, shared by
// every domain that reads one.
//
// It has its own DSN and pool, is read-only on the connection itself, and runs no migrations.
// An unset DSN is normal: reads report ErrNotConfigured and the domain answers 503.
package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gv-api/internal/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotConfigured means no pipeline database is wired up; handlers answer 503.
var ErrNotConfigured = errors.New("pipeline database not configured")

// DB is the shared connection to central-pipeline. The zero DB is unconfigured and safe to use.
type DB struct {
	pool *pgxpool.Pool
}

// Connect opens the pool. An empty DSN returns an unconfigured DB. Connectivity is not verified;
// the pool reconnects once the pipeline is up.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		slog.Warn("PIPELINE_DATABASE_URL not set: pipeline-backed endpoints will answer 503")
		return &DB{}, nil
	}
	pool, err := database.NewWithOptions(ctx, dsn, database.Options{
		MaxConns: 5,
		MinConns: 0,
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func FromPool(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

func (db *DB) Configured() bool {
	return db != nil && db.pool != nil
}

func (db *DB) Close() {
	if db.Configured() {
		db.pool.Close()
	}
}

// Collect runs a query and scans every row. Returns an empty slice, never nil.
func Collect[T any](ctx context.Context, db *DB, scan func(pgx.Rows) (T, error), sql string, args ...any) ([]T, error) {
	if !db.Configured() {
		return nil, ErrNotConfigured
	}
	return retryRebuild(ctx, func(ctx context.Context) ([]T, error) {
		rows, err := db.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := []T{}
		for rows.Next() {
			item, err := scan(rows)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	})
}

// A dbt run drops and recreates the marts, so a read can briefly hit a missing relation.
const (
	rebuildRetries = 2
	rebuildBackoff = 200 * time.Millisecond
)

// retryRebuild retries fn while the error looks like a relation being swapped under it.
func retryRebuild[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var (
		out T
		err error
	)
	for attempt := 0; ; attempt++ {
		out, err = fn(ctx)
		if err == nil || attempt == rebuildRetries || !isRebuildError(err) {
			return out, err
		}
		select {
		case <-ctx.Done():
			return out, err
		case <-time.After(rebuildBackoff):
		}
	}
}

func isRebuildError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "42P01", // undefined_table: dropped between plan and read
		"3F000", // invalid_schema_name: the whole schema is being rebuilt
		"40001", // serialization_failure
		"40P01", // deadlock_detected
		"55P03": // lock_not_available
		return true
	}
	return false
}

// IsStale reports whether a mart's computed-at marker is too old to present as current. Nothing
// computed yet counts as stale.
func IsStale(computedAt *time.Time, after time.Duration) bool {
	return computedAt == nil || time.Since(*computedAt) > after
}
