package tasks_test

import (
	"testing"
	"time"

	"gv-api/internal/tasks"

	"github.com/stretchr/testify/assert"
)

func TestCalcPace_GoalReached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC) // Monday
	pb := tasks.CalcPace(now, tasks.WeeklyTaskTargetSeconds, 0)

	assert.True(t, pb.GoalReached)
	assert.Zero(t, pb.WeightedTodayShareSeconds)
	assert.Zero(t, pb.UniformPerDaySeconds)
}

func TestCalcPace_GoalOverreached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, tasks.WeeklyTaskTargetSeconds+50000, 0)

	assert.True(t, pb.GoalReached)
}

func TestCalcPace_MondayMorning(t *testing.T) {
	// 2026-05-04 is a Monday. Today counts as a full waking day (17h).
	now := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 0, 0)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 6, pb.RemainingFullDays)
	// weightedTotal = 17 (today) + 17*4 (Tue–Fri) + 12*2 (Sat,Sun) = 109
	// weightedTodayShare = 288000 * 17 / 109 ≈ 44917
	assert.InDelta(t, 44917, pb.WeightedTodayShareSeconds, 2)
	assert.InDelta(t, 44917, pb.WeightedWeekdaySeconds, 2)
	assert.InDelta(t, 31706, pb.WeightedWeekendSeconds, 2)
	// uniformTotalDays = 1 + 6 = 7 ; uniformPerDay = 288000 / 7 ≈ 41142
	assert.InDelta(t, 41142, pb.UniformPerDaySeconds, 2)
	assert.InDelta(t, 41142, pb.UniformTodayShareSeconds, 2)
}

func TestCalcPace_StableThroughoutDay(t *testing.T) {
	// The daily target must not move as the clock advances within a single day,
	// provided previous-days' totals (here: 0) don't change.
	weekSeconds, todaySeconds := int64(0), int64(0)
	morning := tasks.CalcPace(time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC), weekSeconds, todaySeconds)
	noon := tasks.CalcPace(time.Date(2026, 5, 4, 14, 0, 0, 0, time.UTC), weekSeconds, todaySeconds)
	evening := tasks.CalcPace(time.Date(2026, 5, 4, 22, 0, 0, 0, time.UTC), weekSeconds, todaySeconds)

	assert.Equal(t, morning.WeightedTodayShareSeconds, noon.WeightedTodayShareSeconds)
	assert.Equal(t, morning.WeightedTodayShareSeconds, evening.WeightedTodayShareSeconds)
	assert.Equal(t, morning.UniformPerDaySeconds, evening.UniformPerDaySeconds)
}

func TestCalcPace_DailyTargetIgnoresTodaySeconds(t *testing.T) {
	// Logging time today must not lower today's goal — only past days do.
	now := time.Date(2026, 5, 4, 14, 0, 0, 0, time.UTC) // Monday
	zero := tasks.CalcPace(now, 0, 0)
	withToday := tasks.CalcPace(now, 20000, 20000) // 20k all from today

	assert.Equal(t, zero.WeightedTodayShareSeconds, withToday.WeightedTodayShareSeconds)
}

func TestCalcPace_SaturdayLateEvening(t *testing.T) {
	// 2026-05-09 Saturday, 22:00. Today still counted as a full 12h day.
	now := time.Date(2026, 5, 9, 22, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 200000, 0)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 1, pb.RemainingFullDays)
	// remaining = 88000
	// weightedTotal = 12 (Sat) + 12 (Sun) = 24
	// weightedTodayShare = 88000 * 12 / 24 = 44000
	assert.InDelta(t, 44000, pb.WeightedTodayShareSeconds, 2)
	assert.InDelta(t, 44000, pb.WeightedWeekendSeconds, 2)
	assert.InDelta(t, 62333, pb.WeightedWeekdaySeconds, 2)
}

func TestCalcPace_SundayLastHour(t *testing.T) {
	// 2026-05-10 Sunday, 23:00. No full days left, today is the only slot.
	now := time.Date(2026, 5, 10, 23, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 200000, 0)

	assert.False(t, pb.GoalReached)
	assert.Equal(t, 0, pb.RemainingFullDays)
	// remaining = 88000, weightedTotal = 12 (today only) → all remaining lands on today
	assert.Equal(t, int64(88000), pb.WeightedTodayShareSeconds)
}

func TestCalcDailyTargetSeconds_GoalReached(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	got := tasks.CalcDailyTargetSeconds(now, tasks.WeeklyTaskTargetSeconds, 0)
	assert.Zero(t, got)
}

func TestCalcDailyTargetSeconds_MatchesWeightedTodayShare(t *testing.T) {
	now := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	pb := tasks.CalcPace(now, 0, 0)
	got := tasks.CalcDailyTargetSeconds(now, 0, 0)
	assert.Equal(t, pb.WeightedTodayShareSeconds, got)
}
