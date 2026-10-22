package restore_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gv-api/internal/restore"
	"gv-api/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const schema = `
CREATE TABLE parents  (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE children (id SERIAL PRIMARY KEY, parent_id INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE, label TEXT NOT NULL);
INSERT INTO parents (name) VALUES ('one'), ('two'), ('three');
INSERT INTO children (parent_id, label) VALUES (1, 'c1'), (2, 'c2');
`

const homeChanges = `
INSERT INTO parents (name) VALUES ('home-new');
UPDATE parents SET name = 'home-edit' WHERE id = 1;
UPDATE parents SET name = 'home-clash' WHERE id = 2;
`

const ec2Changes = `
INSERT INTO parents (name) VALUES ('ec2-new');
UPDATE parents SET name = 'ec2-clash' WHERE id = 2;
`

var (
	homeNames = []string{"home-edit", "home-clash", "three", "home-new"}
	ec2Names  = []string{"one", "ec2-clash", "three", "ec2-new"}
)

type env struct {
	admin *pgxpool.Pool
	home  string
	dir   string
	url   func(string) string
	base  string
	ec2   string
}

func setup(t *testing.T, homeSQL string) *env {
	t.Helper()
	e := &env{admin: testutil.NewPool(t), dir: t.TempDir()}
	e.home = fmt.Sprintf("restore_%d", time.Now().UnixNano())
	e.url = func(db string) string {
		u, err := url.Parse(testutil.DSN(t))
		require.NoError(t, err)
		u.Path = "/" + db
		return u.String()
	}

	ctx := context.Background()
	_, err := e.admin.Exec(ctx, "CREATE DATABASE "+e.home)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, db := range []string{e.home, e.home + "_restore_base", e.home + "_restore_ec2", e.home + "_pre_restore"} {
			_, _ = e.admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+db+`" WITH (FORCE)`)
		}
	})
	if homeSQL != "" {
		e.exec(t, e.home, homeSQL)
	}
	e.base = e.dump(t, "base.sql.gz", schema)
	e.ec2 = e.dump(t, "ec2.sql.gz", schema+ec2Changes)
	return e
}

func (e *env) pool(t *testing.T, db string) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(context.Background(), e.url(db))
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func (e *env) exec(t *testing.T, db, sql string) {
	t.Helper()
	_, err := e.pool(t, db).Exec(context.Background(), sql)
	require.NoError(t, err)
}

