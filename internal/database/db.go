// Package database opens PostgreSQL pools and runs migrations.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options tune a pool; see NewWithOptions.
type Options struct {
	MaxConns int32
	MinConns int32
	// ReadOnly refuses writes on the connection itself (default_transaction_read_only).
	ReadOnly bool
	// Ping verifies connectivity before returning. Off for databases allowed to be down.
	Ping bool
}

func New(ctx context.Context, connectionString string) (*pgxpool.Pool, error) {
	return NewWithOptions(ctx, connectionString, Options{MaxConns: 25, MinConns: 5, Ping: true})
}

func NewWithOptions(ctx context.Context, connectionString string, opts Options) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	config.MaxConns = opts.MaxConns
	config.MaxConnLifetime = 5 * time.Minute
	config.MinConns = opts.MinConns
	if opts.ReadOnly {
		config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if opts.Ping {
		if err := pool.Ping(ctx); err != nil {
			return nil, fmt.Errorf("failed to ping pool: %w", err)
		}
	}

	return pool, nil
}
