package habits

import (
	"context"
	"time"

	"gv-api/internal/database/habitsdb"
)

type Repository interface {
	GetHabitsWithLogs(ctx context.Context, date time.Time) ([]HabitWithLog, error)
	UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error
	CreateHabit(ctx context.Context, name string, description *string, frequency string, targetMin, targetMax *float32, recordingRequired bool) (CreateHabitResponse, error)
	DeleteHabit(ctx context.Context, id int32) error
	GetHabitByID(ctx context.Context, id int32) (habitsdb.GetHabitByIDRow, error)
	GetHabitLogs(ctx context.Context, habitID int32) ([]habitsdb.HabitLog, error)
	UpdateHabitStreak(ctx context.Context, id int32, currentStreak, longestStreak int32) error
	GetHabitHistory(ctx context.Context, habitID int32, frequency string, startAt, endAt time.Time) ([]HistoryPoint, error)
	GetHabitHistoryAvg(ctx context.Context, habitID int32, frequency string, startAt, endAt time.Time) ([]HistoryPoint, error)
}

type PostgresRepository struct {
	q habitsdb.Querier
}

func NewRepository(q habitsdb.Querier) *PostgresRepository {
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
			ID:                row.ID,
			Name:              row.Name,
			Description:       row.Description,
			Frequency:         row.Frequency,
			TargetMin:         row.TargetMin,
			TargetMax:         row.TargetMax,
			RecordingRequired: row.RecordingRequired,
			LogValue:          row.LogValue,
			PeriodValue:       row.PeriodValue,
			CurrentStreak:     row.CurrentStreak,
			LongestStreak:     row.LongestStreak,
		}

		results = append(results, habit)
	}
	return results, nil
}

func (r *PostgresRepository) UpsertLog(ctx context.Context, habitID int32, date time.Time, value float32) error {
	params := habitsdb.UpsertLogParams{
		HabitID: habitID,
		LogDate: date,
		Value:   value,
	}
	return r.q.UpsertLog(ctx, params)
}

func (r *PostgresRepository) DeleteHabit(ctx context.Context, id int32) error {
	return r.q.DeleteHabit(ctx, id)
}

func (r *PostgresRepository) CreateHabit(ctx context.Context, name string, description *string, frequency string, targetMin, targetMax *float32, recordingRequired bool) (CreateHabitResponse, error) {
	habit, err := r.q.CreateHabit(ctx, habitsdb.CreateHabitParams{
		Name:              name,
		Description:       description,
		Frequency:         frequency,
		TargetMin:         targetMin,
		TargetMax:         targetMax,
		RecordingRequired: recordingRequired,
	})
	if err != nil {
		return CreateHabitResponse{}, err
	}
	return CreateHabitResponse{
		ID:                habit.ID,
		Name:              habit.Name,
		Description:       habit.Description,
		Frequency:         habit.Frequency,
		TargetMin:         habit.TargetMin,
		TargetMax:         habit.TargetMax,
		RecordingRequired: habit.RecordingRequired,
		CurrentStreak:     habit.CurrentStreak,
		LongestStreak:     habit.LongestStreak,
	}, nil
}

func (r *PostgresRepository) GetHabitByID(ctx context.Context, id int32) (habitsdb.GetHabitByIDRow, error) {
	return r.q.GetHabitByID(ctx, id)
}

func (r *PostgresRepository) GetHabitLogs(ctx context.Context, habitID int32) ([]habitsdb.HabitLog, error) {
	return r.q.GetHabitLogs(ctx, habitID)
}

func (r *PostgresRepository) UpdateHabitStreak(ctx context.Context, id int32, currentStreak, longestStreak int32) error {
	return r.q.UpdateHabitStreak(ctx, habitsdb.UpdateHabitStreakParams{
		ID:            id,
		CurrentStreak: currentStreak,
		LongestStreak: longestStreak,
	})
}

func (r *PostgresRepository) GetHabitHistory(ctx context.Context, habitID int32, frequency string, startAt, endAt time.Time) ([]HistoryPoint, error) {
	rows, err := r.q.GetHabitHistory(ctx, habitsdb.GetHabitHistoryParams{
		HabitID:   habitID,
		Frequency: frequency,
		StartAt:   startAt,
		EndAt:     endAt,
	})
	if err != nil {
		return nil, err
	}

	results := make([]HistoryPoint, len(rows))
	for i, row := range rows {
		results[i] = HistoryPoint{
			Date:  row.Date.Format("2006-01-02"),
			Value: row.Value,
		}
	}
	return results, nil
}

func (r *PostgresRepository) GetHabitHistoryAvg(ctx context.Context, habitID int32, frequency string, startAt, endAt time.Time) ([]HistoryPoint, error) {
	rows, err := r.q.GetHabitHistoryAvg(ctx, habitsdb.GetHabitHistoryAvgParams{
		HabitID:   habitID,
		Frequency: frequency,
		StartAt:   startAt,
		EndAt:     endAt,
	})
	if err != nil {
		return nil, err
	}

	results := make([]HistoryPoint, len(rows))
	for i, row := range rows {
		results[i] = HistoryPoint{
			Date:  row.Date.Format("2006-01-02"),
			Value: row.Value,
		}
	}
	return results, nil
}
