package backup_test

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gv-api/internal/backup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const day = 24 * time.Hour

type fakeDumper struct {
	out string
	err error
}

func (d fakeDumper) Dump(_ context.Context, w io.Writer) error {
	if d.err != nil {
		return d.err
	}
	_, err := io.WriteString(w, d.out)
	return err
}

type fakeLocker struct{ err error }

func (l fakeLocker) TryLock(context.Context) (func(), error) {
	if l.err != nil {
		return nil, l.err
	}
	return func() {}, nil
}

func newService(t *testing.T, d backup.Dumper, l backup.Locker) (*backup.Service, string) {
	t.Helper()
	dir := t.TempDir()
	return backup.NewService(d, l, backup.Config{Dir: dir, KeepHourly: 2 * day, KeepDaily: 30 * day}), dir
}

func nameAt(at time.Time) string {
	return "gv-db-" + at.UTC().Format("20060102T150405Z") + ".sql.gz"
}

func touch(t *testing.T, dir, sub, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, sub), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, sub, name), []byte("x"), 0o600))
}

func backups(t *testing.T, dir, sub string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, sub, "gv-db-*T*Z.sql.gz"))
	require.NoError(t, err)
	out := []string{}
	for i := len(matches) - 1; i >= 0; i-- {
		out = append(out, filepath.Base(matches[i]))
	}
	return out
}

func TestService_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("writes the dump gzipped into hourly/", func(t *testing.T) {
		svc, dir := newService(t, fakeDumper{out: "CREATE TABLE x();"}, fakeLocker{})

		b, err := svc.Run(ctx)
		require.NoError(t, err)

		f, err := os.Open(filepath.Join(dir, "hourly", b.Name))
		require.NoError(t, err)
		defer f.Close()
		gz, err := gzip.NewReader(f)
		require.NoError(t, err)
		body, err := io.ReadAll(gz)
		require.NoError(t, err)
		assert.Equal(t, "CREATE TABLE x();", string(body))
	})

	t.Run("only the first backup of the day is copied to daily/", func(t *testing.T) {
		svc, dir := newService(t, fakeDumper{out: "x"}, fakeLocker{})
		earlier := nameAt(time.Now().Add(-time.Second))
		touch(t, dir, "hourly", earlier)

		first, err := svc.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{first.Name}, backups(t, dir, "daily"))

		_, err = svc.Run(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{first.Name}, backups(t, dir, "daily"))

		list, err := svc.List()
		require.NoError(t, err)
		assert.Len(t, list, len(backups(t, dir, "hourly")))
	})

	t.Run("deletes by age per folder and leaves other files alone", func(t *testing.T) {
		svc, dir := newService(t, fakeDumper{out: "x"}, fakeLocker{})
		now := time.Now()

		recent := nameAt(now.Add(-time.Hour))
		staleHourly := nameAt(now.Add(-3 * day))
		recentDaily := nameAt(now.Add(-5 * day))
		staleDaily := nameAt(now.Add(-40 * day))
		touch(t, dir, "hourly", recent)
		touch(t, dir, "hourly", staleHourly)
		touch(t, dir, "daily", recentDaily)
		touch(t, dir, "daily", staleDaily)
		touch(t, dir, "daily", "gv-db-20260926.sql.gz")

		b, err := svc.Run(ctx)
		require.NoError(t, err)

		assert.Equal(t, []string{b.Name, recent}, backups(t, dir, "hourly"))
		assert.Equal(t, []string{b.Name, recentDaily}, backups(t, dir, "daily"))
		assert.FileExists(t, filepath.Join(dir, "daily", "gv-db-20260926.sql.gz"))

		list, err := svc.List()
		require.NoError(t, err)
		var names []string
		for _, l := range list {
			names = append(names, l.Name)
		}
		assert.Equal(t, []string{b.Name, recent, recentDaily}, names)
	})

	t.Run("a second run in the same second returns the existing backup", func(t *testing.T) {
		svc, dir := newService(t, fakeDumper{out: "new"}, fakeLocker{})
		now := time.Now()
		for i := range 3 {
			touch(t, dir, "hourly", nameAt(now.Add(time.Duration(i)*time.Second)))
		}

		b, err := svc.Run(ctx)
		require.NoError(t, err)

		body, err := os.ReadFile(filepath.Join(dir, "hourly", b.Name))
		require.NoError(t, err)
		assert.Equal(t, "x", string(body))
		assert.Len(t, backups(t, dir, "hourly"), 3)
	})

	t.Run("a failed dump leaves no file and deletes nothing", func(t *testing.T) {
		svc, dir := newService(t, fakeDumper{err: errors.New("boom")}, fakeLocker{})
		touch(t, dir, "hourly", nameAt(time.Now().Add(-40*day)))
		touch(t, dir, "hourly", nameAt(time.Now().Add(-41*day)))

		_, err := svc.Run(ctx)
		assert.ErrorContains(t, err, "boom")

		entries, err := os.ReadDir(filepath.Join(dir, "hourly"))
		require.NoError(t, err)
		assert.Len(t, entries, 2)
	})

	t.Run("ErrRunning when the lock is held elsewhere", func(t *testing.T) {
		svc, _ := newService(t, fakeDumper{}, fakeLocker{err: backup.ErrRunning})

		_, err := svc.Run(ctx)
		assert.ErrorIs(t, err, backup.ErrRunning)
	})
}

func TestService_Path(t *testing.T) {
	svc, dir := newService(t, fakeDumper{}, fakeLocker{})
	inHourly := "gv-db-20260102T000000Z.sql.gz"
	onlyDaily := "gv-db-20260101T000000Z.sql.gz"
	touch(t, dir, "hourly", inHourly)
	touch(t, dir, "daily", inHourly)
	touch(t, dir, "daily", onlyDaily)

	_, err := svc.Path("../.env")
	assert.ErrorIs(t, err, backup.ErrInvalidName)

	_, err = svc.Path("gv-db-20260103T000000Z.sql.gz")
	assert.ErrorIs(t, err, backup.ErrNotFound)

	path, err := svc.Path(inHourly)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "hourly", inHourly), path)

	path, err = svc.Path(onlyDaily)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "daily", onlyDaily), path)
}
