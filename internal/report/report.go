// Package report lists what home changed since the dump both sides started from and that the data
// restored from EC2 does not hold. It only reads: the restore keeps EC2's data as it is.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type Op string

const (
	Insert Op = "insert"
	Update Op = "update"
	Delete Op = "delete"
)

type Related struct {
	Table string         `json:"table"`
	Row   map[string]any `json:"row"`
}

type Item struct {
	Table string `json:"table"`
	Key   string `json:"key"`
	Op    Op     `json:"op"`
	// Base is the row in the common dump, Home the row at home now, EC2 the row that was restored.
	Base map[string]any `json:"base"`
	Home map[string]any `json:"home"`
	EC2  map[string]any `json:"ec2"`
	// Related holds, per foreign-key column, the row it points at.
	Related map[string]Related `json:"related,omitempty"`
}

type Report struct {
	At      time.Time `json:"at"`
	Items   []Item    `json:"items"`
	Skipped []string  `json:"skipped,omitempty"`
	Error   string    `json:"error,omitempty"`
}

type row struct {
	data map[string]any
	sig  string
}

type snapshot map[string]map[string]row

// Build compares home with base, and keeps what differs from the row ec2 holds. Columns in ignore,
// per table, are derived by a trigger and are not compared.
func Build(ctx context.Context, base, home, ec2 Querier, ignore map[string][]string) (*Report, error) {
	tables, keyless, err := Introspect(ctx, home)
	if err != nil {
		return nil, err
	}

	var snaps [3]snapshot
	for i, q := range []Querier{base, home, ec2} {
		snaps[i] = snapshot{}
		for _, t := range tables {
			if snaps[i][t.Name], err = read(ctx, q, t, ignore[t.Name]); err != nil {
				return nil, err
			}
		}
	}
	baseRows, homeRows, ec2Rows := snaps[0], snaps[1], snaps[2]

	rep := &Report{At: time.Now().UTC(), Items: []Item{}, Skipped: keyless}
	for _, t := range tables {
		keys := map[string]bool{}
		for k := range baseRows[t.Name] {
			keys[k] = true
		}
		for k := range homeRows[t.Name] {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		slices.Sort(sorted)

		for _, k := range sorted {
			b, h, e := lookup(baseRows[t.Name], k), lookup(homeRows[t.Name], k), lookup(ec2Rows[t.Name], k)
			if same(b, h) || same(h, e) {
				continue
			}
			item := Item{Table: t.Name, Key: k, Op: op(b, h), Base: data(b), Home: data(h), EC2: data(e)}
			item.Related = related(t, item, snaps)
			rep.Items = append(rep.Items, item)
		}
	}
	return rep, nil
}

func op(base, home *row) Op {
	switch {
	case base == nil:
		return Insert
	case home == nil:
		return Delete
	}
	return Update
}

func lookup(rows map[string]row, key string) *row {
	if r, ok := rows[key]; ok {
		return &r
	}
	return nil
}

func same(a, b *row) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.sig == b.sig
}

func data(r *row) map[string]any {
	if r == nil {
		return nil
	}
	return r.data
}

func read(ctx context.Context, q Querier, t Table, ignore []string) (map[string]row, error) {
	rows, err := q.Query(ctx, "SELECT to_jsonb(t) FROM public."+pgx.Identifier{t.Name}.Sanitize()+" t")
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", t.Name, err)
	}
	defer rows.Close()

	out := map[string]row{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("read %s: %w", t.Name, err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var d map[string]any
		if err := dec.Decode(&d); err != nil {
			return nil, fmt.Errorf("read %s: %w", t.Name, err)
		}

		key := make([]any, len(t.Key))
		for i, col := range t.Key {
			key[i] = d[col]
		}
		for _, col := range ignore {
			delete(d, col)
		}
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		sig, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		out[string(k)] = row{data: d, sig: string(sig)}
	}
	return out, rows.Err()
}

// related finds the row each foreign-key column of the item points at, in the first side that has it.
func related(t Table, item Item, snaps [3]snapshot) map[string]Related {
	var out map[string]Related
	for col, parent := range t.Refs {
		for _, d := range []map[string]any{item.Home, item.EC2, item.Base} {
			if d == nil || d[col] == nil {
				continue
			}
			key, err := json.Marshal([]any{d[col]})
			if err != nil {
				continue
			}
			for _, snap := range []snapshot{snaps[2], snaps[1], snaps[0]} {
				if r, ok := snap[parent][string(key)]; ok {
					if out == nil {
						out = map[string]Related{}
					}
					out[col] = Related{Table: parent, Row: r.data}
					break
				}
			}
			if _, ok := out[col]; ok {
				break
			}
		}
	}
	return out
}
