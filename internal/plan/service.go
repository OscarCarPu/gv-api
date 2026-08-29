package plan

import (
	"context"
	"errors"
	"strings"
	"time"

	"gv-api/internal/tasks"

	"github.com/shopspring/decimal"
)

type tasksSummaryProvider interface {
	GetTimeEntrySummary(ctx context.Context) (tasks.TimeEntrySummaryResponse, error)
}

type Service struct {
	repo     Repository
	tasksSvc tasksSummaryProvider
	location *time.Location
}

func NewService(repo Repository, tasksSvc tasksSummaryProvider, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{repo: repo, tasksSvc: tasksSvc, location: loc}
}

func (s *Service) GetToday(ctx context.Context) (PlanTodayResponse, error) {
	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)

	blocks, err := s.repo.ListByDate(ctx, today)
	if err != nil {
		return PlanTodayResponse{}, err
	}

	var taskSecs, freeSecs int64
	for _, b := range blocks {
		secs := int64(b.EndedAt.Sub(b.StartedAt).Seconds())
		if b.TaskID != nil {
			taskSecs += secs
		} else {
			freeSecs += secs
		}
	}

	budget, err := s.tasksSvc.GetTimeEntrySummary(ctx)
	if err != nil {
		return PlanTodayResponse{}, err
	}

	return PlanTodayResponse{
		Date:   today.Format("2006-01-02"),
		Blocks: blocks,
		Totals: PlanTotals{TaskSeconds: taskSecs, FreeSeconds: freeSecs},
		Budget: budget,
	}, nil
}

func (s *Service) GetRange(ctx context.Context, from, to time.Time) (PlanRangeResponse, error) {
	if err := s.EnsureRecurringBlocks(ctx, from, to); err != nil {
		return PlanRangeResponse{}, err
	}
	blocks, err := s.repo.ListByDateRange(ctx, from, to)
	if err != nil {
		return PlanRangeResponse{}, err
	}
	return PlanRangeResponse{
		From:   from.Format("2006-01-02"),
		To:     to.Format("2006-01-02"),
		Blocks: blocks,
	}, nil
}

// BusyHoursByDate is what capacity.Service needs to compute free/busy per day.
func (s *Service) BusyHoursByDate(ctx context.Context, from, to time.Time) (map[string]decimal.Decimal, error) {
	if err := s.EnsureRecurringBlocks(ctx, from, to); err != nil {
		return nil, err
	}
	return s.repo.SumBusyHoursByDate(ctx, from, to, s.location.String())
}

// PlannedHoursByTask is what tasks.Service needs to keep a task's remaining_hours from
// double-counting time it has already scheduled for itself.
func (s *Service) PlannedHoursByTask(ctx context.Context, taskIDs []int32, from time.Time) (map[int32]decimal.Decimal, error) {
	if len(taskIDs) == 0 {
		return map[int32]decimal.Decimal{}, nil
	}
	return s.repo.SumPlannedHoursByTask(ctx, taskIDs, from)
}

func (s *Service) Create(ctx context.Context, req CreatePlanBlockRequest) (PlanBlockResponse, error) {
	if !req.EndedAt.After(req.StartedAt) {
		return PlanBlockResponse{}, ErrInvalidTimeRange
	}

	label, err := s.resolveLabel(ctx, req.TaskID, req.Label)
	if err != nil {
		return PlanBlockResponse{}, err
	}

	localStart := req.StartedAt.In(s.location)
	planDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, s.location)

	overlap, err := s.repo.HasOverlap(ctx, req.StartedAt, req.EndedAt, nil)
	if err != nil {
		return PlanBlockResponse{}, err
	}
	if overlap {
		return PlanBlockResponse{}, ErrOverlap
	}

	return s.repo.Create(ctx, CreatePlanBlockParams{
		PlanDate:  planDate,
		StartedAt: req.StartedAt,
		EndedAt:   req.EndedAt,
		TaskID:    req.TaskID,
		Label:     label,
		Note:      req.Note,
		EventRef:  req.EventRef,
	})
}

