package habits

import (
	"time"

	"gv-api/internal/database/habitsdb"
)

// periodStart returns the start date of the calendar period containing date.
func periodStart(date time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		weekday := date.Weekday()
		offset := (int(weekday) - int(time.Monday) + 7) % 7
		return date.AddDate(0, 0, -offset)
	case "monthly":
		return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
	default: // daily
		return date
	}
}

// previousPeriodStart returns the start of the period immediately before the given period start.
func previousPeriodStart(start time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return start.AddDate(0, 0, -7)
	case "monthly":
		return start.AddDate(0, -1, 0)
	default: // daily
		return start.AddDate(0, 0, -1)
	}
}

// nextPeriodStart returns the start of the period immediately after the given period start.
func nextPeriodStart(start time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return start.AddDate(0, 0, 7)
	case "monthly":
		return start.AddDate(0, 1, 0)
	default: // daily
		return start.AddDate(0, 0, 1)
	}
}

// meetsTarget returns true if value satisfies the target bounds.
func meetsTarget(value float32, targetMin, targetMax *float32) bool {
	if targetMin != nil && value < *targetMin {
		return false
	}
	if targetMax != nil && value > *targetMax {
		return false
	}
	return targetMin != nil || targetMax != nil
}

// groupLogsByPeriod sums log values by their period start date.
func groupLogsByPeriod(logs []habitsdb.HabitLog, frequency string) map[time.Time]float32 {
	sums := make(map[time.Time]float32)
	for _, log := range logs {
		date := time.Date(log.LogDate.Year(), log.LogDate.Month(), log.LogDate.Day(), 0, 0, 0, 0, time.UTC)
		ps := periodStart(date, frequency)
		sums[ps] += log.Value
	}
	return sums
}
