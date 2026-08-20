package pipeline_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/pipeline"
	"gv-api/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnconfigured(t *testing.T) {
	// An unset DSN is a normal deployment state: Connect succeeds, and it is the read that
	// says there is nothing behind it.
	db, err := pipeline.Connect(context.Background(), "")
	require.NoError(t, err)
	assert.False(t, db.Configured())

	_, err = pipeline.Collect(context.Background(), db, func(pgx.Rows) (int, error) { return 0, nil }, "SELECT 1")
	assert.ErrorIs(t, err, pipeline.ErrNotConfigured)

	db.Close() // must not panic without a pool
}

func TestConnect_RejectsUnparseableDSN(t *testing.T) {
	_, err := pipeline.Connect(context.Background(), "not-a-dsn")
	assert.Error(t, err)
}

func TestIsStale(t *testing.T) {
	fresh := time.Now().Add(-time.Minute)
	old := time.Now().Add(-3 * time.Hour)

	assert.False(t, pipeline.IsStale(&fresh, time.Hour))
	assert.True(t, pipeline.IsStale(&old, time.Hour))
	assert.True(t, pipeline.IsStale(nil, time.Hour), "nothing computed yet cannot be fresh")
}

func TestIntegration_Collect(t *testing.T) {
	// The pipeline's own database is another project's stack, so this runs against gv's
	// test database: what is under test is Collect itself, not any particular mart.
	db := pipeline.FromPool(testutil.NewPool(t))
	ctx := context.Background()

	got, err := pipeline.Collect(ctx, db, func(rows pgx.Rows) (int, error) {
		var n int
		err := rows.Scan(&n)
		return n, err
	}, "SELECT * FROM generate_series(1, $1)", 3)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, got)

	// Empty is a slice, not nil, so responses carry [] instead of null.
	got, err = pipeline.Collect(ctx, db, func(rows pgx.Rows) (int, error) {
		var n int
		err := rows.Scan(&n)
		return n, err
	}, "SELECT * FROM generate_series(1, 0)")
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestIntegration_Collect_MissingRelationSurfacesAfterRetries(t *testing.T) {
	// A dropped relation is what a dbt rebuild looks like, so it is retried; a table that
	// is missing for good still has to fail rather than hang.
	db := pipeline.FromPool(testutil.NewPool(t))

	start := time.Now()
	_, err := pipeline.Collect(context.Background(), db, func(pgx.Rows) (int, error) { return 0, nil },
		"SELECT 1 FROM marts.does_not_exist")
	assert.Error(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 400*time.Millisecond, "two retries at 200ms")
}

func TestIntegration_ReadOnlyConnection(t *testing.T) {
	// The grant on the far side is the pipeline's business; this pins that gv-api's own
	// connection refuses a write even if the role would allow one.
	url := testutil.DSN(t)
	db, err := pipeline.Connect(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(db.Close)

	_, err = pipeline.Collect(context.Background(), db, func(pgx.Rows) (int, error) { return 0, nil },
		"CREATE TABLE pipeline_write_probe (id int)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
}
