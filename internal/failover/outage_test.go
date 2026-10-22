package failover_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gv-api/internal/failover"
	"gv-api/internal/restore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recorded struct {
	lastAlive *time.Time
	ec2UpAt   time.Time
	homeAt    time.Time
}

type fakeOutages struct{ calls []recorded }

func (f *fakeOutages) RecordFailover(_ context.Context, lastAlive *time.Time, ec2UpAt, homeAt time.Time) error {
	f.calls = append(f.calls, recorded{lastAlive, ec2UpAt, homeAt})
	return nil
}

var (
	alive   = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	ec2UpAt = time.Date(2026, 8, 1, 10, 20, 0, 0, time.UTC)
	homeAt  = time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
)

const outageBody = `{"ec2_up_at":"2026-08-01T10:20:00Z","home_at":"2026-08-01T11:00:00Z"}`

func withLastAlive(int) (*restore.Outcome, error) {
	return &restore.Outcome{Swapped: true, LastAlive: &alive}, nil
}

func restored(t *testing.T, outcome func(int) (*restore.Outcome, error)) (*fixture, *fakeOutages) {
	t.Helper()
	f := newFixture(t, state("FAILING_BACK"), outcome)
	outages := &fakeOutages{}
	f.ctl.SetOutages(outages)
	require.Equal(t, http.StatusOK, f.do("POST", "/failover", token, `{"mode":"merge"}`).Code)
	f.waitFor(t, failover.MergeDone)
	f.params.set("/home-lab-failover/state", state("HOME_READY"))
	return f, outages
}

func TestOutage_NeedsTheToken(t *testing.T) {
	f, outages := restored(t, withLastAlive)

	for _, tok := range []string{"", "wrong"} {
		assert.Equal(t, http.StatusUnauthorized, f.do("POST", "/failover/outage", tok, outageBody).Code)
	}
	assert.Empty(t, outages.calls)
}

func TestOutage_RefusesABodyThatMakesNoSense(t *testing.T) {
	f, outages := restored(t, withLastAlive)

	for _, body := range []string{
		`not json`,
		`{}`,
		`{"ec2_up_at":"2026-08-01T11:00:00Z","home_at":"2026-08-01T10:20:00Z"}`,
		`{"ec2_up_at":"2026-08-01T10:20:00Z","home_at":"2999-01-01T00:00:00Z"}`,
	} {
		assert.Equal(t, http.StatusBadRequest, f.do("POST", "/failover/outage", token, body).Code, body)
	}
	assert.Empty(t, outages.calls)
}

func TestOutage_OnlyWhileTheStateIsHomeReady(t *testing.T) {
	f, outages := restored(t, withLastAlive)
	f.params.set("/home-lab-failover/state", state("PENDING_MERGE"))

	assert.Equal(t, http.StatusConflict, f.do("POST", "/failover/outage", token, outageBody).Code)
	assert.Empty(t, outages.calls)
}

func TestOutage_WithoutTheUptimeHistoryIs503(t *testing.T) {
	f := newFixture(t, state("HOME_READY"), withLastAlive)

	assert.Equal(t, http.StatusServiceUnavailable, f.do("POST", "/failover/outage", token, outageBody).Code)
}

func TestOutage_RecordsWithHomesLastHeartbeatThenForgetsIt(t *testing.T) {
	f, outages := restored(t, withLastAlive)

	require.Equal(t, http.StatusOK, f.do("POST", "/failover/outage", token, outageBody).Code)
	require.Equal(t, http.StatusOK, f.do("POST", "/failover/outage", token, outageBody).Code)

	require.Len(t, outages.calls, 2)
	require.NotNil(t, outages.calls[0].lastAlive)
	assert.True(t, outages.calls[0].lastAlive.Equal(alive))
	assert.True(t, outages.calls[0].ec2UpAt.Equal(ec2UpAt))
	assert.True(t, outages.calls[0].homeAt.Equal(homeAt))
	assert.Nil(t, outages.calls[1].lastAlive)
}

func TestOutage_ARestartLosesTheHeartbeatButStillRecords(t *testing.T) {
	f, _ := restored(t, withLastAlive)
	restarted := f.controller(token)
	outages := &fakeOutages{}
	restarted.SetOutages(outages)
	f.router = f.routes(restarted)

	require.Equal(t, http.StatusOK, f.do("POST", "/failover/outage", token, outageBody).Code)

	require.Len(t, outages.calls, 1)
	assert.Nil(t, outages.calls[0].lastAlive)
}

func TestBeating(t *testing.T) {
	ctx := context.Background()

	t.Run("false while held", func(t *testing.T) {
		f := newFixture(t, state("FAILED_OVER"), swapped)
		f.ctl.Start(ctx)
		assert.False(t, f.ctl.Beating(ctx))
	})

	t.Run("false while the state is not NORMAL even if the app is open", func(t *testing.T) {
		f, _ := restored(t, withLastAlive)
		require.Equal(t, failover.ModeNormal, f.ctl.Mode())
		assert.False(t, f.ctl.Beating(ctx))
	})

	t.Run("true once the state is NORMAL", func(t *testing.T) {
		f, _ := restored(t, withLastAlive)
		f.params.set("/home-lab-failover/state", state("NORMAL"))
		assert.True(t, f.ctl.Beating(ctx))
	})

	t.Run("true when the state cannot be read", func(t *testing.T) {
		f := newFixture(t, state("NORMAL"), swapped)
		f.params.err = assert.AnError
		assert.True(t, f.ctl.Beating(ctx))
	})
}
