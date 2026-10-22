package merge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

type Op string

const (
	Insert Op = "insert"
	Update Op = "update"
	Delete Op = "delete"
)

type Row struct {
	Key  string
	Data map[string]any
	sig  string
}

type Change struct {
	Table string
	Key   string
	Op    Op
	Old   *Row
	New   *Row
}

func readRows(ctx context.Context, q Querier, t Table) (map[string]Row, error) {
	rows, err := q.Query(ctx, "SELECT to_jsonb(t) FROM public."+pgx.Identifier{t.Name}.Sanitize()+" t")
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}
	defer rows.Close()

	out := map[string]Row{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("read %s: %w", t.Name, err)
		}
		row, err := parseRow(t, raw)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", t.Name, err)
		}
		out[row.Key] = row
	}
	return out, rows.Err()
}

func parseRow(t Table, raw []byte) (Row, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var data map[string]any
	if err := dec.Decode(&data); err != nil {
		return Row{}, err
	}

	pk := make([]any, len(t.PK))
	for i, col := range t.PK {
		pk[i] = data[col]
	}
	key, err := json.Marshal(pk)
	if err != nil {
		return Row{}, err
	}

	for _, col := range t.Ignore {
		delete(data, col)
	}
	sig, err := json.Marshal(data)
	if err != nil {
		return Row{}, err
	}
	return Row{Key: string(key), Data: data, sig: string(sig)}, nil
}

func same(a, b *Row) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.sig == b.sig
}

// changes lists what side did to the base, by row key.
func changes(table string, base, side map[string]Row) []Change {
	var out []Change
	for key, b := range base {
		s, ok := side[key]
		switch {
		case !ok:
			out = append(out, Change{Table: table, Key: key, Op: Delete, Old: ptr(b)})
		case s.sig != b.sig:
			out = append(out, Change{Table: table, Key: key, Op: Update, Old: ptr(b), New: ptr(s)})
		}
	}
	for key, s := range side {
		if _, ok := base[key]; !ok {
			out = append(out, Change{Table: table, Key: key, Op: Insert, New: ptr(s)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func ptr(r Row) *Row { return &r }
