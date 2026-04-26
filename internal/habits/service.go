package habits

import (
	"context"
	"fmt"
	"time"

	"gv-api/internal/history"
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

	recordingRequired := true
	if req.RecordingRequired != nil {
		recordingRequired = *req.RecordingRequired
	}

	return s.repo.CreateHabit(ctx, req.Name, req.Description, frequency, req.TargetMin, req.TargetMax, recordingRequired)
}

func (s *Service) DeleteHabit(ctx context.Context, id int32) error {
	return s.repo.DeleteHabit(ctx, id)
}

// frequencyRank orders frequencies from finest to coarsest.
var frequencyRank = map[string]int{
	"daily":   0,
	"weekly":  1,
	"monthly": 2,
}

func (s *Service) GetHistory(ctx context.Context, habitID int32, frequency, startAt, endAt string) (HistoryResponse, error) {
	habit, err := s.repo.GetHabitByID(ctx, habitID)
	if err != nil {
		return HistoryResponse{}, err
	}

	if frequency == "" {
		frequency = habit.Frequency
	}

	trunc, err := history.ValidFrequency(frequency)
	if err != nil {
		return HistoryResponse{}, err
	}

	start, end, err := history.ParseDateRange(s.location, frequency, startAt, endAt)
	if err != nil {
		return HistoryResponse{}, err
	}

	// Use AVG when viewing at a coarser frequency than the habit's native one.
	var data []HistoryPoint
	if frequencyRank[frequency] > frequencyRank[habit.Frequency] {
		data, err = s.repo.GetHabitHistoryAvg(ctx, habitID, trunc, start, end, habit.RecordingRequired)
	} else {
		data, err = s.repo.GetHabitHistory(ctx, habitID, trunc, start, end, habit.RecordingRequired)
	}
	if err != nil {
		return HistoryResponse{}, err
	}

	return HistoryResponse{
		StartAt: start.Format("2006-01-02"),
		EndAt:   end.Format("2006-01-02"),
		Data:    data,
	}, nil
}

func (s *Service) recalculateStreak(ctx context.Context, habitID int32) error {
	habit, err := s.repo.GetHabitByID(ctx, habitID)
	if err != nil {
		return err
	}

	if habit.TargetMin == nil && habit.TargetMax == nil {
		return nil
	}

	now := time.Now().In(s.location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	return s.repo.RecalculateStreak(ctx, habitID, today)
}
