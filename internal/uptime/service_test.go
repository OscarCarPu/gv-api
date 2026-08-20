package uptime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gv-api/internal/uptime"
	"gv-api/internal/uptime/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The service's job is everything the pipeline deliberately does not do: decide what
// "stale" means, keep both devices in the answer even when one is missing, and turn
// clipped seconds into a percentage over the part of the range that is actually covered.

const staleAfter = 2 * time.Hour

func newService(repo uptime.Repository) *uptime.Service {
	return uptime.NewService(repo, staleAfter)
}

func TestService_Overview_FreshRun(t *testing.T) {
	computedAt := time.Now().Add(-5 * time.Minute)
	since := time.Now().Add(-72 * time.Hour)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CurrentStates(mock.Anything).Return([]uptime.CurrentState{
		{Device: uptime.DeviceWatchdog, State: uptime.StateDown, Since: since},
		{Device: uptime.DeviceLab, State: uptime.StateUp, Since: since},
	}, nil)
	repo.EXPECT().Aggregations(mock.Anything).Return([]uptime.Aggregation{
		{Device: uptime.DeviceLab, Range: uptime.LookbackYear, Uptime: 98.05, RangeEnd: computedAt},
		{Device: uptime.DeviceLab, Range: uptime.LookbackMonth, Uptime: 98.92, RangeEnd: computedAt},
		{Device: uptime.DeviceWatchdog, Range: uptime.LookbackAll, Uptime: 98.04, RangeEnd: computedAt},
	}, nil)

	got, err := newService(repo).Overview(context.Background())
	require.NoError(t, err)

	require.NotNil(t, got.ComputedAt)
	assert.WithinDuration(t, computedAt, *got.ComputedAt, time.Second)
	assert.False(t, got.Stale)
	assert.Equal(t, int(staleAfter.Seconds()), got.StaleAfterSeconds)

	// Devices come back in a fixed order regardless of how the rows arrived, and each
	// device's ranges are ordered shortest lookback first.
	require.Len(t, got.Devices, 2)
	assert.Equal(t, uptime.DeviceLab, got.Devices[0].Device)
	assert.Equal(t, uptime.StateUp, got.Devices[0].State)
	require.NotNil(t, got.Devices[0].Since)
	assert.Equal(t, since, *got.Devices[0].Since)
	require.Len(t, got.Devices[0].Ranges, 2)
	assert.Equal(t, uptime.LookbackMonth, got.Devices[0].Ranges[0].Range)
	assert.Equal(t, uptime.LookbackYear, got.Devices[0].Ranges[1].Range)
	assert.InDelta(t, 98.92, got.Devices[0].Ranges[0].Uptime, 0.001)

	assert.Equal(t, uptime.DeviceWatchdog, got.Devices[1].Device)
	assert.Equal(t, uptime.StateDown, got.Devices[1].State)
	require.Len(t, got.Devices[1].Ranges, 1)
	assert.Equal(t, uptime.LookbackAll, got.Devices[1].Ranges[0].Range)
}

func TestService_Overview_StaleRun(t *testing.T) {
	old := time.Now().Add(-3 * time.Hour)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CurrentStates(mock.Anything).Return([]uptime.CurrentState{}, nil)
	repo.EXPECT().Aggregations(mock.Anything).Return([]uptime.Aggregation{
		{Device: uptime.DeviceLab, Range: uptime.LookbackMonth, Uptime: 99, RangeEnd: old},
	}, nil)

	got, err := newService(repo).Overview(context.Background())
	require.NoError(t, err)
	assert.True(t, got.Stale, "a run older than staleAfter is not current")
}

func TestService_Overview_UnknownDevice(t *testing.T) {
	// A device the pipeline has never heard from still appears: dropping it would read as
	// "there is no watchdog" rather than "nothing is known about it".
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CurrentStates(mock.Anything).Return([]uptime.CurrentState{
		{Device: uptime.DeviceLab, State: uptime.StateUp, Since: time.Now()},
	}, nil)
	repo.EXPECT().Aggregations(mock.Anything).Return([]uptime.Aggregation{}, nil)

	got, err := newService(repo).Overview(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Devices, 2)
	assert.Equal(t, uptime.StateUnknown, got.Devices[1].State)
	assert.Nil(t, got.Devices[1].Since)
	assert.Empty(t, got.Devices[1].Ranges)
	assert.Nil(t, got.ComputedAt)
	assert.True(t, got.Stale, "nothing computed yet cannot be fresh")
}

func TestService_Overview_NotConfigured(t *testing.T) {
	// With no pipeline database the repository reports it and the service passes it
	// through, so the handler can answer 503 rather than 500.
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CurrentStates(mock.Anything).Return(nil, uptime.ErrNotConfigured)

	_, err := newService(repo).Overview(context.Background())
	assert.ErrorIs(t, err, uptime.ErrNotConfigured)
}

func TestService_Overview_RepoError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CurrentStates(mock.Anything).Return(nil, errors.New("db down"))

	_, err := newService(repo).Overview(context.Background())
	assert.Error(t, err)
}

