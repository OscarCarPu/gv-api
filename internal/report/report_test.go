package report_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"gv-api/internal/report"
	"gv-api/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const miniSchema = `
CREATE TABLE parents  (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE children (id SERIAL PRIMARY KEY, parent_id INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE, label TEXT NOT NULL);
CREATE TABLE wallets  (id SERIAL PRIMARY KEY, name TEXT NOT NULL, total NUMERIC NOT NULL DEFAULT 0);
CREATE TABLE entries  (id SERIAL PRIMARY KEY, wallet_id INT NOT NULL REFERENCES wallets(id), amount NUMERIC NOT NULL);
CREATE TABLE notes    (text TEXT);
CREATE FUNCTION add_to_wallet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN UPDATE wallets SET total = total + NEW.amount WHERE id = NEW.wallet_id; RETURN NULL; END $$;
CREATE TRIGGER entries_total AFTER INSERT ON entries FOR EACH ROW EXECUTE FUNCTION add_to_wallet();

INSERT INTO parents (name) VALUES ('one'), ('two'), ('three'), ('four');
INSERT INTO children (parent_id, label) VALUES (1, 'c1');
INSERT INTO wallets (name) VALUES ('bank');
`

var ignore = map[string][]string{"wallets": {"total"}}

func newDB(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	admin := testutil.NewPool(t)
	db := fmt.Sprintf("report_%s_%d", name, time.Now().UnixNano())
	ctx := context.Background()
	_, err := admin.Exec(ctx, "CREATE DATABASE "+db)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)") })

	u, err := url.Parse(testutil.DSN(t))
	require.NoError(t, err)
	u.Path = "/" + db
	pool, err := pgxpool.New(ctx, u.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), sql)
	require.NoError(t, err, sql)
}

func build(t *testing.T, homeSQL, ec2SQL string) *report.Report {
	t.Helper()
	base, home, ec2 := newDB(t, "base"), newDB(t, "home"), newDB(t, "ec2")
	for _, db := range []*pgxpool.Pool{base, home, ec2} {
		exec(t, db, miniSchema)
	}
	exec(t, home, homeSQL)
	exec(t, ec2, ec2SQL)

	rep, err := report.Build(context.Background(), base, home, ec2, report.Options{Ignore: ignore})
	require.NoError(t, err)
	return rep
}

func TestBuild_SkipsTheTablesItIsTold(t *testing.T) {
	base, home, ec2 := newDB(t, "base"), newDB(t, "home"), newDB(t, "ec2")
	for _, db := range []*pgxpool.Pool{base, home, ec2} {
		exec(t, db, miniSchema)
	}
	exec(t, home, `INSERT INTO parents (name) VALUES ('home-new')`)

	rep, err := report.Build(context.Background(), base, home, ec2, report.Options{Skip: []string{"parents"}})

	require.NoError(t, err)
	assert.Empty(t, find(rep, "parents", "insert"))
}

func find(rep *report.Report, table, op string) []report.Item {
	var out []report.Item
	for _, it := range rep.Items {
		if it.Table == table && string(it.Op) == op {
			out = append(out, it)
		}
	}
	return out
}

func TestBuild_NothingChangedOnHomeIsAnEmptyReport(t *testing.T) {
	rep := build(t, `SELECT 1`, `INSERT INTO parents (name) VALUES ('only-ec2'); UPDATE parents SET name = 'ec2' WHERE id = 1`)

	assert.Empty(t, rep.Items)
}

func TestBuild_ListsEveryKindOfChangeHomeMade(t *testing.T) {
	rep := build(t, `
		INSERT INTO parents (name) VALUES ('home-new');
		UPDATE parents SET name = 'home-two' WHERE id = 2;
		DELETE FROM parents WHERE id = 3`, `SELECT 1`)

	require.Len(t, rep.Items, 3)
	inserted := find(rep, "parents", "insert")
	require.Len(t, inserted, 1)
	assert.Equal(t, "home-new", inserted[0].Home["name"])
	assert.Nil(t, inserted[0].Base)
	assert.Nil(t, inserted[0].EC2)

	updated := find(rep, "parents", "update")
	require.Len(t, updated, 1)
	assert.Equal(t, "two", updated[0].Base["name"])
	assert.Equal(t, "home-two", updated[0].Home["name"])
	assert.Equal(t, "two", updated[0].EC2["name"])

	deleted := find(rep, "parents", "delete")
	require.Len(t, deleted, 1)
	assert.Nil(t, deleted[0].Home)
	assert.Equal(t, "three", deleted[0].EC2["name"])
}

func TestBuild_SkipsWhatEC2AlreadyHolds(t *testing.T) {
	rep := build(t,
		`UPDATE parents SET name = 'same' WHERE id = 2; DELETE FROM parents WHERE id = 3`,
		`UPDATE parents SET name = 'same' WHERE id = 2; DELETE FROM parents WHERE id = 3`)

	assert.Empty(t, rep.Items)
}

func TestBuild_ShowsTheEC2VersionOfARowBothSidesEdited(t *testing.T) {
	rep := build(t, `UPDATE parents SET name = 'home' WHERE id = 2`, `UPDATE parents SET name = 'ec2' WHERE id = 2`)

	require.Len(t, rep.Items, 1)
	assert.Equal(t, "home", rep.Items[0].Home["name"])
	assert.Equal(t, "ec2", rep.Items[0].EC2["name"])
}

func TestBuild_NamesTheRowAChangePointsAt(t *testing.T) {
	rep := build(t, `INSERT INTO children (parent_id, label) VALUES (2, 'c2')`, `SELECT 1`)

	items := find(rep, "children", "insert")
	require.Len(t, items, 1)
	assert.Equal(t, "parents", items[0].Related["parent_id"].Table)
	assert.Equal(t, "two", items[0].Related["parent_id"].Row["name"])
}

func TestBuild_DoesNotCompareColumnsATriggerDerives(t *testing.T) {
	rep := build(t, `INSERT INTO entries (wallet_id, amount) VALUES (1, 50)`, `SELECT 1`)

	assert.Len(t, find(rep, "entries", "insert"), 1)
	assert.Empty(t, find(rep, "wallets", "update"))
}

func TestBuild_SaysWhichTablesHaveNoKey(t *testing.T) {
	rep := build(t, `INSERT INTO notes VALUES ('lost')`, `SELECT 1`)

	assert.Equal(t, []string{"notes"}, rep.Skipped)
	assert.Empty(t, rep.Items)
}
