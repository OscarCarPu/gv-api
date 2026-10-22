package restore_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gv-api/internal/merge"
	"gv-api/internal/restore"
	"gv-api/internal/testutil"

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

const offsetSequences = `
SELECT setval(format('%I.%I', schemaname, sequencename)::regclass, coalesce(last_value, start_value) + 1000000)
FROM pg_sequences WHERE schemaname = 'public';
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
		for _, db := range []string{e.home, e.home + "_merge_base", e.home + "_merge_ec2", e.home + "_pre_restore"} {
			_, _ = e.admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+db+`" WITH (FORCE)`)
		}
	})
	if homeSQL != "" {
		e.exec(t, e.home, homeSQL)
	}

	e.base = e.dump(t, "base.sql.gz", schema)
	e.ec2 = e.dump(t, "ec2.sql.gz", schema+offsetSequences+ec2Changes)
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

func (e *env) request(commit bool, decisions map[merge.Ref]merge.Choice) restore.Request {
	return restore.Request{ID: "dump-1", Base: e.base, EC2: e.ec2, Decisions: decisions, Commit: commit}
}

var clash = merge.Ref{Table: "parents", Key: "[2]"}

func TestRun_MergesAndSwaps(t *testing.T) {
	e := setup(t, schema+homeChanges)
	var released bool

	out, err := e.runner(&released).Run(context.Background(), e.request(true, map[merge.Ref]merge.Choice{clash: merge.Home}))

	require.NoError(t, err)
	assert.True(t, out.Swapped)
	assert.True(t, released, "the app's pool is released before the swap")
	assert.Equal(t, []string{"home-edit", "home-clash", "three", "home-new", "ec2-new"}, e.names(t, e.home))
	assert.Equal(t, []string{"home-edit", "home-clash", "three", "home-new"}, e.names(t, e.home+"_pre_restore"),
		"the old home database is kept")
	assert.FileExists(t, filepath.Join(e.dir, "failback", "swapped-dump-1"))

	f, err := os.Open(out.PreRestore)
	require.NoError(t, err)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	body, err := io.ReadAll(gz)
	require.NoError(t, err)
	assert.Contains(t, string(body), "home-new", "the pre-restore dump holds what home had")

	for _, scratch := range []string{"_merge_base", "_merge_ec2"} {
		var n int
		require.NoError(t, e.admin.QueryRow(context.Background(),
			"SELECT count(*) FROM pg_database WHERE datname = $1", e.home+scratch).Scan(&n))
		assert.Zero(t, n, "scratch database %s is gone", scratch)
	}
}

func TestRun_AConflictStopsBeforeTheSwap(t *testing.T) {
	e := setup(t, schema+homeChanges)

	out, err := e.runner(nil).Run(context.Background(), e.request(true, nil))

	require.NoError(t, err)
	require.Len(t, out.Result.Conflicts, 1)
	assert.Equal(t, clash, out.Result.Conflicts[0].Ref)
	assert.False(t, out.Swapped)
	assert.Equal(t, []string{"home-edit", "home-clash", "three", "home-new"}, e.names(t, e.home), "home is untouched")

	again, err := e.runner(nil).Run(context.Background(), e.request(true, map[merge.Ref]merge.Choice{clash: merge.EC2}))

	require.NoError(t, err)
	assert.True(t, again.Swapped)
	assert.Equal(t, []string{"home-edit", "ec2-clash", "three", "home-new", "ec2-new"}, e.names(t, e.home))
}

func TestRun_DryRunDoesNotSwap(t *testing.T) {
	e := setup(t, schema+homeChanges)

	out, err := e.runner(nil).Run(context.Background(), e.request(false, map[merge.Ref]merge.Choice{clash: merge.EC2}))

	require.NoError(t, err)
	assert.False(t, out.Swapped)
	assert.Empty(t, out.Result.Conflicts)
	assert.Equal(t, []string{"home-edit", "home-clash", "three", "home-new"}, e.names(t, e.home))
}

func TestRun_AnEmptyHomeTakesEC2AsIs(t *testing.T) {
	e := setup(t, "")

	out, err := e.runner(nil).Run(context.Background(), e.request(true, nil))

	require.NoError(t, err)
	assert.True(t, out.Skipped)
	assert.True(t, out.Swapped)
	assert.Equal(t, []string{"one", "ec2-clash", "three", "ec2-new"}, e.names(t, e.home))
}

func TestRun_ATruncatedDumpChangesNothing(t *testing.T) {
	e := setup(t, schema+homeChanges)
	raw, err := os.ReadFile(e.ec2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(e.ec2, raw[:len(raw)/2], 0o600))

	_, err = e.runner(nil).Run(context.Background(), e.request(true, nil))

	assert.ErrorIs(t, err, restore.ErrBadDump)
	assert.Equal(t, []string{"home-edit", "home-clash", "three", "home-new"}, e.names(t, e.home))
	assert.NoDirExists(t, filepath.Join(e.dir, "failback"), "nothing was written")
}

func TestRun_ARepeatedRunIsAlreadyDone(t *testing.T) {
	e := setup(t, schema+homeChanges)
	decisions := map[merge.Ref]merge.Choice{clash: merge.EC2}
	_, err := e.runner(nil).Run(context.Background(), e.request(true, decisions))
	require.NoError(t, err)

	out, err := e.runner(nil).Run(context.Background(), e.request(true, decisions))

	require.NoError(t, err)
	assert.True(t, out.AlreadyDone)
	assert.Equal(t, []string{"home-edit", "ec2-clash", "three", "home-new", "ec2-new"}, e.names(t, e.home))
}
