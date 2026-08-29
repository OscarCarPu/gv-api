package plan

import (
	"context"
	"time"
)

// EnsureRecurringBlocks materializes, for each active commitment, a real plan_block in
// [from, to) for every date whose weekday is in days_of_week — unless a row already exists for
// that (commitment, date) or the date is in recurring_commitment_skips. Idempotent: safe (and
// meant) to call on every range read, no background worker needed.
func (s *Service) EnsureRecurringBlocks(ctx context.Context, from, to time.Time) error {
	commitments, err := s.repo.ListActiveCommitments(ctx)
	if err != nil {
		return err
	}

	for _, c := range commitments {
		existing, err := s.repo.ListPlanBlockDatesByCommitment(ctx, c.ID, from, to)
		if err != nil {
			return err
		}
		skips, err := s.repo.ListCommitmentSkips(ctx, c.ID, from, to)
		if err != nil {
			return err
		}

		for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
			if !containsWeekday(c.DaysOfWeek, d.Weekday()) {
				continue
			}
			dateStr := d.Format("2006-01-02")
			if existing[dateStr] || skips[dateStr] {
				continue
			}

			started, err := combineDateTime(d, c.StartTime, s.location)
			if err != nil {
				return err
			}
			ended, err := combineDateTime(d, c.EndTime, s.location)
			if err != nil {
				return err
			}

			taskID := c.TaskID
			commitmentID := c.ID
			if _, err := s.repo.Create(ctx, CreatePlanBlockParams{
				PlanDate:     d,
				StartedAt:    started,
				EndedAt:      ended,
				TaskID:       &taskID,
				Label:        c.Label,
				CommitmentID: &commitmentID,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func containsWeekday(days []int32, wd time.Weekday) bool {
	for _, d := range days {
		if int32(wd) == d {
			return true
		}
	}
	return false
}

func combineDateTime(d time.Time, hhmm string, loc *time.Location) (time.Time, error) {
	t, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(d.Year(), d.Month(), d.Day(), t.Hour(), t.Minute(), 0, 0, loc), nil
}
