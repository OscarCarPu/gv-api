package habits

import (
	"context"
	"time"

	"gv-api/internal/database/sqlc"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
	q  *sqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{
		db: db,
		q:  sqlc.New(db),
	}
}

func (r *PostgresRepository) GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error) {
	rows, err := r.q.GetHabitsWithLogs(ctx, date)
	if err != nil {
		return nil, err
	}

	var results []HabitWithLog
	for _, row := range rows {
		habit := HabitWithLog{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			LogValue:    row.Value,
		}

		results = append(results, habit)
	}
	return results, nil
}

func (r *PostgresRepository) UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error {
	params := sqlc.UpsertLogParams{
		HabitID: int32(habitID),
		LogDate: date,
		Value:   value,
	}
	return r.q.UpsertLog(ctx, params)
}
