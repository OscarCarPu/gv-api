package habits

import (
	"time"

	"gv-api/internal/database/habitsdb"
	"gv-api/internal/history"
)

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
		ps := history.PeriodStart(date, frequency)
		sums[ps] += log.Value
	}
	return sums
}
