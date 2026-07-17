package assistant

import (
	"context"
	"testing"

	"gv-api/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestReadExecutor_Integration_SelectWorks(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 200, 40)

	res, err := ex.Run(context.Background(), "SELECT 1 AS n, 'hi' AS greeting")
	require.NoError(t, err)
	require.Equal(t, []string{"n", "greeting"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.False(t, res.Truncated)
}

func TestReadExecutor_Integration_RejectsNonSelect(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 200, 40)

	for _, sql := range []string{
		"INSERT INTO habits (name) VALUES ('x')",
		"UPDATE habits SET name='x'",
		"DELETE FROM habits",
		"DROP TABLE habits",
	} {
		_, err := ex.Run(context.Background(), sql)
		require.ErrorIs(t, err, ErrUnsafeSQL, "sql: %s", sql)
	}
}

func TestReadExecutor_Integration_RejectsMultiStatement(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 200, 40)

	_, err := ex.Run(context.Background(), "SELECT 1; DROP TABLE habits")
	require.ErrorIs(t, err, ErrUnsafeSQL)
}

// A writing CTE passes the static check (first keyword WITH) but the read-only
// transaction must reject it at the engine level — proving the tx, not the
// static check, is the real guarantee.
func TestReadExecutor_Integration_ReadOnlyTxBlocksWritingCTE(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 200, 40)

	_, err := ex.Run(context.Background(),
		"WITH x AS (INSERT INTO habits(name) VALUES ('should-not-persist') RETURNING id) SELECT * FROM x")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrUnsafeSQL, "should fail at execution, not static validation")

	// And nothing was written.
	res, err := ex.Run(context.Background(), "SELECT count(*) AS n FROM habits WHERE name = 'should-not-persist'")
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	require.EqualValues(t, 0, res.Rows[0][0])
}

func TestReadExecutor_Integration_RowCapTruncates(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 2, 40)

	res, err := ex.Run(context.Background(), "SELECT g FROM generate_series(1, 10) AS g")
	require.NoError(t, err)
	require.Len(t, res.Rows, 2)
	require.True(t, res.Truncated)
}

func TestReadExecutor_Integration_ColumnCap(t *testing.T) {
	pool := testutil.NewPool(t)
	ex := NewReadExecutor(pool, 3000, 200, 2)

	_, err := ex.Run(context.Background(), "SELECT 1 AS a, 2 AS b, 3 AS c")
	require.ErrorIs(t, err, ErrReadTooLarge)
}
