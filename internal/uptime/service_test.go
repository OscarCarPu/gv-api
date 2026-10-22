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

var now = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

func at(hoursAgo float64) time.Time {
	return now.Add(-time.Duration(hoursAgo * float64(time.Hour)))
}

func rng(kind uptime.Kind, fromHoursAgo, toHoursAgo float64) uptime.Range {
	return uptime.Range{Kind: kind, Start: at(fromHoursAgo), End: at(toHoursAgo)}
}

func newService(t *testing.T, since time.Time, ranges ...uptime.Range) *uptime.Service {
	t.Helper()
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(since, nil)
	repo.EXPECT().Ranges(mock.Anything, mock.Anything, mock.Anything).Return(ranges, nil).Maybe()
	return uptime.NewService(repo, func() time.Time { return now })
}

func lookback(t *testing.T, o uptime.Overview, l uptime.Lookback) uptime.RangeUptime {
	t.Helper()
	for _, r := range o.Ranges {
		if r.Range == l {
			return r
		}
	}
	t.Fatalf("no %s lookback", l)
	return uptime.RangeUptime{}
}

func TestService_Overview_NothingCoveredYetHasNoPercentage(t *testing.T) {
	o, err := newService(t, now).Overview(context.Background())

	require.NoError(t, err)
	assert.Len(t, o.Ranges, 4)
	assert.Nil(t, lookback(t, o, uptime.LookbackAll).Uptime)
	assert.Equal(t, now, o.UpSince)
}

func TestService_Overview_OnlyUpIsOneHundred(t *testing.T) {
	o, err := newService(t, at(10), rng(uptime.KindUp, 10, 0)).Overview(context.Background())

	require.NoError(t, err)
	all := lookback(t, o, uptime.LookbackAll)
	require.NotNil(t, all.Uptime)
	assert.Equal(t, 100.0, *all.Uptime)
	assert.Equal(t, at(10), all.RangeStart)
	assert.Equal(t, at(10), o.UpSince)
}

func TestService_Overview_DowntimeLowersItAndFailoverDoesNot(t *testing.T) {
	o, err := newService(t, at(10),
		rng(uptime.KindUp, 10, 6),
		rng(uptime.KindDown, 6, 5),
		rng(uptime.KindFailover, 5, 3),
		rng(uptime.KindUp, 3, 0),
	).Overview(context.Background())

	require.NoError(t, err)
	all := lookback(t, o, uptime.LookbackAll)
	require.NotNil(t, all.Uptime)
	assert.Equal(t, 90.0, *all.Uptime)
	assert.Equal(t, 3600.0, all.DownSeconds)
	assert.Equal(t, 7200.0, all.FailoverSeconds)
	assert.Equal(t, 7*3600.0, all.UpSeconds)
	assert.Equal(t, at(3), o.UpSince)
}

func TestService_Overview_ALookbackCutsTheRangeAndKeepsTheStart(t *testing.T) {
	since := now.Add(-100 * 24 * time.Hour)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(since, nil)
	repo.EXPECT().Ranges(mock.Anything, since, now).Return([]uptime.Range{{Kind: uptime.KindUp, Start: since, End: now}}, nil)

	o, err := uptime.NewService(repo, func() time.Time { return now }).Overview(context.Background())

	require.NoError(t, err)
	assert.Equal(t, now.Add(-30*24*time.Hour), lookback(t, o, uptime.LookbackMonth).RangeStart)
	assert.Equal(t, since, lookback(t, o, uptime.LookbackAll).RangeStart)
}

func TestService_Overview_RepoError(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(time.Time{}, errors.New("db down"))

	_, err := uptime.NewService(repo, func() time.Time { return now }).Overview(context.Background())

	assert.EqualError(t, err, "db down")
}

func TestService_Windows_DefaultsToLastThirtyDays(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(now.Add(-365*24*time.Hour), nil)
	repo.EXPECT().Ranges(mock.Anything, now.Add(-30*24*time.Hour), now).Return(nil, nil)

	report, err := uptime.NewService(repo, func() time.Time { return now }).Windows(context.Background(), uptime.WindowsQuery{})

	require.NoError(t, err)
	assert.Equal(t, now, report.To)
	assert.Empty(t, report.Windows)
	assert.Nil(t, report.Uptime)
}

func TestService_Windows_ClampsAFutureToAndFromToTheStart(t *testing.T) {
	since := at(10)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(since, nil)
	repo.EXPECT().Ranges(mock.Anything, since, now).Return(nil, nil)

	report, err := uptime.NewService(repo, func() time.Time { return now }).Windows(context.Background(), uptime.WindowsQuery{
		From: at(100),
		To:   now.Add(48 * time.Hour),
	})

	require.NoError(t, err)
	assert.Equal(t, since, report.From)
	assert.Equal(t, now, report.To)
}

func TestService_Windows_RejectsABackwardsRange(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().TrackingStart(mock.Anything).Return(at(100), nil)

	_, err := uptime.NewService(repo, func() time.Time { return now }).Windows(context.Background(), uptime.WindowsQuery{
		From: at(1),
		To:   at(5),
	})

	assert.ErrorIs(t, err, uptime.ErrInvalidRange)
}

func TestService_Windows_ListsNewestFirstClipsAndCountsDowntimes(t *testing.T) {
	svc := newService(t, at(20),
		rng(uptime.KindUp, 20, 8),
		rng(uptime.KindDown, 8, 7),
		rng(uptime.KindFailover, 7, 4),
		rng(uptime.KindUp, 4, 0),
	)

	report, err := svc.Windows(context.Background(), uptime.WindowsQuery{From: at(10), To: now})

	require.NoError(t, err)
	require.Len(t, report.Windows, 4)
	assert.Equal(t, uptime.KindUp, report.Windows[0].Kind)
	assert.Equal(t, uptime.KindUp, report.Windows[3].Kind)
	assert.Equal(t, 2*3600.0, report.Windows[3].Seconds)
	assert.Equal(t, 1, report.Outages)
	assert.Equal(t, 3600.0, report.DownSeconds)
	assert.Equal(t, 3*3600.0, report.FailoverSeconds)
	assert.Equal(t, 6*3600.0, report.UpSeconds)
	require.NotNil(t, report.Uptime)
	assert.Equal(t, 90.0, *report.Uptime)
	assert.False(t, report.Truncated)
}

func TestService_Windows_FlagsTruncation(t *testing.T) {
	svc := newService(t, at(20),
		rng(uptime.KindUp, 20, 8),
		rng(uptime.KindDown, 8, 7),
		rng(uptime.KindUp, 7, 0),
	)

	report, err := svc.Windows(context.Background(), uptime.WindowsQuery{From: at(20), To: now, Limit: 2})

	require.NoError(t, err)
	assert.Len(t, report.Windows, 2)
	assert.True(t, report.Truncated)
	assert.Equal(t, 1, report.Outages)
}
