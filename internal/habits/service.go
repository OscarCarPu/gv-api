package habits

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error) {
	targetDate := time.Now()

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
	return s.repo.UpsertLog(ctx, req.HabitID, date, req.Value)
}

func (s *Service) CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error) {
	return s.repo.CreateHabit(ctx, req.Name, req.Description)
}
