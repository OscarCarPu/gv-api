package habits

import (
	"testing"
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/history"

	"github.com/stretchr/testify/assert"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestPeriodStart(t *testing.T) {
	tests := []struct {
		name      string
		date      time.Time
		frequency string
		want      time.Time
	}{
		{"daily", date(2026, 3, 17), "daily", date(2026, 3, 17)},
		{"weekly monday", date(2026, 3, 16), "weekly", date(2026, 3, 16)},             // Monday
		{"weekly wednesday", date(2026, 3, 18), "weekly", date(2026, 3, 16)},           // Wed -> Mon
		{"weekly sunday", date(2026, 3, 22), "weekly", date(2026, 3, 16)},              // Sun -> Mon
		{"monthly first", date(2026, 3, 1), "monthly", date(2026, 3, 1)},               // 1st
		{"monthly mid", date(2026, 3, 17), "monthly", date(2026, 3, 1)},                // 17th -> 1st
		{"monthly last", date(2026, 2, 28), "monthly", date(2026, 2, 1)},               // 28th -> 1st
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := history.PeriodStart(tc.date, tc.frequency)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPreviousPeriodStart(t *testing.T) {
	assert.Equal(t, date(2026, 3, 16), history.PreviousPeriodStart(date(2026, 3, 17), "daily"))
	assert.Equal(t, date(2026, 3, 9), history.PreviousPeriodStart(date(2026, 3, 16), "weekly"))
	assert.Equal(t, date(2026, 2, 1), history.PreviousPeriodStart(date(2026, 3, 1), "monthly"))
}

func TestGroupLogsByPeriod(t *testing.T) {
	logs := []habitsdb.HabitLog{
		{HabitID: 1, LogDate: date(2026, 3, 16), Value: 1},
		{HabitID: 1, LogDate: date(2026, 3, 17), Value: 2},
		{HabitID: 1, LogDate: date(2026, 3, 18), Value: 3},
	}

	t.Run("daily keeps separate", func(t *testing.T) {
		sums := groupLogsByPeriod(logs, "daily")
		assert.Equal(t, float32(1), sums[date(2026, 3, 16)])
		assert.Equal(t, float32(2), sums[date(2026, 3, 17)])
		assert.Equal(t, float32(3), sums[date(2026, 3, 18)])
	})

	t.Run("weekly groups into same week", func(t *testing.T) {
		sums := groupLogsByPeriod(logs, "weekly")
		// All three dates are in the same Mon-Sun week starting 2026-03-16
		assert.Equal(t, float32(6), sums[date(2026, 3, 16)])
		assert.Len(t, sums, 1)
	})

	t.Run("monthly groups into same month", func(t *testing.T) {
		sums := groupLogsByPeriod(logs, "monthly")
		assert.Equal(t, float32(6), sums[date(2026, 3, 1)])
		assert.Len(t, sums, 1)
	})
}
