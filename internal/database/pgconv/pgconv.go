// Package pgconv converts nullable pgx column types to Go pointers.
package pgconv

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TimePtr(ts pgtype.Timestamptz) *time.Time {
	if ts.Valid {
		t := ts.Time
		return &t
	}
	return nil
}

func DatePtr(d pgtype.Date) *time.Time {
	if d.Valid {
		t := d.Time
		return &t
	}
	return nil
}

// AnyDatePtr handles a date column sqlc typed as any (nullability lost through a CTE).
func AnyDatePtr(v any) *time.Time {
	t, ok := v.(time.Time)
	if !ok {
		return nil
	}
	return &t
}
