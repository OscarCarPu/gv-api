// Package pipeline is gv-api's read-only side of central-pipeline.
//
// central-pipeline collects what the devices around the house publish over MQTT and models
// it with dbt into marts in its own PostgreSQL instance. Any gv-api domain reading one of
// those marts — uptime today, whatever the next device turns out to be — goes through the
// single connection here rather than opening its own.
//
// It is deliberately not gv's database and deliberately not gv's schema:
//
//   - Separate DSN and separate pool. The two servers are different servers, on different
//     ports, and the pipeline stack is allowed to be down while gv-api runs.
//   - Read-only on the connection itself, not only by grant. dbt owns these relations;
//     nothing on this side may write to them.
//   - No migrations, no views, no long-lived prepared statements built on top. Every dbt
//     run drops and recreates the marts, which is also why reads retry instead of
//     surfacing the moment a relation was missing.
//   - Unset DSN is a normal state, not a failure: reads report ErrNotConfigured and the
//     domain answers 503 while the rest of the app is unaffected.
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

// ErrNotConfigured means no pipeline database is wired up. Domains reading a mart should
// let it through so their handler can answer 503: it is a deployment state, not a fault.
var ErrNotConfigured = errors.New("pipeline database not configured")

// DB is the shared connection to central-pipeline. A zero DB is the not-configured one and
// is safe to use: every read reports ErrNotConfigured.
type DB struct {
	pool *pgxpool.Pool
}

// Connect opens the pool. An empty DSN returns a usable, unconfigured DB rather than an
// error, so wiring does not have to branch. Connectivity is not verified: the pipeline is a
// separate stack that may well be down when this one starts, and the pool reconnects on its
// own once it is back.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		slog.Warn("PIPELINE_DATABASE_URL not set: pipeline-backed endpoints will answer 503")
		return &DB{}, nil
	}
	pool, err := database.NewWithOptions(ctx, dsn, database.Options{
		// A handful of connections is plenty for dashboard reads, and none are kept warm
		// against a database that is allowed to be absent.
		MaxConns: 5,
		MinConns: 0,
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

// FromPool wraps an existing pool. For tests, and for anything that already holds one.
func FromPool(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Configured says whether there is a database behind this DB at all.
func (db *DB) Configured() bool {
	return db != nil && db.pool != nil
}

func (db *DB) Close() {
	if db.Configured() {
		db.pool.Close()
	}
}

// Collect runs a query and scans every row with scan. It is the only way domains read from
// here, so the retry below covers all of them at once. Returns an empty slice, never nil,
// so a JSON response carries `[]` rather than `null`.
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

// A dbt run drops and recreates the marts, so a read can land in the gap where a relation
// does not exist. That window is milliseconds; retrying beats surfacing a 500 for it.
const (
	rebuildRetries = 2
	rebuildBackoff = 200 * time.Millisecond
)

// retryRebuild runs fn again while the error looks like a relation being swapped under it.
// Anything else is returned as it is.
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

// IsStale reports whether a mart's computed-at marker is too old to present as current.
// Every mart carries the dbt run time rather than now(), and dbt is a batch job, so this is
// the question each pipeline-backed domain has to answer. Nothing computed yet counts as
// stale: there is no run to call fresh.
func IsStale(computedAt *time.Time, after time.Duration) bool {
	return computedAt == nil || time.Since(*computedAt) > after
}
