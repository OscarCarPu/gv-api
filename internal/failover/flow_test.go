package failover_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gv-api/internal/failover"
	"gv-api/internal/restore"
	"gv-api/internal/testutil"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const flowSchema = `
CREATE TABLE parents (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO parents (name) VALUES ('one'), ('two'), ('three');
`

// copyStore serves the "S3" objects from a local folder.
type copyStore struct{ dir string }

func (s copyStore) Download(_ context.Context, key, dest string) error {
	raw, err := os.ReadFile(filepath.Join(s.dir, filepath.Base(key)))
	if err != nil {
		return err
	}
	return os.WriteFile(dest, raw, 0o600)
}

func gz(t *testing.T, path, sql string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	w := gzip.NewWriter(f)
	_, err = w.Write([]byte(sql))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
}

func TestFlow_PushToSwapThroughTheRealRunner(t *testing.T) {
	admin := testutil.NewPool(t)
	ctx := context.Background()
	home := fmt.Sprintf("flow_%d", time.Now().UnixNano())
	urlFor := func(db string) string {
		u, err := url.Parse(testutil.DSN(t))
		require.NoError(t, err)
		u.Path = "/" + db
		return u.String()
	}
	_, err := admin.Exec(ctx, "CREATE DATABASE "+home)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, db := range []string{home, home + "_restore_base", home + "_restore_ec2", home + "_pre_restore"} {
			_, _ = admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+db+`" WITH (FORCE)`)
		}
	})
	homePool, err := pgxpool.New(ctx, urlFor(home))
	require.NoError(t, err)
	t.Cleanup(homePool.Close)
	_, err = homePool.Exec(ctx, flowSchema+`
		UPDATE parents SET name = 'home-clash' WHERE id = 2;
		INSERT INTO parents (name) VALUES ('home-new');`)
	require.NoError(t, err)

	objects := t.TempDir()
	gz(t, filepath.Join(objects, "base.sql.gz"), flowSchema)
	gz(t, filepath.Join(objects, "dump.sql.gz"), flowSchema+`
		SELECT setval('parents_id_seq', 1000000);
		UPDATE parents SET name = 'ec2-clash' WHERE id = 2;
		INSERT INTO parents (name) VALUES ('ec2-new');`)

	var released, after int
	dir := t.TempDir()
	ctl := failover.New(failover.Options{
		Token: token,
		Params: &params{vals: map[string]string{
			"/home-lab-failover/state": state("FAILING_BACK"),
			"/home-lab-failover/base":  "gv-db/hourly/base.sql.gz",
			"/home-lab-failover/dump":  "gv-db/hourly/dump.sql.gz",
		}},
		Store: copyStore{dir: objects},
		Dir:   dir,
		Restorer: restore.New(restore.Config{
			Admin: admin, URL: urlFor, Home: home, Dir: dir,
			Release: func() { released++; homePool.Reset() },
		}),
		After: func() { after++ },
	})
	r := chi.NewRouter()
	r.Use(ctl.Lock)
	ctl.Routes(r)
	ctl.ReportRoutes(r)
	call := func(method, path, tok, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Failover-Token", tok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	status := func() string {
		body := call("GET", "/failover/status", token, "").Body.String()
		for _, s := range []string{"running", "done", "failed"} {
			if strings.Contains(body, `"merge":"`+s+`"`) {
				return s
			}
		}
		return ""
	}

	require.Equal(t, http.StatusOK, call("POST", "/failover", token, `{"mode":"merge"}`).Code)
	require.Eventually(t, func() bool { return status() == "done" }, 10*time.Second, 20*time.Millisecond)

	assert.Equal(t, failover.ModeNormal, ctl.Mode())
	assert.Equal(t, 1, released)
	assert.Equal(t, 1, after)
	rows, err := homePool.Query(ctx, "SELECT name FROM parents ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	assert.Equal(t, []string{"one", "ec2-clash", "three", "ec2-new"}, names, "the EC2 data is restored as it is")

	body := call("GET", "/report", "", "").Body.String()
	assert.Contains(t, body, "home-clash", "the report holds what home had edited")
	assert.Contains(t, body, "home-new", "the report holds what home had created")
}
