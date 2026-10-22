package merge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Table struct {
	Name    string
	PK      []string
	Cols    []string
	Parents []string
	Ignore  []string
}

const (
	columnsQuery = `
SELECT c.relname, a.attname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid
WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'schema_migrations'
  AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''
ORDER BY c.relname, a.attnum`

	// A table's key is its primary key, or else its first unique constraint with no nullable column.
	primaryKeysQuery = `
WITH keys AS (
  SELECT k.conrelid, k.conkey,
         row_number() OVER (PARTITION BY k.conrelid ORDER BY (k.contype = 'p') DESC, k.conname) AS rank
  FROM pg_constraint k
  WHERE k.contype IN ('p', 'u')
    AND NOT EXISTS (
      SELECT 1 FROM unnest(k.conkey) x(attnum)
      JOIN pg_attribute na ON na.attrelid = k.conrelid AND na.attnum = x.attnum
      WHERE NOT na.attnotnull)
)
SELECT c.relname, a.attname
FROM keys
JOIN pg_class c ON c.oid = keys.conrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
CROSS JOIN LATERAL unnest(keys.conkey) WITH ORDINALITY AS u(attnum, ord)
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = u.attnum
WHERE keys.rank = 1 AND n.nspname = 'public'
ORDER BY c.relname, u.ord`

	foreignKeysQuery = `
SELECT c.relname, p.relname
FROM pg_constraint k
JOIN pg_class c ON c.oid = k.conrelid
JOIN pg_class p ON p.oid = k.confrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE k.contype = 'f' AND n.nspname = 'public'`
)

// Introspect returns the public tables, parents before children.
func Introspect(ctx context.Context, q Querier) ([]Table, error) {
	byName := map[string]*Table{}
	var names []string

	err := scanPairs(ctx, q, columnsQuery, func(table, col string) {
		t, ok := byName[table]
		if !ok {
			t = &Table{Name: table}
			byName[table] = t
			names = append(names, table)
		}
		t.Cols = append(t.Cols, col)
	})
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}

	err = scanPairs(ctx, q, primaryKeysQuery, func(table, col string) {
		if t, ok := byName[table]; ok {
			t.PK = append(t.PK, col)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("read primary keys: %w", err)
	}

	err = scanPairs(ctx, q, foreignKeysQuery, func(child, parent string) {
		if t, ok := byName[child]; ok {
			t.Parents = append(t.Parents, parent)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("read foreign keys: %w", err)
	}

	for _, name := range names {
		if len(byName[name].PK) == 0 {
			return nil, fmt.Errorf("table %s has no primary key or unique constraint on non-null columns", name)
		}
	}
	return order(byName, names)
}

func scanPairs(ctx context.Context, q Querier, sql string, each func(a, b string)) error {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return err
		}
		each(a, b)
	}
	return rows.Err()
}

func order(byName map[string]*Table, names []string) ([]Table, error) {
	sort.Strings(names)
	done := map[string]bool{}
	var out []Table
	for len(out) < len(names) {
		before := len(out)
		for _, name := range names {
			t := byName[name]
			if done[name] || !ready(t, done) {
				continue
			}
			done[name] = true
			out = append(out, *t)
		}
		if len(out) == before {
			return nil, fmt.Errorf("foreign keys form a cycle among %s", strings.Join(pending(names, done), ", "))
		}
	}
	return out, nil
}

func ready(t *Table, done map[string]bool) bool {
	for _, p := range t.Parents {
		if p != t.Name && !done[p] {
			return false
		}
	}
	return true
}

func pending(names []string, done map[string]bool) []string {
	var out []string
	for _, n := range names {
		if !done[n] {
			out = append(out, n)
		}
	}
	return out
}

// Signature changes whenever a table or column is added, dropped or reordered.
func Signature(tables []Table) string {
	var b strings.Builder
	for _, t := range tables {
		fmt.Fprintf(&b, "%s(%s|%s);", t.Name, strings.Join(t.Cols, ","), strings.Join(t.PK, ","))
	}
	return b.String()
}
