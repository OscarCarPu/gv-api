package plan

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/database/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// CreatePlanBlockParams is a struct rather than positional args: it already grew past the
// point where a wrong-order call is easy to make (event_ref/commitment_id are both *int32
// -ish and sit right next to task_id).
type CreatePlanBlockParams struct {
	PlanDate     time.Time
	StartedAt    time.Time
	EndedAt      time.Time
	TaskID       *int32
	Label        string
	Note         *string
	EventRef     *string
	CommitmentID *int32
}

type Repository interface {
	ListByDate(ctx context.Context, date time.Time) ([]PlanBlockResponse, error)
	ListByDateRange(ctx context.Context, from, to time.Time) ([]PlanBlockResponse, error)
	Get(ctx context.Context, id int32) (PlanBlockResponse, error)
	GetByEventRef(ctx context.Context, eventRef string) (PlanBlockResponse, error)
	GetTaskName(ctx context.Context, taskID int32) (string, error)
	HasOverlap(ctx context.Context, startedAt, endedAt time.Time, excludeID *int32) (bool, error)
	Create(ctx context.Context, params CreatePlanBlockParams) (PlanBlockResponse, error)
	// CreateGenerated inserts a commitment-generated block. created is false, with no error,
	// when a concurrent request already generated the same (commitment_id, plan_date) occurrence
	// — see plan_blocks_commitment_date_uidx.
	CreateGenerated(ctx context.Context, params CreatePlanBlockParams) (created bool, err error)
	Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error)
	UpdateTimes(ctx context.Context, id int32, startedAt, endedAt time.Time) error
	ClearEventRef(ctx context.Context, id int32) error
	Delete(ctx context.Context, id int32) error
	DeleteEndingAfter(ctx context.Context, t time.Time) error

	SumBusyHoursByDate(ctx context.Context, from, to time.Time, timezone string) (map[string]decimal.Decimal, error)
	SumPlannedHoursByTask(ctx context.Context, taskIDs []int32, from time.Time) (map[int32]decimal.Decimal, error)

	ListActiveCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error)
	ListCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error)
	CreateCommitment(ctx context.Context, req CreateCommitmentRequest) (RecurringCommitmentResponse, error)
	UpdateCommitment(ctx context.Context, req UpdateCommitmentRequest) (RecurringCommitmentResponse, error)
	DeleteCommitment(ctx context.Context, id int32) error
	ListPlanBlockDatesByCommitment(ctx context.Context, commitmentID int32, from, to time.Time) (map[string]bool, error)
	ListCommitmentSkips(ctx context.Context, commitmentID int32, from, to time.Time) (map[string]bool, error)
	InsertCommitmentSkip(ctx context.Context, commitmentID int32, skipDate time.Time) error
}

type PostgresRepository struct {
	q *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: gvdb.New(pool)}
}

func toPlanBlockResponse(id int32, planDate time.Time, startedAt, endedAt pgtype.Timestamptz,
	taskID *int32, label string, note, eventRef *string, commitmentID *int32,
	taskName, taskType *string, taskRecurrence *int32, taskStartedAt, taskFinishedAt pgtype.Timestamptz,
) PlanBlockResponse {
	return PlanBlockResponse{
		ID:             id,
		PlanDate:       planDate,
		StartedAt:      startedAt.Time,
		EndedAt:        endedAt.Time,
		TaskID:         taskID,
		TaskName:       taskName,
		Label:          label,
		Note:           note,
		EventRef:       eventRef,
		CommitmentID:   commitmentID,
		TaskType:       taskType,
		TaskRecurrence: taskRecurrence,
		TaskStartedAt:  pgconv.TimePtr(taskStartedAt),
		TaskFinishedAt: pgconv.TimePtr(taskFinishedAt),
	}
}

func (r *PostgresRepository) ListByDate(ctx context.Context, date time.Time) ([]PlanBlockResponse, error) {
	rows, err := r.q.ListPlanBlocksByDate(ctx, date)
	if err != nil {
		return nil, err
	}

	out := make([]PlanBlockResponse, len(rows))
	for i, row := range rows {
		out[i] = toPlanBlockResponse(row.ID, row.PlanDate, row.StartedAt, row.EndedAt, row.TaskID, row.Label,
			row.Note, row.EventRef, row.CommitmentID, row.TaskName, row.TaskType, row.TaskRecurrence,
			row.TaskStartedAt, row.TaskFinishedAt)
	}
	return out, nil
}

