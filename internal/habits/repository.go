package habits

import (
	"context"
	"time"

	"gv-api/internal/database/postgres"

	"github.com/jmoiron/sqlx"
)

type Repository interface {
	// Update return type to match DTO
	GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	UpsertLog(ctx context.Context, habitID int32, date time.Time, value float64) error
}

type PostgresRepository struct {
	db *sqlx.DB
	q  *postgres.Queries
}

func NewRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{
		db: db,
		q:  postgres.New(db),
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
			ID:   row.ID,
			Name: row.Name,
		}
		if row.Description.Valid {
			habit.Description = &row.Description.String
		}
		if row.Value.Valid {
			logValue := row.Value.Float64
			habit.LogValue = &logValue
		}
		results = append(results, habit)
	}
	return results, nil
}

func (r *PostgresRepository) UpsertLog(ctx context.Context, habitID int32, date time.Time, value float64) error {
	params := postgres.UpsertLogParams{
		HabitID: int32(habitID),
		LogDate: date,
		Value:   float32(value), // Cast generic float64 to DB type float32
	}
	return r.q.UpsertLog(ctx, params)
}
