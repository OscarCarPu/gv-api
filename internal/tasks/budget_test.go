package tasks_test

import (
	"testing"
	"time"

	"gv-api/internal/tasks"

	"github.com/stretchr/testify/assert"
)

func TestCalcPace_GoalReached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC) // Monday
	pb := tasks.CalcPace(now, tasks.WeeklyTaskTargetSeconds)

	assert.True(t, pb.GoalReached)
	assert.Zero(t, pb.WeightedTodayShareSeconds)
	assert.Zero(t, pb.UniformPerDaySeconds)
}

func TestCalcPace_GoalOverreached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, tasks.WeeklyTaskTargetSeconds+50000)

	assert.True(t, pb.GoalReached)
}

func TestCalcPace_MondayMorning(t *testing.T) {
	// 2026-05-04 is a Monday. 08:00 → 16h waking remaining.
	now := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 0)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 6, pb.RemainingFullDays)
	// weightedTotal = 16 (today) + 17*4 (Tue–Fri) + 12*2 (Sat,Sun) = 108
	// weightedTodayShare = 288000 * 16 / 108 = 42666
	assert.InDelta(t, 42666, pb.WeightedTodayShareSeconds, 2)
	assert.InDelta(t, 45333, pb.WeightedWeekdaySeconds, 2)
	assert.InDelta(t, 32000, pb.WeightedWeekendSeconds, 2)
	// uniformTotalDays = 16/17 + 6 ≈ 6.9412
	// uniformPerDay = 288000 / 6.9412 ≈ 41492
	assert.InDelta(t, 41492, pb.UniformPerDaySeconds, 2)
}

func TestCalcPace_SaturdayLateEvening(t *testing.T) {
	// 2026-05-09 Saturday, 22:00 → 2h waking remaining.
	now := time.Date(2026, 5, 9, 22, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 200000)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 1, pb.RemainingFullDays)
	// remaining = 88000
	// weightedTotal = 2 (today, capped) + 12 (Sun) = 14
	// weightedTodayShare = 88000 * 2 / 14 = 12571
	assert.InDelta(t, 12571, pb.WeightedTodayShareSeconds, 2)
	assert.InDelta(t, 75428, pb.WeightedWeekendSeconds, 2)
	assert.InDelta(t, 106857, pb.WeightedWeekdaySeconds, 2)
}

func TestCalcPace_SundayLastHour(t *testing.T) {
	// 2026-05-10 Sunday, 23:00 → 1h waking remaining, no full days left.
	now := time.Date(2026, 5, 10, 23, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 200000)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 0, pb.RemainingFullDays)
	// remaining = 88000, weightedTotal = 1
	// weightedTodayShare = 88000
	assert.Equal(t, int64(88000), pb.WeightedTodayShareSeconds)
}

func TestCalcDailyTargetSeconds_GoalReached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	got := tasks.CalcDailyTargetSeconds(now, tasks.WeeklyTaskTargetSeconds)
	assert.Zero(t, got)
}

func TestCalcDailyTargetSeconds_MatchesWeightedTodayShare(t *testing.T) {
	now := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 0)
	got := tasks.CalcDailyTargetSeconds(now, 0)
	assert.Equal(t, pb.WeightedTodayShareSeconds, got)
}