func TestService_Windows_DefaultsToLastThirtyDays(t *testing.T) {
	var gotFrom, gotTo time.Time
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, from, to time.Time, _ *uptime.Device) ([]uptime.RangeStat, error) {
			gotFrom, gotTo = from, to
			return []uptime.RangeStat{}, nil
		})
	repo.EXPECT().Windows(mock.Anything, mock.Anything, mock.Anything, mock.Anything, uptime.DefaultWindowLimit).
		Return([]uptime.WindowRow{}, nil)

	got, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{})
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now(), gotTo, 2*time.Second)
	assert.WithinDuration(t, gotTo.Add(-30*24*time.Hour), gotFrom, time.Second)
	assert.Equal(t, gotFrom, got.From)
	assert.Equal(t, gotTo, got.To)
	// No stats for either device: the range is served, both devices report nothing.
	require.Len(t, got.Devices, 2)
	assert.Nil(t, got.Devices[0].Uptime)
	assert.Empty(t, got.Devices[0].Windows)
}

func TestService_Windows_ClampsFutureTo(t *testing.T) {
	// The open window is counted up to `to`, so honouring a future `to` would invent
	// uptime that has not happened yet.
	var gotTo time.Time
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _, to time.Time, _ *uptime.Device) ([]uptime.RangeStat, error) {
			gotTo = to
			return []uptime.RangeStat{}, nil
		})
	repo.EXPECT().Windows(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]uptime.WindowRow{}, nil)

	_, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{
		From: time.Now().Add(-time.Hour),
		To:   time.Now().Add(48 * time.Hour),
	})
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), gotTo, 2*time.Second)
}

func TestService_Windows_RejectsBackwardsRange(t *testing.T) {
	now := time.Now()
	_, err := newService(mocks.NewMockRepository(t)).Windows(context.Background(), uptime.WindowsQuery{
		From: now,
		To:   now.Add(-time.Hour),
	})
	assert.ErrorIs(t, err, uptime.ErrInvalidRange)
}

func TestService_Windows_PercentageOverCoveredTime(t *testing.T) {
	// 9.5 hours up, 1 down, inside a 30-day range the device only partly covers: 90.48%,
	// not the 1.3% that dividing by the whole range would give. The odd split also pins the
	// rounding to two decimals that the precomputed rows use.
	to := time.Now()
	from := to.Add(-30 * 24 * time.Hour)
	coveredFrom, coveredTo := to.Add(-10*time.Hour), to

	device := uptime.DeviceLab
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, &device).Return([]uptime.RangeStat{{
		Device:      uptime.DeviceLab,
		UpSeconds:   9.5 * 3600,
		DownSeconds: 1 * 3600,
		Outages:     1,
		CoveredFrom: coveredFrom,
		CoveredTo:   coveredTo,
	}}, nil)
	repo.EXPECT().Windows(mock.Anything, mock.Anything, mock.Anything, &device, mock.Anything).
		Return([]uptime.WindowRow{}, nil)

	got, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{
		From: from, To: to, Device: &device,
	})
	require.NoError(t, err)

	// A device filter narrows the answer to that device alone.
	require.Len(t, got.Devices, 1)
	entry := got.Devices[0]
	require.NotNil(t, entry.Uptime)
	assert.Equal(t, 90.48, *entry.Uptime)
	assert.Equal(t, 1, entry.Outages)
	require.NotNil(t, entry.CoveredFrom)
	assert.Equal(t, coveredFrom, *entry.CoveredFrom)
}

func TestService_Windows_ClipsAndFlagsTruncation(t *testing.T) {
	to := time.Now()
	from := to.Add(-2 * time.Hour)
	// Starts before the range, so only the part inside it counts.
	earlier := from.Add(-10 * time.Hour)
	closed := from.Add(30 * time.Minute)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]uptime.RangeStat{}, nil)
	// One row over the limit is what tells the service there is more; it must not be
	// reported as data.
	repo.EXPECT().Windows(mock.Anything, mock.Anything, mock.Anything, mock.Anything, 1).
		Return([]uptime.WindowRow{
			{Device: uptime.DeviceLab, State: uptime.StateUp, StartTime: earlier, EndTime: &closed},
			{Device: uptime.DeviceLab, State: uptime.StateDown, StartTime: earlier.Add(-time.Hour), EndTime: &earlier},
		}, nil)

	got, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{From: from, To: to, Limit: 1})
	require.NoError(t, err)

	lab := got.Devices[0]
	require.Len(t, lab.Windows, 1)
	assert.True(t, lab.Truncated)
	assert.InDelta(t, 30*60.0, lab.Windows[0].Seconds, 1)
}

func TestService_Windows_OpenWindowCountsUpToTo(t *testing.T) {
	to := time.Now()
	from := to.Add(-2 * time.Hour)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]uptime.RangeStat{}, nil)
	repo.EXPECT().Windows(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]uptime.WindowRow{
			{Device: uptime.DeviceLab, State: uptime.StateUp, StartTime: to.Add(-time.Hour), EndTime: nil},
		}, nil)

	got, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{From: from, To: to})
	require.NoError(t, err)

	window := got.Devices[0].Windows[0]
	assert.Nil(t, window.EndTime, "the open window stays open in the response")
	assert.InDelta(t, 3600.0, window.Seconds, 1)
}

func TestService_Windows_NotConfigured(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().RangeStats(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, uptime.ErrNotConfigured)

	_, err := newService(repo).Windows(context.Background(), uptime.WindowsQuery{})
	assert.ErrorIs(t, err, uptime.ErrNotConfigured)
}

func TestParseDevice(t *testing.T) {
	got, err := uptime.ParseDevice("watchdog")
	require.NoError(t, err)
	assert.Equal(t, uptime.DeviceWatchdog, got)

	_, err = uptime.ParseDevice("printer")
	assert.ErrorIs(t, err, uptime.ErrUnknownDevice)
}
