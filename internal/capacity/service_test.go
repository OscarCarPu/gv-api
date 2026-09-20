package capacity_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gv-api/internal/capacity"
)

type stubBusy struct {
	busy  map[string]decimal.Decimal
	calls int
}

func (s *stubBusy) BusyHoursByDate(_ context.Context, _, _ time.Time) (map[string]decimal.Decimal, error) {
	s.calls++
	return s.busy, nil
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestService_FreeBusyRange(t *testing.T) {
	daily := decimal.RequireFromString("8")

	t.Run("one entry per day in [from, to)", func(t *testing.T) {
		svc := capacity.NewService(daily, &stubBusy{busy: map[string]decimal.Decimal{
			"2026-09-21": decimal.RequireFromString("3"),
		}})
		got, err := svc.FreeBusyRange(context.Background(), day(2026, 9, 20), day(2026, 9, 23))
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, "2026-09-20", got[0].Date)
		assert.True(t, got[0].FreeHours.Equal(daily))
		assert.True(t, got[1].FreeHours.Equal(decimal.RequireFromString("5")), "busy hours come off capacity")
	})

	t.Run("busy hours beyond capacity floor free time at zero", func(t *testing.T) {
		svc := capacity.NewService(daily, &stubBusy{busy: map[string]decimal.Decimal{
			"2026-09-20": decimal.RequireFromString("12"),
		}})
		got, err := svc.FreeBusyRange(context.Background(), day(2026, 9, 20), day(2026, 9, 21))
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].FreeHours.IsZero())
	})

	// A task's due date is a legitimate source of `to`, and a due date in the past puts it before
	// `from`. That used to size a slice with a negative capacity and panic the request.
	t.Run("a reversed or empty range has no days and does not panic", func(t *testing.T) {
		busy := &stubBusy{}
		svc := capacity.NewService(daily, busy)
		for name, to := range map[string]time.Time{
			"before from": day(2026, 9, 1),
			"equal":       day(2026, 9, 20),
		} {
			t.Run(name, func(t *testing.T) {
				var got []capacity.DayFreeBusy
				var err error
				require.NotPanics(t, func() {
					got, err = svc.FreeBusyRange(context.Background(), day(2026, 9, 20), to)
				})
				require.NoError(t, err)
				assert.NotNil(t, got, "an empty range is an empty list, not null, on the wire")
				assert.Empty(t, got)
			})
		}
		assert.Zero(t, busy.calls, "no point asking the plan about a range with no days")
	})
}