func (s *Service) Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error) {
	var current *PlanBlockResponse
	var effStart, effEnd time.Time
	timesProvided := false

	if req.StartedAt != nil && req.EndedAt != nil {
		if !req.EndedAt.After(*req.StartedAt) {
			return PlanBlockResponse{}, ErrInvalidTimeRange
		}
		effStart, effEnd, timesProvided = *req.StartedAt, *req.EndedAt, true
	} else if req.StartedAt != nil || req.EndedAt != nil {
		// Only one side provided — verify against the persisted other side.
		c, err := s.repo.Get(ctx, req.ID)
		if err != nil {
			return PlanBlockResponse{}, err
		}
		current = &c
		effStart, effEnd = current.StartedAt, current.EndedAt
		if req.StartedAt != nil {
			effStart = *req.StartedAt
		}
		if req.EndedAt != nil {
			effEnd = *req.EndedAt
		}
		if !effEnd.After(effStart) {
			return PlanBlockResponse{}, ErrInvalidTimeRange
		}
		timesProvided = true
	}

	if req.Label != nil {
		trimmed := strings.TrimSpace(*req.Label)
		if trimmed == "" {
			return PlanBlockResponse{}, ErrLabelRequired
		}
		if len(trimmed) > 200 {
			return PlanBlockResponse{}, ErrLabelTooLong
		}
		req.Label = &trimmed
	}

	if timesProvided {
		localEffStart := effStart.In(s.location)
		planDate := time.Date(localEffStart.Year(), localEffStart.Month(), localEffStart.Day(), 0, 0, 0, 0, s.location)
		excludeID := req.ID
		overlap, err := s.repo.HasOverlap(ctx, effStart, effEnd, &excludeID)
		if err != nil {
			return PlanBlockResponse{}, err
		}
		if overlap {
			return PlanBlockResponse{}, ErrOverlap
		}

		if current == nil {
			c, err := s.repo.Get(ctx, req.ID)
			if err != nil {
				return PlanBlockResponse{}, err
			}
			current = &c
		}
		// Moving a commitment-generated block to a different day: detach it from the
		// commitment and remember the original date, or the next generation pass recreates a
		// block there and the moved one lives on alongside it.
		if current.CommitmentID != nil && planDate.Format("2006-01-02") != current.PlanDate.Format("2006-01-02") {
			req.ClearCommitmentID = true
			if err := s.repo.InsertCommitmentSkip(ctx, *current.CommitmentID, current.PlanDate); err != nil {
				return PlanBlockResponse{}, err
			}
		}
	}

	return s.repo.Update(ctx, req)
}

func (s *Service) Delete(ctx context.Context, id int32) error {
	block, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if block.CommitmentID != nil {
		if err := s.repo.InsertCommitmentSkip(ctx, *block.CommitmentID, block.PlanDate); err != nil {
			return err
		}
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) DeleteFuture(ctx context.Context) error {
	return s.repo.DeleteEndingAfter(ctx, time.Now())
}

// SyncEventTime keeps the plan block linked to a calendar event (if any) in step with it.
// A direct time update, not Service.Update: this is a passive follow, not a user edit, and
// should not re-run overlap validation or label resolution.
func (s *Service) SyncEventTime(ctx context.Context, eventRef string, startedAt, endedAt time.Time) error {
	block, err := s.repo.GetByEventRef(ctx, eventRef)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.UpdateTimes(ctx, block.ID, startedAt, endedAt)
}

// DetachEvent unlinks the plan block from an event that changed identity (restructured
// recurrence, moved to another Google account) or was deleted. The block itself is kept — it
// becomes a normal, unlinked block.
func (s *Service) DetachEvent(ctx context.Context, eventRef string) error {
	block, err := s.repo.GetByEventRef(ctx, eventRef)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.ClearEventRef(ctx, block.ID)
}

func (s *Service) ListCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error) {
	return s.repo.ListCommitments(ctx)
}

func (s *Service) CreateCommitment(ctx context.Context, req CreateCommitmentRequest) (RecurringCommitmentResponse, error) {
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" {
		return RecurringCommitmentResponse{}, ErrLabelRequired
	}
	if len(req.DaysOfWeek) == 0 {
		return RecurringCommitmentResponse{}, ErrDaysOfWeekRequired
	}
	if _, err := s.repo.GetTaskName(ctx, req.TaskID); err != nil {
		return RecurringCommitmentResponse{}, err
	}
	return s.repo.CreateCommitment(ctx, req)
}

func (s *Service) UpdateCommitment(ctx context.Context, req UpdateCommitmentRequest) (RecurringCommitmentResponse, error) {
	if req.Label != nil {
		trimmed := strings.TrimSpace(*req.Label)
		if trimmed == "" {
			return RecurringCommitmentResponse{}, ErrLabelRequired
		}
		req.Label = &trimmed
	}
	if req.DaysOfWeek != nil && len(*req.DaysOfWeek) == 0 {
		return RecurringCommitmentResponse{}, ErrDaysOfWeekRequired
	}
	return s.repo.UpdateCommitment(ctx, req)
}

func (s *Service) DeleteCommitment(ctx context.Context, id int32) error {
	return s.repo.DeleteCommitment(ctx, id)
}

// resolveLabel produces the effective label for a block. If the caller
// supplied one, trim and validate it. Otherwise, fetch the linked task name.
// When neither is available, ErrLabelRequired.
func (s *Service) resolveLabel(ctx context.Context, taskID *int32, label *string) (string, error) {
	if label != nil {
		trimmed := strings.TrimSpace(*label)
		if trimmed != "" {
			if len(trimmed) > 200 {
				return "", ErrLabelTooLong
			}
			return trimmed, nil
		}
	}

	if taskID != nil {
		name, err := s.repo.GetTaskName(ctx, *taskID)
		if err != nil {
			return "", err
		}
		return name, nil
	}

	return "", ErrLabelRequired
}
