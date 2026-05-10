package plan

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/plandb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrNotFound = errors.New("not found")
var ErrTaskNotFound = errors.New("task not found")

type Repository interface {
	ListByDate(ctx context.Context, date time.Time) ([]PlanBlockResponse, error)
	Get(ctx context.Context, id int32) (PlanBlockResponse, error)
	GetTaskName(ctx context.Context, taskID int32) (string, error)
	HasOverlap(ctx context.Context, planDate, startedAt, endedAt time.Time, excludeID *int32) (bool, error)
	Create(ctx context.Context, planDate time.Time, startedAt, endedAt time.Time, taskID *int32, label string, note *string) (PlanBlockResponse, error)
	Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error)
	Delete(ctx context.Context, id int32) error
}

type PostgresRepository struct {
	q plandb.Querier
}

func NewRepository(q plandb.Querier) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func pgTimestamptzToPtr(ts pgtype.Timestamptz) *time.Time {
	if ts.Valid {
		t := ts.Time
		return &t
	}
	return nil
}

func (r *PostgresRepository) ListByDate(ctx context.Context, date time.Time) ([]PlanBlockResponse, error) {
	rows, err := r.q.ListPlanBlocksByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	out := make([]PlanBlockResponse, len(rows))
	for i, row := range rows {
		out[i] = PlanBlockResponse{
			ID:             row.ID,
			StartedAt:      row.StartedAt.Time,
			EndedAt:        row.EndedAt.Time,
			TaskID:         row.TaskID,
			TaskName:       row.TaskName,
			Label:          row.Label,
			Note:           row.Note,
			TaskType:       row.TaskType,
			TaskRecurrence: row.TaskRecurrence,
			TaskStartedAt:  pgTimestamptzToPtr(row.TaskStartedAt),
			TaskFinishedAt: pgTimestamptzToPtr(row.TaskFinishedAt),
		}
	}
	return out, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id int32) (PlanBlockResponse, error) {
	row, err := r.q.GetPlanBlock(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanBlockResponse{}, ErrNotFound
		}
		return PlanBlockResponse{}, err
	}

	return PlanBlockResponse{
		ID:             row.ID,
		StartedAt:      row.StartedAt.Time,
		EndedAt:        row.EndedAt.Time,
		TaskID:         row.TaskID,
		TaskName:       row.TaskName,
		Label:          row.Label,
		Note:           row.Note,
		TaskType:       row.TaskType,
		TaskRecurrence: row.TaskRecurrence,
		TaskStartedAt:  pgTimestamptzToPtr(row.TaskStartedAt),
		TaskFinishedAt: pgTimestamptzToPtr(row.TaskFinishedAt),
	}, nil
}

func (r *PostgresRepository) HasOverlap(ctx context.Context, planDate, startedAt, endedAt time.Time, excludeID *int32) (bool, error) {
	params := plandb.CountOverlappingPlanBlocksParams{
		PlanDate:  planDate,
		StartedAt: pgtype.Timestamptz{Time: startedAt, Valid: true},
		EndedAt:   pgtype.Timestamptz{Time: endedAt, Valid: true},
	}
	if excludeID != nil {
		params.HasExcludeID = true
		params.ExcludeID = *excludeID
	}
	count, err := r.q.CountOverlappingPlanBlocks(ctx, params)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresRepository) GetTaskName(ctx context.Context, taskID int32) (string, error) {
	name, err := r.q.GetTaskName(ctx, taskID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrTaskNotFound
		}
		return "", err
	}
	return name, nil
}

func (r *PostgresRepository) Create(ctx context.Context, planDate, startedAt, endedAt time.Time, taskID *int32, label string, note *string) (PlanBlockResponse, error) {
	row, err := r.q.CreatePlanBlock(ctx, plandb.CreatePlanBlockParams{
		PlanDate:  planDate,
		StartedAt: pgtype.Timestamptz{Time: startedAt, Valid: true},
		EndedAt:   pgtype.Timestamptz{Time: endedAt, Valid: true},
		TaskID:    taskID,
		Label:     label,
		Note:      note,
	})
	if err != nil {
		return PlanBlockResponse{}, err
	}

	// Refetch via Get so the response carries the joined task_* fields, matching
	// the shape of ListByDate.
	return r.Get(ctx, row.ID)
}

func (r *PostgresRepository) Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error) {
	params := plandb.UpdatePlanBlockParams{ID: req.ID}

	if req.StartedAt != nil {
		params.SetStartedAt = true
		params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt, Valid: true}
	}
	if req.EndedAt != nil {
		params.SetEndedAt = true
		params.EndedAt = pgtype.Timestamptz{Time: *req.EndedAt, Valid: true}
	}
	// plan_date is derived from started_at on the service side when provided.
	if req.StartedAt != nil {
		params.SetPlanDate = true
		params.PlanDate = req.StartedAt.UTC()
	}
	if req.ClearTask {
		params.ClearTaskID = true
	} else if req.TaskID != nil {
		params.SetTaskID = true
		params.TaskID = *req.TaskID
	}
	if req.Label != nil {
		params.SetLabel = true
		params.Label = *req.Label
	}
	if req.ClearNote {
		params.ClearNote = true
	} else if req.Note != nil {
		params.SetNote = true
		params.Note = *req.Note
	}

	row, err := r.q.UpdatePlanBlock(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanBlockResponse{}, ErrNotFound
		}
		return PlanBlockResponse{}, err
	}

	// Refetch via Get so the response carries the joined task_* fields, matching
	// the shape of ListByDate.
	return r.Get(ctx, row.ID)
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	return r.q.DeletePlanBlock(ctx, id)
}
