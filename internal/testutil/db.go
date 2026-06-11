package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(tb testing.TB) *pgxpool.Pool {
	tb.Helper()
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		tb.Skip("TEST_DB_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(pool.Close)
	return pool
}

func Truncate(tb testing.TB, pool *pgxpool.Pool, tables ...string) {
	tb.Helper()
	ctx := context.Background()
	for _, table := range tables {
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+table+" RESTART IDENTITY CASCADE"); err != nil {
			tb.Fatalf("truncate %s: %v", table, err)
		}
	}
}