func (r *PostgresRepository) ListByDateRange(ctx context.Context, from, to time.Time) ([]PlanBlockResponse, error) {
	rows, err := r.q.ListPlanBlocksByDateRange(ctx, gvdb.ListPlanBlocksByDateRangeParams{
		FromDate: pgtype.Timestamptz{Time: from, Valid: true},
		ToDate:   pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	out := make([]PlanBlockResponse, len(rows))
	for i, row := range rows {
		out[i] = toPlanBlockResponse(row.ID, row.PlanDate, row.StartedAt, row.EndedAt, row.TaskID, row.Label,
			row.Note, row.EventRef, row.CommitmentID, row.TaskName, row.TaskType, row.TaskRecurrence,
			row.TaskStartedAt, row.TaskFinishedAt)
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

	return toPlanBlockResponse(row.ID, row.PlanDate, row.StartedAt, row.EndedAt, row.TaskID, row.Label,
		row.Note, row.EventRef, row.CommitmentID, row.TaskName, row.TaskType, row.TaskRecurrence,
		row.TaskStartedAt, row.TaskFinishedAt), nil
}

func (r *PostgresRepository) GetByEventRef(ctx context.Context, eventRef string) (PlanBlockResponse, error) {
	row, err := r.q.GetPlanBlockByEventRef(ctx, &eventRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlanBlockResponse{}, ErrNotFound
		}
		return PlanBlockResponse{}, err
	}

	return PlanBlockResponse{
		ID:           row.ID,
		PlanDate:     row.PlanDate,
		StartedAt:    row.StartedAt.Time,
		EndedAt:      row.EndedAt.Time,
		TaskID:       row.TaskID,
		Label:        row.Label,
		Note:         row.Note,
		EventRef:     row.EventRef,
		CommitmentID: row.CommitmentID,
	}, nil
}

func (r *PostgresRepository) HasOverlap(ctx context.Context, startedAt, endedAt time.Time, excludeID *int32) (bool, error) {
	params := gvdb.CountOverlappingPlanBlocksParams{
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

func (r *PostgresRepository) Create(ctx context.Context, params CreatePlanBlockParams) (PlanBlockResponse, error) {
	row, err := r.q.CreatePlanBlock(ctx, gvdb.CreatePlanBlockParams{
		PlanDate:     params.PlanDate,
		StartedAt:    pgtype.Timestamptz{Time: params.StartedAt, Valid: true},
		EndedAt:      pgtype.Timestamptz{Time: params.EndedAt, Valid: true},
		TaskID:       params.TaskID,
		Label:        params.Label,
		Note:         params.Note,
		EventRef:     params.EventRef,
		CommitmentID: params.CommitmentID,
	})
	if err != nil {
		return PlanBlockResponse{}, err
	}

	// Refetch via Get so the response carries the joined task_* fields, matching
	// the shape of ListByDate.
	return r.Get(ctx, row.ID)
}

func (r *PostgresRepository) CreateGenerated(ctx context.Context, params CreatePlanBlockParams) (bool, error) {
	_, err := r.q.CreateGeneratedPlanBlock(ctx, gvdb.CreateGeneratedPlanBlockParams{
		PlanDate:     params.PlanDate,
		StartedAt:    pgtype.Timestamptz{Time: params.StartedAt, Valid: true},
		EndedAt:      pgtype.Timestamptz{Time: params.EndedAt, Valid: true},
		TaskID:       params.TaskID,
		Label:        params.Label,
		CommitmentID: params.CommitmentID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *PostgresRepository) Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error) {
	params := gvdb.UpdatePlanBlockParams{ID: req.ID}

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
	if req.ClearCommitmentID {
		params.ClearCommitmentID = true
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

func (r *PostgresRepository) UpdateTimes(ctx context.Context, id int32, startedAt, endedAt time.Time) error {
	return r.q.UpdatePlanBlockTimes(ctx, gvdb.UpdatePlanBlockTimesParams{
		ID:        id,
		StartedAt: pgtype.Timestamptz{Time: startedAt, Valid: true},
		EndedAt:   pgtype.Timestamptz{Time: endedAt, Valid: true},
		PlanDate:  time.Date(startedAt.Year(), startedAt.Month(), startedAt.Day(), 0, 0, 0, 0, startedAt.Location()),
	})
}

func (r *PostgresRepository) ClearEventRef(ctx context.Context, id int32) error {
	return r.q.ClearPlanBlockEventRef(ctx, id)
}

func (r *PostgresRepository) Delete(ctx context.Context, id int32) error {
	return r.q.DeletePlanBlock(ctx, id)
}

func (r *PostgresRepository) DeleteEndingAfter(ctx context.Context, t time.Time) error {
	return r.q.DeletePlanBlocksEndingAfter(ctx, pgtype.Timestamptz{Time: t, Valid: true})
}

func (r *PostgresRepository) SumBusyHoursByDate(ctx context.Context, from, to time.Time, timezone string) (map[string]decimal.Decimal, error) {
	// SumBusyHoursByDate treats @to_date as the last day INCLUDED, not exclusive — see the
	// query comment for why (an inline `- interval '1 day'` there breaks sqlc's rewriter).
	rows, err := r.q.SumBusyHoursByDate(ctx, gvdb.SumBusyHoursByDateParams{
		Timezone: timezone,
		FromDate: from,
		ToDate:   to.AddDate(0, 0, -1),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]decimal.Decimal, len(rows))
	for _, row := range rows {
		out[row.Day.Format("2006-01-02")] = row.Hours
	}
	return out, nil
}

func (r *PostgresRepository) SumPlannedHoursByTask(ctx context.Context, taskIDs []int32, from time.Time) (map[int32]decimal.Decimal, error) {
	if len(taskIDs) == 0 {
		return map[int32]decimal.Decimal{}, nil
	}
	rows, err := r.q.SumPlannedHoursByTask(ctx, gvdb.SumPlannedHoursByTaskParams{
		TaskIds: taskIDs,
		FromTs:  pgtype.Timestamptz{Time: from, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make(map[int32]decimal.Decimal, len(rows))
	for _, row := range rows {
		if row.TaskID != nil {
			out[*row.TaskID] = row.Hours
		}
	}
	return out, nil
}

func timeOfDayToPg(s string) (pgtype.Time, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return pgtype.Time{}, err
	}
	micros := t.Hour()*3600_000_000 + t.Minute()*60_000_000 + t.Second()*1_000_000
	return pgtype.Time{Microseconds: int64(micros), Valid: true}, nil
}

func timeOfDayFromPg(t pgtype.Time) string {
	total := t.Microseconds / 1_000_000
	h := total / 3600
	m := (total % 3600) / 60
	return time.Date(0, 1, 1, int(h), int(m), 0, 0, time.UTC).Format("15:04")
}

func daysOfWeekToInt16(days []int32) []int16 {
	out := make([]int16, len(days))
	for i, d := range days {
		out[i] = int16(d)
	}
	return out
}

func daysOfWeekFromInt16(days []int16) []int32 {
	out := make([]int32, len(days))
	for i, d := range days {
		out[i] = int32(d)
	}
	return out
}

func toCommitmentResponse(id, taskID int32, taskName, label string, daysOfWeek []int16, startTime, endTime pgtype.Time, active bool) RecurringCommitmentResponse {
	return RecurringCommitmentResponse{
		ID:         id,
		TaskID:     taskID,
		TaskName:   taskName,
		Label:      label,
		DaysOfWeek: daysOfWeekFromInt16(daysOfWeek),
		StartTime:  timeOfDayFromPg(startTime),
		EndTime:    timeOfDayFromPg(endTime),
		Active:     active,
	}
}

func (r *PostgresRepository) ListActiveCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error) {
	rows, err := r.q.ListActiveCommitments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RecurringCommitmentResponse, len(rows))
	for i, row := range rows {
		name, err := r.q.GetTaskName(ctx, row.TaskID)
		if err != nil {
			return nil, err
		}
		out[i] = toCommitmentResponse(row.ID, row.TaskID, name, row.Label, row.DaysOfWeek, row.StartTime, row.EndTime, row.Active)
	}
	return out, nil
}

func (r *PostgresRepository) ListCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error) {
	rows, err := r.q.ListCommitments(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RecurringCommitmentResponse, len(rows))
	for i, row := range rows {
		out[i] = toCommitmentResponse(row.ID, row.TaskID, row.TaskName, row.Label, row.DaysOfWeek, row.StartTime, row.EndTime, row.Active)
	}
	return out, nil
}

func (r *PostgresRepository) CreateCommitment(ctx context.Context, req CreateCommitmentRequest) (RecurringCommitmentResponse, error) {
	startTime, err := timeOfDayToPg(req.StartTime)
	if err != nil {
		return RecurringCommitmentResponse{}, err
	}
	endTime, err := timeOfDayToPg(req.EndTime)
	if err != nil {
		return RecurringCommitmentResponse{}, err
	}
	row, err := r.q.CreateCommitment(ctx, gvdb.CreateCommitmentParams{
		TaskID:     req.TaskID,
		Label:      req.Label,
		DaysOfWeek: daysOfWeekToInt16(req.DaysOfWeek),
		StartTime:  startTime,
		EndTime:    endTime,
	})
	if err != nil {
		return RecurringCommitmentResponse{}, err
	}
	name, err := r.q.GetTaskName(ctx, row.TaskID)
	if err != nil {
		return RecurringCommitmentResponse{}, err
	}
	return toCommitmentResponse(row.ID, row.TaskID, name, row.Label, row.DaysOfWeek, row.StartTime, row.EndTime, row.Active), nil
}

func (r *PostgresRepository) UpdateCommitment(ctx context.Context, req UpdateCommitmentRequest) (RecurringCommitmentResponse, error) {
	params := gvdb.UpdateCommitmentParams{ID: req.ID}
	if req.Label != nil {
		params.SetLabel = true
		params.Label = *req.Label
	}
	if req.DaysOfWeek != nil {
		params.SetDaysOfWeek = true
		params.DaysOfWeek = daysOfWeekToInt16(*req.DaysOfWeek)
	}
	if req.StartTime != nil {
		t, err := timeOfDayToPg(*req.StartTime)
		if err != nil {
			return RecurringCommitmentResponse{}, err
		}
		params.SetStartTime = true
		params.StartTime = t
	}
	if req.EndTime != nil {
		t, err := timeOfDayToPg(*req.EndTime)
		if err != nil {
			return RecurringCommitmentResponse{}, err
		}
		params.SetEndTime = true
		params.EndTime = t
	}
	if req.Active != nil {
		params.SetActive = true
		params.Active = *req.Active
	}

	row, err := r.q.UpdateCommitment(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RecurringCommitmentResponse{}, ErrNotFound
		}
		return RecurringCommitmentResponse{}, err
	}
	name, err := r.q.GetTaskName(ctx, row.TaskID)
	if err != nil {
		return RecurringCommitmentResponse{}, err
	}
	return toCommitmentResponse(row.ID, row.TaskID, name, row.Label, row.DaysOfWeek, row.StartTime, row.EndTime, row.Active), nil
}

func (r *PostgresRepository) DeleteCommitment(ctx context.Context, id int32) error {
	return r.q.DeleteCommitment(ctx, id)
}

func (r *PostgresRepository) ListPlanBlockDatesByCommitment(ctx context.Context, commitmentID int32, from, to time.Time) (map[string]bool, error) {
	dates, err := r.q.ListPlanBlocksByCommitment(ctx, gvdb.ListPlanBlocksByCommitmentParams{
		CommitmentID: &commitmentID,
		FromDate:     from,
		ToDate:       to,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(dates))
	for _, d := range dates {
		out[d.Format("2006-01-02")] = true
	}
	return out, nil
}

func (r *PostgresRepository) ListCommitmentSkips(ctx context.Context, commitmentID int32, from, to time.Time) (map[string]bool, error) {
	dates, err := r.q.ListRecurringCommitmentSkips(ctx, gvdb.ListRecurringCommitmentSkipsParams{
		CommitmentID: commitmentID,
		FromDate:     from,
		ToDate:       to,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(dates))
	for _, d := range dates {
		out[d.Format("2006-01-02")] = true
	}
	return out, nil
}

func (r *PostgresRepository) InsertCommitmentSkip(ctx context.Context, commitmentID int32, skipDate time.Time) error {
	return r.q.InsertRecurringCommitmentSkip(ctx, gvdb.InsertRecurringCommitmentSkipParams{
		CommitmentID: commitmentID,
		SkipDate:     skipDate,
	})
}
