package calendar

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// planBlockSyncer keeps a plan_block linked to an event in step with it. Both methods are
// no-ops if nothing links to that event_ref, which is the event id as a string.
type planBlockSyncer interface {
	SyncEventTime(ctx context.Context, eventRef string, startsAt, endsAt time.Time) error
	DetachEvent(ctx context.Context, eventRef string) error
}

type Service struct {
	repo     Repository
	planSync planBlockSyncer
}

func NewService(repo Repository, planSync planBlockSyncer) *Service {
	return &Service{repo: repo, planSync: planSync}
}

// maxRangeSpan bounds a single listing.
const maxRangeSpan = 2 * 366 * 24 * time.Hour

func (s *Service) ListEvents(ctx context.Context, from, to time.Time) ([]Event, error) {
	if !to.After(from) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalid)
	}
	if to.Sub(from) > maxRangeSpan {
		return nil, fmt.Errorf("%w: range is longer than two years", ErrInvalid)
	}
	return s.repo.List(ctx, from, to)
}

func (s *Service) GetEvent(ctx context.Context, id int32) (Event, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) CreateEvent(ctx context.Context, req CreateEventRequest) (Event, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Event{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if req.StartTime.IsZero() || req.EndTime.IsZero() || !req.EndTime.After(req.StartTime) {
		return Event{}, fmt.Errorf("%w: end_time must be after start_time", ErrInvalid)
	}
	return s.repo.Create(ctx, name, req.StartTime, req.EndTime)
}

func (s *Service) UpdateEvent(ctx context.Context, id int32, req UpdateEventRequest) (Event, error) {
	if req.Name == nil && req.StartTime == nil && req.EndTime == nil {
		return Event{}, fmt.Errorf("%w: nothing to update", ErrInvalid)
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Event{}, err
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return Event{}, fmt.Errorf("%w: name cannot be empty", ErrInvalid)
		}
		req.Name = &trimmed
	}
	start, end := current.StartTime, current.EndTime
	if req.StartTime != nil {
		start = *req.StartTime
	}
	if req.EndTime != nil {
		end = *req.EndTime
	}
	if !end.After(start) {
		return Event{}, fmt.Errorf("%w: end_time must be after start_time", ErrInvalid)
	}

	updated, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return Event{}, err
	}
	if req.StartTime != nil || req.EndTime != nil {
		ref := strconv.Itoa(int(id))
		if err := s.planSync.SyncEventTime(ctx, ref, updated.StartTime, updated.EndTime); err != nil {
			slog.ErrorContext(ctx, "calendar: syncing linked plan block", "event", id, "error", err)
		}
	}
	return updated, nil
}

func (s *Service) DeleteEvent(ctx context.Context, id int32) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if err := s.planSync.DetachEvent(ctx, strconv.Itoa(int(id))); err != nil {
		slog.ErrorContext(ctx, "calendar: detaching plan block", "event", id, "error", err)
	}
	return nil
}
