package history_test

import (
	"testing"
	"time"

	"gv-api/internal/history"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestPeriodStart(t *testing.T) {
	t.Run("daily returns same date", func(t *testing.T) {
		date := d(2026, 3, 17)
		assert.Equal(t, date, history.PeriodStart(date, "daily"))
	})

	t.Run("weekly snaps to Monday", func(t *testing.T) {
		// 2026-03-19 is a Thursday
		thursday := d(2026, 3, 19)
		monday := d(2026, 3, 16)
		assert.Equal(t, monday, history.PeriodStart(thursday, "weekly"))
	})

	t.Run("weekly on Monday returns same date", func(t *testing.T) {
		monday := d(2026, 3, 16)
		assert.Equal(t, monday, history.PeriodStart(monday, "weekly"))
	})

	t.Run("weekly on Sunday snaps to previous Monday", func(t *testing.T) {
		sunday := d(2026, 3, 22)
		monday := d(2026, 3, 16)
		assert.Equal(t, monday, history.PeriodStart(sunday, "weekly"))
	})

	t.Run("monthly snaps to first of month", func(t *testing.T) {
		date := d(2026, 3, 17)
		assert.Equal(t, d(2026, 3, 1), history.PeriodStart(date, "monthly"))
	})

	t.Run("monthly on first returns same date", func(t *testing.T) {
		first := d(2026, 3, 1)
		assert.Equal(t, first, history.PeriodStart(first, "monthly"))
	})
}

func TestPeriodCeil(t *testing.T) {
	t.Run("boundary date returns unchanged", func(t *testing.T) {
		monday := d(2026, 3, 16)
		assert.Equal(t, monday, history.PeriodCeil(monday, "weekly"))
	})

	t.Run("daily boundary returns unchanged", func(t *testing.T) {
		date := d(2026, 3, 17)
		assert.Equal(t, date, history.PeriodCeil(date, "daily"))
	})

	t.Run("monthly boundary returns unchanged", func(t *testing.T) {
		first := d(2026, 3, 1)
		assert.Equal(t, first, history.PeriodCeil(first, "monthly"))
	})

	t.Run("non-boundary weekly snaps to next Monday", func(t *testing.T) {
		wednesday := d(2026, 3, 18)
		nextMonday := d(2026, 3, 23)
		assert.Equal(t, nextMonday, history.PeriodCeil(wednesday, "weekly"))
	})

	t.Run("non-boundary monthly snaps to next first", func(t *testing.T) {
		date := d(2026, 3, 15)
		assert.Equal(t, d(2026, 4, 1), history.PeriodCeil(date, "monthly"))
	})
}

func TestNextPeriodStart(t *testing.T) {
	t.Run("daily adds one day", func(t *testing.T) {
		date := d(2026, 3, 17)
		assert.Equal(t, d(2026, 3, 18), history.NextPeriodStart(date, "daily"))
	})

	t.Run("weekly adds seven days", func(t *testing.T) {
		monday := d(2026, 3, 16)
		assert.Equal(t, d(2026, 3, 23), history.NextPeriodStart(monday, "weekly"))
	})

	t.Run("monthly adds one month", func(t *testing.T) {
		first := d(2026, 3, 1)
		assert.Equal(t, d(2026, 4, 1), history.NextPeriodStart(first, "monthly"))
	})
}

func TestPreviousPeriodStart(t *testing.T) {
	t.Run("daily subtracts one day", func(t *testing.T) {
		date := d(2026, 3, 17)
		assert.Equal(t, d(2026, 3, 16), history.PreviousPeriodStart(date, "daily"))
	})

	t.Run("weekly subtracts seven days", func(t *testing.T) {
		monday := d(2026, 3, 16)
		assert.Equal(t, d(2026, 3, 9), history.PreviousPeriodStart(monday, "weekly"))
	})

	t.Run("monthly subtracts one month", func(t *testing.T) {
		first := d(2026, 3, 1)
		assert.Equal(t, d(2026, 2, 1), history.PreviousPeriodStart(first, "monthly"))
	})
}

func TestDefaultStartDate(t *testing.T) {
	today := d(2026, 3, 17)

	t.Run("daily returns one month ago", func(t *testing.T) {
		assert.Equal(t, d(2026, 2, 17), history.DefaultStartDate(today, "daily"))
	})

	t.Run("weekly returns 12 weeks ago", func(t *testing.T) {
		expected := today.AddDate(0, 0, -12*7)
		assert.Equal(t, expected, history.DefaultStartDate(today, "weekly"))
	})

	t.Run("monthly returns one year ago", func(t *testing.T) {
		assert.Equal(t, d(2025, 3, 17), history.DefaultStartDate(today, "monthly"))
	})
}

func TestValidFrequency(t *testing.T) {
	t.Run("daily returns day", func(t *testing.T) {
		trunc, err := history.ValidFrequency("daily")
		require.NoError(t, err)
		assert.Equal(t, "day", trunc)
	})

	t.Run("weekly returns week", func(t *testing.T) {
		trunc, err := history.ValidFrequency("weekly")
		require.NoError(t, err)
		assert.Equal(t, "week", trunc)
	})

	t.Run("monthly returns month", func(t *testing.T) {
		trunc, err := history.ValidFrequency("monthly")
		require.NoError(t, err)
		assert.Equal(t, "month", trunc)
	})

	t.Run("invalid returns error", func(t *testing.T) {
		_, err := history.ValidFrequency("yearly")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid frequency")
	})
}

func TestParseDateRange(t *testing.T) {
	loc := time.UTC

	t.Run("with explicit dates", func(t *testing.T) {
		start, end, err := history.ParseDateRange(loc, "daily", "2026-03-10", "2026-03-17")
		require.NoError(t, err)
		assert.Equal(t, d(2026, 3, 10), start)
		assert.Equal(t, d(2026, 3, 17), end)
	})

	t.Run("without explicit dates uses defaults", func(t *testing.T) {
		start, end, err := history.ParseDateRange(loc, "daily", "", "")
		require.NoError(t, err)
		assert.False(t, start.IsZero())
		assert.False(t, end.IsZero())
		assert.True(t, start.Before(end))
	})

	t.Run("invalid start_at returns error", func(t *testing.T) {
		_, _, err := history.ParseDateRange(loc, "daily", "bad", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_at")
	})

	t.Run("invalid end_at returns error", func(t *testing.T) {
		_, _, err := history.ParseDateRange(loc, "daily", "", "bad")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end_at")
	})

	t.Run("weekly dates snap to period boundaries", func(t *testing.T) {
		// 2026-03-18 is Wednesday; start should snap to Monday 2026-03-16
		start, end, err := history.ParseDateRange(loc, "weekly", "2026-03-18", "2026-03-18")
		require.NoError(t, err)
		assert.Equal(t, d(2026, 3, 16), start) // PeriodStart snaps to Monday
		assert.Equal(t, d(2026, 3, 23), end)   // PeriodCeil snaps to next Monday
	})
}
