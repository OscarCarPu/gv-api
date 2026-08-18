package tasks

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/database/pgconv"
	"gv-api/internal/history"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *PostgresRepository) CreateTimeEntry(ctx context.Context, taskID int32, startedAt time.Time, finishedAt *time.Time, comment *string) (TimeEntryResponse, error) {
	pgStartedAt := pgtype.Timestamptz{Time: startedAt, Valid: true}

	var pgFinishedAt pgtype.Timestamptz
	if finishedAt != nil {
		pgFinishedAt = pgtype.Timestamptz{Time: *finishedAt, Valid: true}
	}

	row, err := r.q.CreateTimeEntry(ctx, gvdb.CreateTimeEntryParams{
		TaskID:     taskID,
		StartedAt:  pgStartedAt,
		FinishedAt: pgFinishedAt,
		Comment:    comment,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return TimeEntryResponse{}, ErrActiveTimeEntryExists
		}
		return TimeEntryResponse{}, err
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: pgconv.TimePtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error) {
	params := gvdb.UpdateTimeEntryParams{ID: req.ID}
	if req.TaskID != nil {
		params.SetTaskID = true
		params.TaskID = *req.TaskID
	}
	if req.StartedAt != nil {
		params.SetStartedAt = true
		params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt, Valid: true}
	}
	if req.FinishedAt.Set {
		params.SetFinishedAt = true
		if req.FinishedAt.Value != nil {
			params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt.Value, Valid: true}
		}
	}
	if req.Comment != nil {
		params.SetComment = true
		params.Comment = *req.Comment
	}

	row, err := r.q.UpdateTimeEntry(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TimeEntryResponse{}, ErrNotFound
		}
		return TimeEntryResponse{}, err
	}

	return TimeEntryResponse{
		ID:         row.ID,
		TaskID:     row.TaskID,
		StartedAt:  row.StartedAt.Time,
		FinishedAt: pgconv.TimePtr(row.FinishedAt),
		Comment:    row.Comment,
	}, nil
}

func (r *PostgresRepository) DeleteTimeEntry(ctx context.Context, id int32) error {
	return r.deleteByID(ctx, "time_entries", id)
}

func (r *PostgresRepository) GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error) {
	rows, err := r.q.GetTimeEntriesByTaskID(ctx, taskID)
	if err != nil {
		return TaskTimeEntriesResponse{}, err
	}

	if len(rows) == 0 {
		return TaskTimeEntriesResponse{}, ErrNotFound
	}

	first := rows[0]

	var entries []TimeEntryResponse
	for _, row := range rows {
		if row.TimeEntryID == nil {
			continue
		}
		entries = append(entries, TimeEntryResponse{
			ID:         *row.TimeEntryID,
			TaskID:     row.TaskID,
			StartedAt:  row.EntryStartedAt.Time,
			FinishedAt: pgconv.TimePtr(row.EntryFinishedAt),
			Comment:    row.Comment,
		})
	}

	if entries == nil {
		entries = []TimeEntryResponse{}
	}

	dependsOn, err := unmarshalDepRefs(first.DependsOn)
	if err != nil {
		return TaskTimeEntriesResponse{}, err
	}
	blocks, err := unmarshalDepRefs(first.Blocks)
	if err != nil {
		return TaskTimeEntriesResponse{}, err
	}
	return TaskTimeEntriesResponse{
		Task: TaskDetailResponse{
			ID:          first.TaskID,
			ProjectID:   first.ProjectID,
			Name:        first.Name,
			Description: first.Description,
			DueAt:       pgconv.DatePtr(first.DueAt),
			StartedAt:   pgconv.TimePtr(first.TaskStartedAt),
			FinishedAt:  pgconv.TimePtr(first.TaskFinishedAt),
			TaskType:    first.TaskType,
			Recurrence:  first.Recurrence,
			Priority:    first.Priority,
			TimeSpent:   first.TimeSpent,
			DependsOn:   dependsOn,
			Blocks:      blocks,
			Blocked:     first.Blocked,
		},
		TimeEntries: entries,
	}, nil
}

func (r *PostgresRepository) GetActiveTimeEntry(ctx context.Context) (ActiveTimeEntryResponse, error) {
	row, err := r.q.GetActiveTimeEntry(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ActiveTimeEntryResponse{}, ErrNotFound
		}
		return ActiveTimeEntryResponse{}, err
	}

	return ActiveTimeEntryResponse{
		ID:              row.ID,
		TaskID:          row.TaskID,
		StartedAt:       row.StartedAt.Time,
		FinishedAt:      pgconv.TimePtr(row.FinishedAt),
		Comment:         row.Comment,
		TaskName:        row.TaskName,
		TaskDescription: row.TaskDescription,
		TaskType:        row.TaskType,
		Recurrence:      row.Recurrence,
		Priority:        row.Priority,
		ProjectName:     row.ProjectName,
	}, nil
}

func (r *PostgresRepository) GetTimeEntrySummary(ctx context.Context, todayStart, weekStart time.Time) (TimeEntrySummaryResponse, error) {
	row, err := r.q.GetTimeEntrySummary(ctx, gvdb.GetTimeEntrySummaryParams{
		TodayStart: pgtype.Timestamptz{Time: todayStart, Valid: true},
		WeekStart:  pgtype.Timestamptz{Time: weekStart, Valid: true},
	})
	if err != nil {
		return TimeEntrySummaryResponse{}, err
	}

	return TimeEntrySummaryResponse{
		Today: row.Today,
		Week:  row.Week,
	}, nil
}

func (r *PostgresRepository) GetTimeEntryHistory(ctx context.Context, frequency, timezone string, startAt, endAt time.Time) ([]history.Point, error) {
	rows, err := r.q.GetTimeEntryHistory(ctx, gvdb.GetTimeEntryHistoryParams{
		Frequency: frequency,
		Timezone:  timezone,
		StartAt:   startAt,
		EndAt:     endAt,
	})
	if err != nil {
		return nil, err
	}

	results := make([]history.Point, len(rows))
	for i, row := range rows {
		results[i] = history.Point{
			Date:  row.Date.Format("2006-01-02"),
			Value: row.Value,
		}
	}
	return results, nil
}

func (r *PostgresRepository) GetTimeEntriesByDateRange(ctx context.Context, startTime, endTime time.Time) ([]TimeEntryWithTaskResponse, error) {
	rows, err := r.q.GetTimeEntriesByDateRange(ctx, gvdb.GetTimeEntriesByDateRangeParams{
		StartTime: pgtype.Timestamptz{Time: startTime, Valid: true},
		EndTime:   pgtype.Timestamptz{Time: endTime, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	entries := make([]TimeEntryWithTaskResponse, len(rows))
	for i, row := range rows {
		entries[i] = TimeEntryWithTaskResponse{
			ID:             row.ID,
			TaskID:         row.TaskID,
			TaskName:       row.TaskName,
			TaskType:       row.TaskType,
			Recurrence:     row.Recurrence,
			Priority:       row.Priority,
			ProjectID:      row.ProjectID,
			ProjectName:    row.ProjectName,
			StartedAt:      row.StartedAt.Time,
			FinishedAt:     pgconv.TimePtr(row.FinishedAt),
			Comment:        row.Comment,
			TaskFinishedAt: pgconv.TimePtr(row.TaskFinishedAt),
			TimeSpent:      row.TimeSpent,
		}
	}

	return entries, nil
}
