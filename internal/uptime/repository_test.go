package uptime_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/pipeline"
	"gv-api/internal/testutil"
	"gv-api/internal/uptime"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tables belong to central-pipeline, whose dbt models create them — there is no gv
// migration to lean on, so the fixture builds the two relations the queries read, with the
// column types the contract in central-pipeline's docs/sources/watchdog.md specifies. That
// keeps the SQL under test (the clipping, the quoted `time` column, the overlap filter)
// honest without needing the other project's stack running.
const martsFixture = `
CREATE SCHEMA IF NOT EXISTS marts;
DROP TABLE IF EXISTS marts.uptime_windows;
DROP TABLE IF EXISTS marts.uptime_aggregations;
CREATE TABLE marts.uptime_windows (
    device     text,
    state      text,
    start_time timestamptz,
    end_time   timestamptz
);
CREATE TABLE marts.uptime_aggregations (
    device      text,
    "time"      text,
    range_start timestamptz,
    range_end   timestamptz,
    uptime      double precision
);`

// base is a fixed instant every fixture timestamp is offset from, so the assertions can be
// exact instead of approximate.
var base = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) (*uptime.PipelineRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, martsFixture)
	require.NoError(t, err)
	t.Cleanup(func() {
		//nolint:errcheck // best effort: the test database is dropped after the run anyway
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS marts.uptime_windows, marts.uptime_aggregations")
	})
	return uptime.NewRepository(pipeline.FromPool(pool)), pool
}

// seedWindows lays down a history for both devices:
//
//	lab:      up  [base, base+10h)  down [base+10h, base+12h)  up  [base+12h, open)
//	watchdog: up  [base, base+20h)  down [base+20h, open)
func seedWindows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows := []struct {
		device string
		state  string
		start  time.Time
		end    *time.Time
	}{
		{"lab", "up", base, ptr(base.Add(10 * time.Hour))},
		{"lab", "down", base.Add(10 * time.Hour), ptr(base.Add(12 * time.Hour))},
		{"lab", "up", base.Add(12 * time.Hour), nil},
		{"watchdog", "up", base, ptr(base.Add(20 * time.Hour))},
		{"watchdog", "down", base.Add(20 * time.Hour), nil},
	}
	for _, r := range rows {
		_, err := pool.Exec(context.Background(),
			"INSERT INTO marts.uptime_windows (device, state, start_time, end_time) VALUES ($1, $2, $3, $4)",
			r.device, r.state, r.start, r.end)
		require.NoError(t, err)
	}
}

func ptr[T any](v T) *T { return &v }

// sameInstant compares timestamps by the moment they name. pgx hands timestamptz back in
// the session's own zone, so the fixture's UTC values come out as +02:00 — the same instant
// carrying a different Location, which struct equality would call a difference.
func sameInstant(t *testing.T, want, got time.Time) {
	t.Helper()
	assert.WithinDuration(t, want, got, 0)
}

func TestIntegration_CurrentStates(t *testing.T) {
	repo, pool := newRepo(t)
	seedWindows(t, pool)

	got, err := repo.CurrentStates(context.Background())
	require.NoError(t, err)

	// Exactly one open window per device — that row is the device's state right now.
	require.Len(t, got, 2)
	assert.Equal(t, uptime.DeviceLab, got[0].Device)
	assert.Equal(t, uptime.StateUp, got[0].State)
	sameInstant(t, base.Add(12*time.Hour), got[0].Since)
	assert.Equal(t, uptime.DeviceWatchdog, got[1].Device)
	assert.Equal(t, uptime.StateDown, got[1].State)
	sameInstant(t, base.Add(20*time.Hour), got[1].Since)
}

func TestIntegration_CurrentStates_EmptyPipeline(t *testing.T) {
	repo, _ := newRepo(t)

	got, err := repo.CurrentStates(context.Background())
	require.NoError(t, err)
	assert.Empty(t, got, "no rows is a valid pipeline state, not an error")
}

