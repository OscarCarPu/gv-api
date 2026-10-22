package merge_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"gv-api/internal/database"
	"gv-api/internal/merge"
	"gv-api/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const miniSchema = `
CREATE TABLE parents  (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE children (id SERIAL PRIMARY KEY, parent_id INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE, label TEXT NOT NULL);
CREATE TABLE links    (a INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE, b INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE, PRIMARY KEY (a, b));
CREATE TABLE wallets  (id SERIAL PRIMARY KEY, name TEXT NOT NULL, total NUMERIC NOT NULL DEFAULT 0);
CREATE TABLE entries  (id SERIAL PRIMARY KEY, wallet_id INT NOT NULL REFERENCES wallets(id), amount NUMERIC NOT NULL);
CREATE FUNCTION add_to_wallet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN UPDATE wallets SET total = total + NEW.amount WHERE id = NEW.wallet_id; RETURN NULL; END $$;
CREATE TRIGGER entries_total AFTER INSERT ON entries FOR EACH ROW EXECUTE FUNCTION add_to_wallet();

INSERT INTO parents (name) VALUES ('one'), ('two'), ('three');
INSERT INTO children (parent_id, label) VALUES (1, 'c1'), (2, 'c2');
INSERT INTO links VALUES (1, 2);
INSERT INTO wallets (name) VALUES ('bank');
INSERT INTO entries (wallet_id, amount) VALUES (1, 100);
`

const offsetSequences = `
SELECT setval(format('%I.%I', schemaname, sequencename)::regclass, coalesce(last_value, start_value) + 1000000)
FROM pg_sequences WHERE schemaname = 'public'`

var ignore = map[string][]string{"wallets": {"total"}}

func newDB(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	admin := testutil.NewPool(t)
	db := fmt.Sprintf("merge_%s_%d", name, time.Now().UnixNano())
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

func names(t *testing.T, pool *pgxpool.Pool, query string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query)
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

// scenario is three databases that started equal; ec2's sequences start a million higher.
func scenario(t *testing.T) (base, home, ec2 *pgxpool.Pool) {
	t.Helper()
	base, home, ec2 = newDB(t, "base"), newDB(t, "home"), newDB(t, "ec2")
	for _, db := range []*pgxpool.Pool{base, home, ec2} {
		exec(t, db, miniSchema)
	}
	exec(t, ec2, offsetSequences)

	exec(t, home, `
		INSERT INTO parents (name) VALUES ('home-new');
		INSERT INTO children (parent_id, label) VALUES (4, 'under-home-new');
		UPDATE parents SET name = 'home-edit' WHERE id = 1;
		UPDATE parents SET name = 'home-clash' WHERE id = 2;
		DELETE FROM children WHERE id = 1;
		INSERT INTO links VALUES (2, 3);
		INSERT INTO entries (wallet_id, amount) VALUES (1, 5);`)
	exec(t, ec2, `
		INSERT INTO parents (name) VALUES ('ec2-new');
		UPDATE parents SET name = 'ec2-clash' WHERE id = 2;
		INSERT INTO entries (wallet_id, amount) VALUES (1, 7);`)
	return base, home, ec2
}

func run(t *testing.T, base, home, ec2 *pgxpool.Pool, opt merge.Options) (*merge.Result, error) {
	t.Helper()
	opt.Ignore = ignore
	return merge.Merge(context.Background(), base, home, ec2, opt)
}

func TestMerge_AsksOnlyAboutTheClash(t *testing.T) {
	base, home, ec2 := scenario(t)

	res, err := run(t, base, home, ec2, merge.Options{Commit: true})

	require.NoError(t, err)
	require.Len(t, res.Conflicts, 1)
	c := res.Conflicts[0]
	assert.Equal(t, merge.Ref{Table: "parents", Key: "[2]"}, c.Ref)
	assert.Equal(t, "same row edited on both sides", c.Reason)
	assert.Equal(t, "two", c.Base["name"])
	assert.Equal(t, "home-clash", c.Home["name"])
	assert.Equal(t, "ec2-clash", c.EC2["name"])
	assert.False(t, res.Committed)
	assert.Equal(t, []string{"one", "ec2-clash", "three", "ec2-new"},
		names(t, ec2, "SELECT name FROM parents ORDER BY id"), "nothing was written")
}

func TestMerge_AppliesEverythingElseOnceTheClashIsAnswered(t *testing.T) {
	for _, tt := range []struct {
		choice merge.Choice
		clash  string
	}{{merge.EC2, "ec2-clash"}, {merge.Home, "home-clash"}} {
		t.Run(string(tt.choice), func(t *testing.T) {
			base, home, ec2 := scenario(t)

			res, err := run(t, base, home, ec2, merge.Options{
				Commit:    true,
				Decisions: map[merge.Ref]merge.Choice{{Table: "parents", Key: "[2]"}: tt.choice},
			})

			require.NoError(t, err)
			assert.Empty(t, res.Conflicts)
			assert.True(t, res.Committed)
			assert.Equal(t, []string{"home-edit", tt.clash, "three", "home-new", "ec2-new"},
				names(t, ec2, "SELECT name FROM parents ORDER BY id"))
			assert.Equal(t, []string{"c2:2", "under-home-new:4"},
				names(t, ec2, "SELECT label || ':' || parent_id FROM children ORDER BY id"))
			assert.Equal(t, []string{"1-2", "2-3"}, names(t, ec2, "SELECT a || '-' || b FROM links ORDER BY a"))
			assert.Equal(t, []string{"112"}, names(t, ec2, "SELECT total::int::text FROM wallets"),
				"the trigger adds home's entry to ec2's total")
		})
	}
}

func TestMerge_DryRunWritesNothing(t *testing.T) {
	base, home, ec2 := scenario(t)

	res, err := run(t, base, home, ec2, merge.Options{
		Decisions: map[merge.Ref]merge.Choice{{Table: "parents", Key: "[2]"}: merge.EC2},
	})

	require.NoError(t, err)
	assert.False(t, res.Committed)
	assert.NotEmpty(t, res.Entries)
	assert.Equal(t, []string{"one", "ec2-clash", "three", "ec2-new"},
		names(t, ec2, "SELECT name FROM parents ORDER BY id"))
}

func TestMerge_RejectsDifferentSchemas(t *testing.T) {
	base, home, ec2 := scenario(t)
	exec(t, home, "ALTER TABLE parents ADD COLUMN extra TEXT")

	_, err := run(t, base, home, ec2, merge.Options{Commit: true})

	assert.ErrorIs(t, err, merge.ErrSchema)
}

func TestMerge_StopsWhenHomeLostMostOfTheBase(t *testing.T) {
	base, home, ec2 := scenario(t)
	exec(t, home, "DELETE FROM children; DELETE FROM links; DELETE FROM entries; DELETE FROM wallets; DELETE FROM parents")

	_, err := run(t, base, home, ec2, merge.Options{Commit: true})

	assert.ErrorIs(t, err, merge.ErrMassDelete)
}

func TestMerge_StopsWhenADeleteWouldCascadeOverEC2Rows(t *testing.T) {
	base, home, ec2 := scenario(t)
	exec(t, home, "DELETE FROM parents WHERE id = 3")
	exec(t, ec2, "INSERT INTO children (parent_id, label) VALUES (3, 'ec2-child')")

	_, err := run(t, base, home, ec2, merge.Options{Commit: true})

	assert.ErrorIs(t, err, merge.ErrLost)
	assert.Contains(t, names(t, ec2, "SELECT label FROM children"), "ec2-child")
}

func TestMerge_RejectsAnIgnoredColumnThatDoesNotExist(t *testing.T) {
	base, home, ec2 := scenario(t)

	_, err := merge.Merge(context.Background(), base, home, ec2, merge.Options{Ignore: map[string][]string{"wallets": {"nope"}}})

	assert.ErrorContains(t, err, "wallets.nope")
}

func migrated(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	pool := newDB(t, name)
	u := pool.Config().ConnString()
	require.NoError(t, database.Migrate(u, "../../db/migrations"))
	return pool
}

func TestMerge_RealSchema(t *testing.T) {
	base, home, ec2 := migrated(t, "rbase"), migrated(t, "rhome"), migrated(t, "rec2")
	exec(t, ec2, offsetSequences)

	tables, err := merge.Introspect(context.Background(), ec2)
	require.NoError(t, err)
	pos := map[string]int{}
	for i, tb := range tables {
		pos[tb.Name] = i
	}
	assert.Less(t, pos["habits"], pos["habit_logs"])
	assert.Less(t, pos["projects"], pos["tasks"])
	assert.Less(t, pos["tasks"], pos["todos"])
	assert.Less(t, pos["tasks"], pos["recurring_commitments"])
	assert.Less(t, pos["recurring_commitments"], pos["recurring_commitment_skips"])

	exec(t, home, `
		INSERT INTO habits (name) VALUES ('Yoga');
		INSERT INTO tasks (name) VALUES ('Sand walls');
		INSERT INTO recurring_commitments (task_id, label, days_of_week, start_time, end_time)
			VALUES (1, 'Gym', '{1,3}', '09:00', '10:00');
		INSERT INTO calendar_events (name, start_time, end_time) VALUES ('Dentist', now(), now() + interval '1 hour');`)
	exec(t, ec2, "INSERT INTO habits (name) VALUES ('Swim')")

	res, err := merge.Merge(context.Background(), base, home, ec2, merge.Options{
		Commit: true, Ignore: map[string][]string{"accounts": {"total"}},
	})

	require.NoError(t, err)
	assert.Empty(t, res.Conflicts)
	assert.True(t, res.Committed)
	assert.Equal(t, []string{"Yoga", "Swim"}, names(t, ec2, "SELECT name FROM habits ORDER BY id"))
	assert.Equal(t, []string{"Gym:{1,3}"}, names(t, ec2, "SELECT label || ':' || days_of_week::text FROM recurring_commitments"))
	assert.Equal(t, []string{"Dentist"}, names(t, ec2, "SELECT name FROM calendar_events"))
}
