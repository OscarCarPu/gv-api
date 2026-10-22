package calendar

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	List(ctx context.Context, from, to time.Time) ([]Event, error)
	Get(ctx context.Context, id int32) (Event, error)
	Create(ctx context.Context, name string, start, end time.Time) (Event, error)
	Update(ctx context.Context, id int32, req UpdateEventRequest) (Event, error)
	Delete(ctx context.Context, id int32) error
}

type PostgresRepository struct {
	q *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: gvdb.New(pool)}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

func toEvent(e gvdb.CalendarEvent) Event {
	return Event{ID: e.ID, Name: e.Name, StartTime: e.StartTime.Time, EndTime: e.EndTime.Time}
}

func (r *PostgresRepository) List(ctx context.Context, from, to time.Time) ([]Event, error) {
	rows, err := r.q.ListCalendarEvents(ctx, gvdb.ListCalendarEventsParams{RangeStart: ts(from), RangeEnd: ts(to)})
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEvent(row))
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id int32) (Event, error) {
	row, err := r.q.GetCalendarEvent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	return toEvent(row), nil
}

func (r *PostgresRepository) Create(ctx context.Context, name string, start, end time.Time) (Event, error) {
	row, err := r.q.CreateCalendarEvent(ctx, gvdb.CreateCalendarEventParams{
		Name: name, StartTime: ts(start), EndTime: ts(end),
	})
	if err != nil {
		return Event{}, err
	}
	return toEvent(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id int32, req UpdateEventRequest) (Event, error) {
	row, err := r.q.UpdateCalendarEvent(ctx, gvdb.UpdateCalendarEventParams{
		ID: id, Name: req.Name, StartTime: tsPtr(req.StartTime), EndTime: tsPtr(req.EndTime),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	return toEvent(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	n, err := r.q.DeleteCalendarEvent(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
