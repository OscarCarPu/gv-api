package backup_test

import (
	"bytes"
	"context"
	"os/exec"
	"testing"

	"gv-api/internal/backup"
	"gv-api/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_PgLocker(t *testing.T) {
	ctx := context.Background()
	locker := backup.NewPgLocker(testutil.NewPool(t))

	unlock, err := locker.TryLock(ctx)
	require.NoError(t, err)

	_, err = locker.TryLock(ctx)
	assert.ErrorIs(t, err, backup.ErrRunning)

	unlock()
	unlock, err = locker.TryLock(ctx)
	require.NoError(t, err)
	unlock()
}

func TestIntegration_PgDump(t *testing.T) {
	dsn := testutil.DSN(t)
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not installed")
	}

	var out bytes.Buffer
	require.NoError(t, backup.PgDump{URL: dsn}.Dump(context.Background(), &out))
	assert.Contains(t, out.String(), "PostgreSQL database dump")
}
