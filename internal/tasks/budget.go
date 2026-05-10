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

func CalcDailyTargetSeconds(now time.Time, weekSeconds int64) int64 {
	pace := CalcPace(now, weekSeconds)
	if pace.GoalReached {
		return 0
	}
	return pace.WeightedTodayShareSeconds
}

func CalcPace(now time.Time, weekSeconds int64) PaceBreakdown {
	remaining := WeeklyTaskTargetSeconds - weekSeconds
	if remaining <= 0 {
		return PaceBreakdown{GoalReached: true}
	}

	weekday := now.Weekday()
	isoDay := (int(weekday)+6)%7 + 1

	currentHour := float64(now.Hour()) + float64(now.Minute())/60
	wakingLeft := 24 - currentHour
	if wakingLeft < 0 {
		wakingLeft = 0
	}

	todayWakingFull := float64(wakingHours(weekday))
	todayShare := todayWakingFull
	if wakingLeft < todayShare {
		todayShare = wakingLeft
	}

	uniformWaking := float64(WeekdayWakingHours)
	uniformTodayRaw := wakingLeft
	if uniformTodayRaw > uniformWaking {
		uniformTodayRaw = uniformWaking
	}
	uniformTodayFraction := uniformTodayRaw / uniformWaking

	remainingFullDays := 7 - isoDay
	uniformTotalDays := uniformTodayFraction + float64(remainingFullDays)

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

	if uniformTotalDays > 0 {
		uniformPerDay := float64(remaining) / uniformTotalDays
		pb.UniformPerDaySeconds = int64(uniformPerDay)
		pb.UniformTodayShareSeconds = int64(uniformPerDay * uniformTodayFraction)
	}

	if weightedTotal > 0 {
		remF := float64(remaining)
		pb.WeightedTodayShareSeconds = int64(remF * todayShare / weightedTotal)
		pb.WeightedWeekdaySeconds = int64(remF * float64(WeekdayWakingHours) / weightedTotal)
		pb.WeightedWeekendSeconds = int64(remF * float64(WeekendWakingHours) / weightedTotal)
	}

	return pb
}
