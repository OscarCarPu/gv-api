package capacity

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type planBusyProvider interface {
	BusyHoursByDate(ctx context.Context, from, to time.Time) (map[string]decimal.Decimal, error)
}

type Service struct {
	dailyCapacity decimal.Decimal
	plan          planBusyProvider
}

func NewService(dailyCapacity decimal.Decimal, plan planBusyProvider) *Service {
	return &Service{dailyCapacity: dailyCapacity, plan: plan}
}

func (s *Service) FreeBusyRange(ctx context.Context, from, to time.Time) ([]DayFreeBusy, error) {
	busy, err := s.plan.BusyHoursByDate(ctx, from, to)
	if err != nil {
		return nil, err
	}

	days := make([]DayFreeBusy, 0, int(to.Sub(from).Hours()/24))
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		b := busy[dateStr] // decimal.Zero if not present
		free := s.dailyCapacity.Sub(b)
		if free.IsNegative() {
			free = decimal.Zero
		}
		days = append(days, DayFreeBusy{Date: dateStr, CapacityHours: s.dailyCapacity, BusyHours: b, FreeHours: free})

	}
	return days, nil
}
