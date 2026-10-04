package backup

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PgLocker struct {
	db *pgxpool.Pool
}

func NewPgLocker(db *pgxpool.Pool) *PgLocker {
	return &PgLocker{db: db}
}

func (l *PgLocker) TryLock(ctx context.Context) (func(), error) {
	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var ok bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtext('backup'))").Scan(&ok); err != nil {
		conn.Release()
		return nil, err
	}
	if !ok {
		conn.Release()
		return nil, ErrRunning
	}
	return func() {
		if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_unlock(hashtext('backup'))"); err != nil {
			_ = conn.Conn().Close(context.Background())
		}
		conn.Release()
	}, nil
}