func (e *env) names(t *testing.T, db string) []string {
	t.Helper()
	rows, err := e.pool(t, db).Query(context.Background(), "SELECT name FROM parents ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	return out
}

func (e *env) dump(t *testing.T, name, sql string) string {
	t.Helper()
	path := filepath.Join(e.dir, name)
	f, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	_, err = gz.Write([]byte(sql))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	return path
}

func (e *env) runner(released *bool) *restore.Runner {
	return restore.New(restore.Config{
		Admin: e.admin, URL: e.url, Home: e.home, Dir: filepath.Join(e.dir, "failback"),
		Release: func() {
			if released != nil {
				*released = true
			}
		},
	})
}

func (e *env) request() restore.Request {
	return restore.Request{ID: "dump-1", Base: e.base, EC2: e.ec2}
}

func TestRun_RestoresEC2AndReportsWhatHomeHad(t *testing.T) {
	e := setup(t, schema+homeChanges)
	var released bool

	out, err := e.runner(&released).Run(context.Background(), e.request())

	require.NoError(t, err)
	assert.True(t, out.Swapped)
	assert.True(t, released, "the app's pool is released before the swap")
	assert.Equal(t, ec2Names, e.names(t, e.home), "EC2's data is restored as it is")
	assert.Equal(t, homeNames, e.names(t, e.home+"_pre_restore"), "the old home database is kept")
	assert.FileExists(t, filepath.Join(e.dir, "failback", "swapped-dump-1"))

	require.Empty(t, out.Report.Error)
	var seen []string
	for _, it := range out.Report.Items {
		seen = append(seen, fmt.Sprint(it.Op, " ", it.Home["name"]))
	}
	assert.ElementsMatch(t, []string{"insert home-new", "update home-edit", "update home-clash"}, seen)

	f, err := os.Open(out.PreRestore)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	body, err := io.ReadAll(gz)
	require.NoError(t, err)
	assert.Contains(t, string(body), "home-new", "the pre-restore dump holds what home had")

	for _, scratch := range []string{"_restore_base", "_restore_ec2"} {
		var n int
		require.NoError(t, e.admin.QueryRow(context.Background(),
			"SELECT count(*) FROM pg_database WHERE datname = $1", e.home+scratch).Scan(&n))
		assert.Zero(t, n, "scratch database %s is gone", scratch)
	}
}

func TestRun_SwapsEvenWhileTheAppKeepsReconnecting(t *testing.T) {
	e := setup(t, schema+homeChanges)
	// a leftover from an earlier restore makes the drop before the rename take its time
	_, err := e.admin.Exec(context.Background(), "CREATE DATABASE "+e.home+"_pre_restore")
	require.NoError(t, err)
	ctx, stop := context.WithCancel(context.Background())
	var clients sync.WaitGroup
	var connects atomic.Int64
	for range 8 {
		clients.Add(1)
		go func() {
			defer clients.Done()
			for ctx.Err() == nil {
				if conn, err := pgx.Connect(ctx, e.url(e.home)); err == nil {
					connects.Add(1)
					_, _ = conn.Exec(ctx, "SELECT pg_sleep(0.2)")
					_ = conn.Close(ctx)
				}
			}
		}()
	}
	t.Cleanup(func() { stop(); clients.Wait() })

	out, err := e.runner(nil).Run(context.Background(), e.request())

	require.NoError(t, err)
	assert.True(t, out.Swapped)
	assert.Equal(t, ec2Names, e.names(t, e.home))
	for _, db := range []string{e.home, e.home + "_pre_restore"} {
		var allowed bool
		require.NoError(t, e.admin.QueryRow(context.Background(),
			"SELECT datallowconn FROM pg_database WHERE datname = $1", db).Scan(&allowed))
		assert.True(t, allowed, "%s accepts connections again", db)
	}
	assert.Greater(t, connects.Load(), int64(8), "the clients must have been reconnecting")
}

func TestRun_AnEmptyHomeTakesEC2AsIs(t *testing.T) {
	e := setup(t, "")

	out, err := e.runner(nil).Run(context.Background(), e.request())

	require.NoError(t, err)
	assert.True(t, out.Swapped)
	assert.Empty(t, out.Report.Items)
	assert.Equal(t, ec2Names, e.names(t, e.home))
}

func TestRun_AReportThatCannotBeBuiltDoesNotStopTheRestore(t *testing.T) {
	e := setup(t, schema+homeChanges)
	e.base = e.dump(t, "base.sql.gz", "CREATE TABLE unrelated (id INT PRIMARY KEY);")

	out, err := e.runner(nil).Run(context.Background(), e.request())

	require.NoError(t, err)
	assert.True(t, out.Swapped)
	assert.NotEmpty(t, out.Report.Error)
	assert.Equal(t, ec2Names, e.names(t, e.home))
}

func TestRun_ATruncatedDumpChangesNothing(t *testing.T) {
	e := setup(t, schema+homeChanges)
	raw, err := os.ReadFile(e.ec2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(e.ec2, raw[:len(raw)/2], 0o600))

	_, err = e.runner(nil).Run(context.Background(), e.request())

	assert.ErrorIs(t, err, restore.ErrBadDump)
	assert.Equal(t, homeNames, e.names(t, e.home))
	assert.NoDirExists(t, filepath.Join(e.dir, "failback"), "nothing was written")
}

func TestRun_ARepeatedRunIsAlreadyDone(t *testing.T) {
	e := setup(t, schema+homeChanges)
	_, err := e.runner(nil).Run(context.Background(), e.request())
	require.NoError(t, err)

	out, err := e.runner(nil).Run(context.Background(), e.request())

	require.NoError(t, err)
	assert.True(t, out.AlreadyDone)
	assert.Nil(t, out.Report)
	assert.Equal(t, ec2Names, e.names(t, e.home))
}
