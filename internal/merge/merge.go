package merge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSchema     = errors.New("base, home and ec2 do not share the same schema")
	ErrMassDelete = errors.New("home is missing more than half of the base rows")
	ErrLost       = errors.New("the merge would drop rows created on ec2")
)

type Ref struct {
	Table string
	Key   string
}

type Options struct {
	// Ignore lists, per table, the columns a trigger derives. They are neither compared nor copied.
	Ignore    map[string][]string
	Decisions map[Ref]Choice
	Commit    bool
	// Force allows a merge that deletes more than half of the base rows.
	Force bool
}

type Entry struct {
	Ref
	Op     Op
	Do     Do
	Reason string
}

type Conflict struct {
	Ref
	Op     Op
	Reason string
	Base   map[string]any
	Home   map[string]any
	EC2    map[string]any
}

type Result struct {
	Entries   []Entry
	Conflicts []Conflict
	Committed bool
}

// Merge applies to ec2 what home changed since base. Nothing is written when a conflict is left
// unanswered or Commit is false; the result still says what would happen.
func Merge(ctx context.Context, base, home Querier, ec2 *pgxpool.Pool, opt Options) (*Result, error) {
	tx, err := ec2.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tables, err := Introspect(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := sameSchema(ctx, tables, base, home); err != nil {
		return nil, err
	}
	if err := setIgnore(tables, opt.Ignore); err != nil {
		return nil, err
	}

	m := &merger{tx: tx, tables: tables, opt: opt, res: &Result{}}
	if err := m.run(ctx, base, home); err != nil {
		return nil, err
	}
	if len(m.res.Conflicts) > 0 || !opt.Commit {
		return m.res, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	m.res.Committed = true
	return m.res, nil
}

func sameSchema(ctx context.Context, tables []Table, others ...Querier) error {
	want := Signature(tables)
	for _, q := range others {
		got, err := Introspect(ctx, q)
		if err != nil {
			return err
		}
		if Signature(got) != want {
			return ErrSchema
		}
	}
	return nil
}

func setIgnore(tables []Table, ignore map[string][]string) error {
	for name, cols := range ignore {
		i := slices.IndexFunc(tables, func(t Table) bool { return t.Name == name })
		if i < 0 {
			return fmt.Errorf("ignore: no table %s", name)
		}
		for _, col := range cols {
			if !slices.Contains(tables[i].Cols, col) || slices.Contains(tables[i].PK, col) {
				return fmt.Errorf("ignore: %s.%s is not a non-key column", name, col)
			}
		}
		tables[i].Ignore = cols
	}
	return nil
}

type merger struct {
	tx     pgx.Tx
	tables []Table
	opt    Options
	res    *Result
}

type snapshot map[string]map[string]Row

func (m *merger) read(ctx context.Context, q Querier) (snapshot, error) {
	out := snapshot{}
	for _, t := range m.tables {
		rows, err := readRows(ctx, q, t)
		if err != nil {
			return nil, err
		}
		out[t.Name] = rows
	}
	return out, nil
}

func (m *merger) run(ctx context.Context, baseDB, homeDB Querier) error {
	base, err := m.read(ctx, baseDB)
	if err != nil {
		return err
	}
	home, err := m.read(ctx, homeDB)
	if err != nil {
		return err
	}
	ec2, err := m.read(ctx, m.tx)
	if err != nil {
		return err
	}

	var fromHome, fromEC2 [][]Change
	deletes, baseRows := 0, 0
	for _, t := range m.tables {
		a := changes(t.Name, base[t.Name], home[t.Name])
		fromHome = append(fromHome, a)
		fromEC2 = append(fromEC2, changes(t.Name, base[t.Name], ec2[t.Name]))
		baseRows += len(base[t.Name])
		for _, c := range a {
			if c.Op == Delete {
				deletes++
			}
		}
	}
	if deletes*2 > baseRows && !m.opt.Force {
		return ErrMassDelete
	}

	// parents first for inserts and updates, children first for deletes
	for i, t := range m.tables {
		if err := m.each(ctx, t, ec2[t.Name], fromHome[i], func(c Change) bool { return c.Op != Delete }); err != nil {
			return err
		}
	}
	for i := len(m.tables) - 1; i >= 0; i-- {
		t := m.tables[i]
		if err := m.each(ctx, t, ec2[t.Name], fromHome[i], func(c Change) bool { return c.Op == Delete }); err != nil {
			return err
		}
	}
	return m.checkNothingLost(ctx, fromEC2)
}

func (m *merger) each(ctx context.Context, t Table, ec2 map[string]Row, list []Change, want func(Change) bool) error {
	for _, c := range list {
		if !want(c) {
			continue
		}
		var cur *Row
		if r, ok := ec2[c.Key]; ok {
			cur = &r
		}
		ref := Ref{Table: t.Name, Key: c.Key}
		do, reason := decide(c, cur, m.opt.Decisions[ref])

		if do != Ask && do != Nothing {
			if err := m.write(ctx, t, c, do); err != nil {
				var pg *pgconn.PgError
				if !errors.As(err, &pg) {
					return err
				}
				do, reason = Ask, "db refused it: "+pg.Message
			}
		}
		m.res.Entries = append(m.res.Entries, Entry{Ref: ref, Op: c.Op, Do: do, Reason: reason})
		if do == Ask {
			m.res.Conflicts = append(m.res.Conflicts, conflict(ref, c, cur, reason))
		}
	}
	return nil
}

func conflict(ref Ref, c Change, ec2 *Row, reason string) Conflict {
	out := Conflict{Ref: ref, Op: c.Op, Reason: reason}
	if c.Old != nil {
		out.Base = c.Old.Data
	}
	if c.New != nil {
		out.Home = c.New.Data
	}
	if ec2 != nil {
		out.EC2 = ec2.Data
	}
	return out
}

func (m *merger) write(ctx context.Context, t Table, c Change, do Do) error {
	sp, err := m.tx.Begin(ctx)
	if err != nil {
		return err
	}
	row := c.New
	if do == DoDelete {
		row = c.Old
	}
	payload, err := json.Marshal(row.Data)
	if err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	if _, err := sp.Exec(ctx, statement(t, do), string(payload)); err != nil {
		_ = sp.Rollback(ctx)
		return err
	}
	return sp.Commit(ctx)
}

func (m *merger) checkNothingLost(ctx context.Context, fromEC2 [][]Change) error {
	after, err := m.read(ctx, m.tx)
	if err != nil {
		return err
	}
	for i, t := range m.tables {
		for _, c := range fromEC2[i] {
			if _, ok := after[t.Name][c.Key]; c.Op != Delete && !ok {
				return fmt.Errorf("%w: %s %s", ErrLost, t.Name, c.Key)
			}
		}
	}
	return nil
}

// statement writes one row from a jsonb document; $1 holds the document.
func statement(t Table, do Do) string {
	table := pgx.Identifier{t.Name}.Sanitize()
	from := "jsonb_populate_record(NULL::" + table + ", $1::jsonb)"
	var keys []string
	for _, col := range t.PK {
		id := pgx.Identifier{col}.Sanitize()
		keys = append(keys, "t."+id+" = v."+id)
	}
	where := strings.Join(keys, " AND ")

	switch do {
	case DoInsert:
		cols := quoted(t.Cols, t.Ignore)
		return "INSERT INTO " + table + " (" + cols + ") OVERRIDING SYSTEM VALUE SELECT " + cols + " FROM " + from
	case DoUpdate:
		var sets []string
		for _, col := range t.Cols {
			if slices.Contains(t.PK, col) || slices.Contains(t.Ignore, col) {
				continue
			}
			id := pgx.Identifier{col}.Sanitize()
			sets = append(sets, id+" = v."+id)
		}
		return "UPDATE " + table + " t SET " + strings.Join(sets, ", ") + " FROM " + from + " v WHERE " + where
	}
	return "DELETE FROM " + table + " t USING " + from + " v WHERE " + where
}

func quoted(cols, skip []string) string {
	var out []string
	for _, col := range cols {
		if !slices.Contains(skip, col) {
			out = append(out, pgx.Identifier{col}.Sanitize())
		}
	}
	return strings.Join(out, ", ")
}
