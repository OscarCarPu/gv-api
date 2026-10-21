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
	location      *time.Location
	clock         func() time.Time
}

func NewService(dailyCapacity decimal.Decimal, plan planBusyProvider) *Service {
	return &Service{dailyCapacity: dailyCapacity, plan: plan, location: time.UTC, clock: time.Now}
}

// SetToday tells the service which zone "today" is in and what time it is, so today's free
// hours can shrink as the day goes.
func (s *Service) SetToday(loc *time.Location, clock func() time.Time) {
	s.location, s.clock = loc, clock
}

// FreeBusyRange returns one entry per day in [from, to). An empty or reversed range yields an
// empty result, not an error: a past due date is an ordinary input.
func (s *Service) FreeBusyRange(ctx context.Context, from, to time.Time) ([]DayFreeBusy, error) {
	if !to.After(from) {
		return []DayFreeBusy{}, nil
	}

	busy, err := s.plan.BusyHoursByDate(ctx, from, to)
	if err != nil {
		return nil, err
	}

	now := s.clock().In(s.location)
	todayStr := now.Format("2006-01-02")
	endOfToday := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, s.location)
	leftToday := decimal.NewFromFloat(endOfToday.Sub(now).Hours())

	days := make([]DayFreeBusy, 0, int(to.Sub(from).Hours()/24))
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		b := busy[dateStr] // decimal.Zero if not present
		free := s.dailyCapacity.Sub(b)
		if dateStr == todayStr {
			free = decimal.Min(free, leftToday)
		}
		if free.IsNegative() {
			free = decimal.Zero
		}
		days = append(days, DayFreeBusy{Date: dateStr, CapacityHours: s.dailyCapacity, BusyHours: b, FreeHours: free})

	}
	return days, nil
}
