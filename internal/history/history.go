// Package history provides shared types and utilities for time-series history
// endpoints (habits, time entries) that aggregate data by frequency period.
package history

import (
	"fmt"
	"time"
)

// Point represents a single data point in a time-series history.
type Point struct {
	Date  string  `json:"date"`
	Value float32 `json:"value"`
}

// Response is the standard envelope for history endpoints.
type Response struct {
	StartAt string  `json:"start_at"`
	EndAt   string  `json:"end_at"`
	Data    []Point `json:"data"`
}

// FrequencyToTrunc maps API frequency names to PostgreSQL date_trunc names.
var FrequencyToTrunc = map[string]string{
	"daily":   "day",
	"weekly":  "week",
	"monthly": "month",
}

// ValidFrequency returns the trunc name for a frequency, or an error if invalid.
func ValidFrequency(frequency string) (string, error) {
	trunc, ok := FrequencyToTrunc[frequency]
	if !ok {
		return "", fmt.Errorf("invalid frequency: %s", frequency)
	}
	return trunc, nil
}

// DefaultStartDate returns a sensible default start date based on frequency.
func DefaultStartDate(today time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return today.AddDate(0, 0, -12*7)
	case "monthly":
		return today.AddDate(-1, 0, 0)
	default: // daily
		return today.AddDate(0, -1, 0)
	}
}

// PeriodStart returns the start date of the calendar period containing date.
func PeriodStart(date time.Time, frequency string) time.Time {
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

// PeriodCeil returns the start of the next period if date is not already on a period boundary.
func PeriodCeil(date time.Time, frequency string) time.Time {
	floor := PeriodStart(date, frequency)
	if floor.Equal(date) {
		return date
	}
	return NextPeriodStart(floor, frequency)
}

// PreviousPeriodStart returns the start of the period immediately before the given period start.
func PreviousPeriodStart(start time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return start.AddDate(0, 0, -7)
	case "monthly":
		return start.AddDate(0, -1, 0)
	default: // daily
		return start.AddDate(0, 0, -1)
	}
}

// NextPeriodStart returns the start of the period immediately after the given period start.
func NextPeriodStart(start time.Time, frequency string) time.Time {
	switch frequency {
	case "weekly":
		return start.AddDate(0, 0, 7)
	case "monthly":
		return start.AddDate(0, 1, 0)
	default: // daily
		return start.AddDate(0, 0, 1)
	}
}

// ParseDateRange parses start/end date strings and applies defaults based on frequency and location.
// Returns the snapped start and end dates.
func ParseDateRange(loc *time.Location, frequency, startAt, endAt string) (start, end time.Time, err error) {
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	end = today
	if endAt != "" {
		end, err = time.Parse("2006-01-02", endAt)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_at date")
		}
	}

	start = DefaultStartDate(today, frequency)
	if startAt != "" {
		start, err = time.Parse("2006-01-02", startAt)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start_at date")
		}
	}

	start = PeriodStart(start, frequency)
	end = PeriodCeil(end, frequency)

	return start, end, nil
}
