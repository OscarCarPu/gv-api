package assistant

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReadResult holds the outcome of a read query, ready to feed the summarizer.
type ReadResult struct {
	Columns   []string
	Rows      [][]any
	Truncated bool
}

// ReadExecutor runs assistant-proposed SELECT statements safely. The real
// guarantee is a read-only, always-rolled-back transaction (Postgres rejects
// any write, side-effecting function, or sequence advance at the engine level);
// the static checks only stop obvious non-queries and multi-statement input.
type ReadExecutor struct {
	pool      *pgxpool.Pool
	timeoutMS int
	maxRows   int
	maxCols   int
}

// NewReadExecutor builds a ReadExecutor. Non-positive limits fall back to
// sensible defaults.
func NewReadExecutor(pool *pgxpool.Pool, timeoutMS, maxRows, maxCols int) *ReadExecutor {
	if timeoutMS <= 0 {
		timeoutMS = 3000
	}
	if maxRows <= 0 {
		maxRows = 200
	}
	if maxCols <= 0 {
		maxCols = 40
	}
	return &ReadExecutor{pool: pool, timeoutMS: timeoutMS, maxRows: maxRows, maxCols: maxCols}
}

// validateReadSQL enforces the two cheap static rules: a single statement whose
// first keyword is SELECT or WITH. Everything else is left to the read-only tx.
func validateReadSQL(sql string) (string, error) {
	s := strings.TrimSpace(sql)
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrUnsafeSQL)
	}
	// Allow exactly one optional trailing semicolon; reject any other.
	s = strings.TrimRight(s, " \t\r\n")
	s = strings.TrimSuffix(s, ";")
	if strings.Contains(s, ";") {
		return "", fmt.Errorf("%w: multiple statements are not allowed", ErrUnsafeSQL)
	}
	first := strings.ToUpper(strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '('
	})[0])
	if first != "SELECT" && first != "WITH" {
		return "", fmt.Errorf("%w: only SELECT/WITH queries are allowed", ErrUnsafeSQL)
	}
	return s, nil
}

// Run validates and executes a read query inside a read-only transaction that is
// always rolled back, applying a statement timeout and row/column caps.
func (e *ReadExecutor) Run(ctx context.Context, sql string) (ReadResult, error) {
	clean, err := validateReadSQL(sql)
	if err != nil {
		return ReadResult{}, err
	}

	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return ReadResult{}, err
	}
	// Always roll back: nothing a read query does should ever be committed.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", e.timeoutMS)); err != nil {
		return ReadResult{}, err
	}

	rows, err := tx.Query(ctx, clean)
	if err != nil {
		return ReadResult{}, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	if len(fields) > e.maxCols {
		return ReadResult{}, fmt.Errorf("%w: too many columns (%d)", ErrReadTooLarge, len(fields))
	}
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = string(f.Name)
	}

	out := ReadResult{Columns: cols}
	for rows.Next() {
		if len(out.Rows) >= e.maxRows {
			out.Truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return ReadResult{}, err
		}
		out.Rows = append(out.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return ReadResult{}, err
	}
	return out, nil
}
