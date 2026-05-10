package plan

import (
	"context"
	"errors"
	"strings"
	"time"

	"gv-api/internal/tasks"
)

var (
	ErrInvalidTimeRange = errors.New("ended_at must be after started_at")
	ErrLabelRequired    = errors.New("label or task_id is required")
	ErrLabelTooLong     = errors.New("label must be at most 200 characters")
	ErrOverlap          = errors.New("plan block overlaps with an existing one")
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

func (s *Service) Create(ctx context.Context, req CreatePlanBlockRequest) (PlanBlockResponse, error) {
	if !req.EndedAt.After(req.StartedAt) {
		return PlanBlockResponse{}, ErrInvalidTimeRange
	}

	label, err := s.resolveLabel(ctx, req.TaskID, req.Label)
	if err != nil {
		return PlanBlockResponse{}, err
	}

	planDate := time.Date(req.StartedAt.Year(), req.StartedAt.Month(), req.StartedAt.Day(), 0, 0, 0, 0, time.UTC)

	overlap, err := s.repo.HasOverlap(ctx, planDate, req.StartedAt, req.EndedAt, nil)
	if err != nil {
		return PlanBlockResponse{}, err
	}
	if overlap {
		return PlanBlockResponse{}, ErrOverlap
	}

	return s.repo.Create(ctx, planDate, req.StartedAt, req.EndedAt, req.TaskID, label, req.Note)
}

func (s *Service) Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error) {
	var effStart, effEnd time.Time
	timesProvided := false

	if req.StartedAt != nil && req.EndedAt != nil {
		if !req.EndedAt.After(*req.StartedAt) {
			return PlanBlockResponse{}, ErrInvalidTimeRange
		}
		effStart, effEnd, timesProvided = *req.StartedAt, *req.EndedAt, true
	} else if req.StartedAt != nil || req.EndedAt != nil {
		// Only one side provided — verify against the persisted other side.
		current, err := s.repo.Get(ctx, req.ID)
		if err != nil {
			return PlanBlockResponse{}, err
		}
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
		planDate := time.Date(effStart.Year(), effStart.Month(), effStart.Day(), 0, 0, 0, 0, time.UTC)
		excludeID := req.ID
		overlap, err := s.repo.HasOverlap(ctx, planDate, effStart, effEnd, &excludeID)
		if err != nil {
			return PlanBlockResponse{}, err
		}
		if overlap {
			return PlanBlockResponse{}, ErrOverlap
		}
	}

	return s.repo.Update(ctx, req)
}

func (s *Service) Delete(ctx context.Context, id int32) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) DeleteFuture(ctx context.Context) error {
	return s.repo.DeleteEndingAfter(ctx, time.Now())
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
