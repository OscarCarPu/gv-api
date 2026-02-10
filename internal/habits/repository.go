package habits

import (
	"context"
	"time"

	"gv-api/internal/database/sqlc"
)

type Repository interface {
	GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error
	CreateHabit(ctx context.Context, name string, description *string) (CreateHabitResponse, error)
}

type PostgresRepository struct {
	q sqlc.Querier
}

func NewRepository(q sqlc.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
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
		HabitID: habitID,
		LogDate: date,
		Value:   value,
	}
	return r.q.UpsertLog(ctx, params)
}

func (r *PostgresRepository) CreateHabit(ctx context.Context, name string, description *string) (CreateHabitResponse, error) {
	habit, err := r.q.CreateHabit(ctx, sqlc.CreateHabitParams{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return CreateHabitResponse{}, err
	}
	return CreateHabitResponse{
		ID:          habit.ID,
		Name:        habit.Name,
		Description: habit.Description,
	}, nil
}
