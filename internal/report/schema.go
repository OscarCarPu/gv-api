package report

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
	Name string
	Key  []string
	// Refs maps a column to the table its single-column foreign key points at.
	Refs map[string]string
}

const (
	tablesQuery = `
SELECT c.relname
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'schema_migrations'`

	// A table's key is its primary key, or else its first unique constraint with no nullable column.
	keysQuery = `
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
CROSS JOIN LATERAL unnest(keys.conkey) WITH ORDINALITY AS u(attnum, ord)
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = u.attnum
WHERE keys.rank = 1
ORDER BY c.relname, u.ord`

	refsQuery = `
SELECT c.relname || '.' || a.attname, p.relname
FROM pg_constraint k
JOIN pg_class c ON c.oid = k.conrelid
JOIN pg_class p ON p.oid = k.confrelid
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.conkey[1]
WHERE k.contype = 'f' AND cardinality(k.conkey) = 1`
)

// Introspect returns the public tables that have a key, by name, and the names of those that do not.
func Introspect(ctx context.Context, q Querier) ([]Table, []string, error) {
	byName := map[string]*Table{}
	var names []string

	scan := func(sql string, each func(a, b string)) error {
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

	rows, err := q.Query(ctx, tablesQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("read tables: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, nil, err
		}
		byName[name] = &Table{Name: name}
		names = append(names, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	err = scan(keysQuery, func(table, col string) {
		if t, ok := byName[table]; ok {
			t.Key = append(t.Key, col)
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read keys: %w", err)
	}
	err = scan(refsQuery, func(column, parent string) {
		table, col, _ := strings.Cut(column, ".")
		if t, ok := byName[table]; ok {
			if t.Refs == nil {
				t.Refs = map[string]string{}
			}
			t.Refs[col] = parent
		}
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read references: %w", err)
	}

	sort.Strings(names)
	var tables []Table
	var keyless []string
	for _, name := range names {
		if len(byName[name].Key) == 0 {
			keyless = append(keyless, name)
			continue
		}
		tables = append(tables, *byName[name])
	}
	return tables, keyless, nil
}
