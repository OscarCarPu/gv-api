package habits

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	repo     Repository
	location *time.Location
}

func NewService(repo Repository, loc *time.Location) *Service {
	if loc == nil {
		loc = time.UTC
	}
	return &Service{repo: repo, location: loc}
}

func (s *Service) GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
	targetDate := time.Now().In(s.location)

	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, err
		}
		targetDate = parsed
	}

	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)

	return s.repo.GetHabitsWithLogs(ctx, targetDate)
}

func (s *Service) LogHabit(ctx context.Context, req LogUpsertRequest) error {
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return err
	}
	if err := s.repo.UpsertLog(ctx, req.HabitID, date, req.Value); err != nil {
		return err
	}
	return s.recalculateStreak(ctx, req.HabitID)
}

func (s *Service) CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
	frequency := "daily"
	if req.Frequency != nil && *req.Frequency != "" {
		frequency = *req.Frequency
	}
	if frequency != "daily" && frequency != "weekly" && frequency != "monthly" {
		return CreateHabitResponse{}, fmt.Errorf("invalid frequency: %s", frequency)
	}
	return s.repo.CreateHabit(ctx, req.Name, req.Description, frequency, req.Objective)
}

func (s *Service) DeleteHabit(ctx context.Context, id int32) error {
	return s.repo.DeleteHabit(ctx, id)
}

func (s *Service) recalculateStreak(ctx context.Context, habitID int32) error {
	habit, err := s.repo.GetHabitByID(ctx, habitID)
	if err != nil {
		return err
	}

	// No objective means no streak tracking
	if habit.Objective == nil {
		return nil
	}
	objective := *habit.Objective

	logs, err := s.repo.GetHabitLogs(ctx, habitID)
	if err != nil {
		return err
	}

	periodSums := groupLogsByPeriod(logs, habit.Frequency)

	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	currentPeriod := periodStart(today, habit.Frequency)

	// Walk backwards counting consecutive periods that meet the objective
	currentStreak := int32(0)
	period := currentPeriod
	for {
		sum := periodSums[period]
		if sum >= objective {
			currentStreak++
			period = previousPeriodStart(period, habit.Frequency)
			continue
		}
		// Current period not yet met — skip it, don't break
		if period.Equal(currentPeriod) {
			period = previousPeriodStart(period, habit.Frequency)
			continue
		}
		// Past period not met — streak is broken
		break
	}

	longestStreak := habit.LongestStreak
	if currentStreak > longestStreak {
		longestStreak = currentStreak
	}

	return s.repo.UpdateHabitStreak(ctx, habitID, currentStreak, longestStreak)
}
