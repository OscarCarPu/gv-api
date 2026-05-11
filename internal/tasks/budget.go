package tasks

import "time"

const (
	WeeklyTaskTargetSeconds int64 = 288000

	WeekdayWakingHours = 17
	WeekendWakingHours = 12
)

func wakingHours(d time.Weekday) int {
	if d == time.Saturday || d == time.Sunday {
		return WeekendWakingHours
	}
	return WeekdayWakingHours
}

type PaceBreakdown struct {
	UniformPerDaySeconds      int64 `json:"uniform_per_day_seconds"`
	UniformTodayShareSeconds  int64 `json:"uniform_today_share_seconds"`
	WeightedWeekdaySeconds    int64 `json:"weighted_weekday_seconds"`
	WeightedWeekendSeconds    int64 `json:"weighted_weekend_seconds"`
	WeightedTodayShareSeconds int64 `json:"weighted_today_share_seconds"`
	RemainingFullDays         int   `json:"remaining_full_days"`
	GoalReached               bool  `json:"goal_reached"`
}

func CalcDailyTargetSeconds(now time.Time, weekSeconds, todaySeconds int64) int64 {
	pace := CalcPace(now, weekSeconds, todaySeconds)
	if pace.GoalReached {
		return 0
	}
	return pace.WeightedTodayShareSeconds
}

// CalcPace computes the day's goal as a full-day allocation. The daily target
// depends only on prior days' logged time and today's full waking hours — not
// on the current clock or on seconds already logged today — so it represents
// "the day's goal", not "what's left to do today".
func CalcPace(now time.Time, weekSeconds, todaySeconds int64) PaceBreakdown {
	if weekSeconds >= WeeklyTaskTargetSeconds {
		return PaceBreakdown{GoalReached: true}
	}

	previousDaysSeconds := weekSeconds - todaySeconds
	if previousDaysSeconds < 0 {
		previousDaysSeconds = 0
	}
	remaining := WeeklyTaskTargetSeconds - previousDaysSeconds

	weekday := now.Weekday()
	isoDay := (int(weekday)+6)%7 + 1
	remainingFullDays := 7 - isoDay

	if remaining <= 0 {
		return PaceBreakdown{RemainingFullDays: remainingFullDays}
	}

	todayShare := float64(wakingHours(weekday))
	uniformTotalDays := 1.0 + float64(remainingFullDays)

	weightedTotal := todayShare
	for d := isoDay + 1; d <= 7; d++ {
		var wd time.Weekday
		if d == 7 {
			wd = time.Sunday
		} else {
			wd = time.Weekday(d)
		}
		weightedTotal += float64(wakingHours(wd))
	}

	pb := PaceBreakdown{RemainingFullDays: remainingFullDays}

	uniformPerDay := float64(remaining) / uniformTotalDays
	pb.UniformPerDaySeconds = int64(uniformPerDay)
	pb.UniformTodayShareSeconds = int64(uniformPerDay)

	remF := float64(remaining)
	pb.WeightedTodayShareSeconds = int64(remF * todayShare / weightedTotal)
	pb.WeightedWeekdaySeconds = int64(remF * float64(WeekdayWakingHours) / weightedTotal)
	pb.WeightedWeekendSeconds = int64(remF * float64(WeekendWakingHours) / weightedTotal)

	return pb
}