func TestIntegration_Aggregations(t *testing.T) {
	repo, pool := newRepo(t)
	runAt := base.Add(24 * time.Hour)
	// '3 months' exercises the column that shares its name with a type keyword.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO marts.uptime_aggregations (device, "time", range_start, range_end, uptime)
		VALUES ('lab', '3 months', $1, $2, 98.28), ('watchdog', 'all', $1, $2, 98.04)`,
		base, runAt)
	require.NoError(t, err)

	got, err := repo.Aggregations(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, uptime.DeviceLab, got[0].Device)
	assert.Equal(t, uptime.LookbackThreeMonths, got[0].Range)
	assert.InDelta(t, 98.28, got[0].Uptime, 0.001)
	sameInstant(t, base, got[0].RangeStart)
	sameInstant(t, runAt, got[0].RangeEnd)
	assert.Equal(t, uptime.LookbackAll, got[1].Range)
}

func TestIntegration_RangeStats_ClipsToRange(t *testing.T) {
	repo, pool := newRepo(t)
	seedWindows(t, pool)
	ctx := context.Background()

	// A range that starts inside lab's first up window and ends inside its open one: 1h of
	// the first up, the whole 2h outage, then 4h of the open window counted up to `to`.
	from, to := base.Add(9*time.Hour), base.Add(16*time.Hour)
	got, err := repo.RangeStats(ctx, from, to, nil)
	require.NoError(t, err)
	require.Len(t, got, 2)

	lab := got[0]
	assert.Equal(t, uptime.DeviceLab, lab.Device)
	assert.InDelta(t, 5*3600.0, lab.UpSeconds, 0.5)
	assert.InDelta(t, 2*3600.0, lab.DownSeconds, 0.5)
	assert.Equal(t, 1, lab.Outages)
	// A window starting before the range is clipped to it, and the open one is counted up
	// to the end of the range.
	sameInstant(t, from, lab.CoveredFrom)
	sameInstant(t, to, lab.CoveredTo)

	// watchdog was up for the whole of it.
	assert.Equal(t, uptime.DeviceWatchdog, got[1].Device)
	assert.InDelta(t, 7*3600.0, got[1].UpSeconds, 0.5)
	assert.Zero(t, got[1].DownSeconds)
	assert.Equal(t, 0, got[1].Outages)
}

func TestIntegration_RangeStats_DeviceFilterAndNoOverlap(t *testing.T) {
	repo, pool := newRepo(t)
	seedWindows(t, pool)
	ctx := context.Background()

	device := uptime.DeviceWatchdog
	got, err := repo.RangeStats(ctx, base.Add(21*time.Hour), base.Add(30*time.Hour), &device)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, uptime.DeviceWatchdog, got[0].Device)
	assert.InDelta(t, 9*3600.0, got[0].DownSeconds, 0.5)

	// A range entirely before the first event has nothing to report — not zero uptime.
	got, err = repo.RangeStats(ctx, base.Add(-48*time.Hour), base.Add(-24*time.Hour), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestIntegration_Windows_NewestFirstAndOverLimit(t *testing.T) {
	repo, pool := newRepo(t)
	seedWindows(t, pool)
	ctx := context.Background()

	got, err := repo.Windows(ctx, base, base.Add(24*time.Hour), nil, 10)
	require.NoError(t, err)
	require.Len(t, got, 5)
	// Newest first, so cutting the list drops the distant past rather than today.
	sameInstant(t, base.Add(20*time.Hour), got[0].StartTime)
	assert.Equal(t, uptime.StateDown, got[0].State)
	assert.Nil(t, got[0].EndTime, "the open window keeps its null end")
	sameInstant(t, base, got[len(got)-1].StartTime)

	// One row over the limit comes back so the caller can tell "full" from "there is more".
	got, err = repo.Windows(ctx, base, base.Add(24*time.Hour), nil, 2)
	require.NoError(t, err)
	assert.Len(t, got, 3)

	device := uptime.DeviceLab
	got, err = repo.Windows(ctx, base, base.Add(24*time.Hour), &device, 10)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for _, w := range got {
		assert.Equal(t, uptime.DeviceLab, w.Device)
	}
}

func TestIntegration_Windows_ExcludesNonOverlapping(t *testing.T) {
	repo, pool := newRepo(t)
	seedWindows(t, pool)

	// Touching only at the boundary is not an overlap: [from, to) is half-open.
	got, err := repo.Windows(context.Background(), base.Add(-10*time.Hour), base, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, got)
}
