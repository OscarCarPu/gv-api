package uptime_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/testutil"
	"gv-api/internal/uptime"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var origin = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

func newRepo(t *testing.T) (*uptime.PostgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "outages")
	_, err := pool.Exec(context.Background(), "INSERT INTO outages (ends_at, kind) VALUES ($1, 'start')", origin)
	require.NoError(t, err)
	return uptime.NewRepository(pool), pool
}

type row struct {
	Kind string
	End  time.Time
}

func rows(t *testing.T, pool *pgxpool.Pool) []row {
	t.Helper()
	r, err := pool.Query(context.Background(), "SELECT kind::text, ends_at FROM outages ORDER BY ends_at")
	require.NoError(t, err)
	defer r.Close()
	var out []row
	for r.Next() {
		var x row
		require.NoError(t, r.Scan(&x.Kind, &x.End))
		x.End = x.End.UTC()
		out = append(out, x)
	}
	return out
}

func minutes(n float64) time.Time {
	return origin.Add(time.Duration(n * float64(time.Minute)))
}

func TestIntegration_Beat_FirstBeatStartsAnUpRange(t *testing.T) {
	repo, pool := newRepo(t)

	require.NoError(t, repo.Beat(context.Background(), minutes(1), uptime.Gap))

	assert.Equal(t, []row{{"start", origin}, {"up", minutes(1)}}, rows(t, pool))
}

func TestIntegration_Beat_WithinTheGapExtendsTheSameRange(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(1), uptime.Gap))

	require.NoError(t, repo.Beat(ctx, minutes(1.5), uptime.Gap))
	require.NoError(t, repo.Beat(ctx, minutes(2.5), uptime.Gap))

	assert.Equal(t, []row{{"start", origin}, {"up", minutes(2.5)}}, rows(t, pool))
}

func TestIntegration_Beat_ASilenceLongerThanTheGapIsDowntime(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(1), uptime.Gap))

	require.NoError(t, repo.Beat(ctx, minutes(10), uptime.Gap))
	require.NoError(t, repo.Beat(ctx, minutes(10.5), uptime.Gap))

	assert.Equal(t, []row{
		{"start", origin}, {"up", minutes(1)}, {"down", minutes(10)}, {"up", minutes(10.5)},
	}, rows(t, pool))
}

func TestIntegration_Beat_StartsAnUpRangeAfterAFailover(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, "INSERT INTO outages (ends_at, kind) VALUES ($1, 'failover')", minutes(5))
	require.NoError(t, err)

	require.NoError(t, repo.Beat(ctx, minutes(6), uptime.Gap))

	assert.Equal(t, []row{{"start", origin}, {"failover", minutes(5)}, {"up", minutes(6)}}, rows(t, pool))
}

func TestIntegration_Beat_AClockBehindTheLastRowWritesNothing(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(5), uptime.Gap))

	require.NoError(t, repo.Beat(ctx, minutes(4), uptime.Gap))

	assert.Equal(t, []row{{"start", origin}, {"up", minutes(5)}}, rows(t, pool))
}

func TestIntegration_Ranges_StartWhereThePreviousOneEnds(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	for _, r := range []row{{"up", minutes(10)}, {"down", minutes(20)}, {"failover", minutes(30)}, {"up", minutes(40)}} {
		_, err := pool.Exec(ctx, "INSERT INTO outages (ends_at, kind) VALUES ($1, $2)", r.End, r.Kind)
		require.NoError(t, err)
	}

	got, err := repo.Ranges(ctx, minutes(15), minutes(35))

	require.NoError(t, err)
	for i := range got {
		got[i].Start, got[i].End = got[i].Start.UTC(), got[i].End.UTC()
	}
	assert.Equal(t, []uptime.Range{
		{Kind: uptime.KindDown, Start: minutes(10), End: minutes(20)},
		{Kind: uptime.KindFailover, Start: minutes(20), End: minutes(30)},
		{Kind: uptime.KindUp, Start: minutes(30), End: minutes(40)},
	}, got)
}

func TestIntegration_Ranges_NeverReturnTheStartRow(t *testing.T) {
	repo, _ := newRepo(t)

	got, err := repo.Ranges(context.Background(), origin.Add(-time.Hour), origin.Add(time.Hour))

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestIntegration_TrackingStart(t *testing.T) {
	repo, _ := newRepo(t)

	got, err := repo.TrackingStart(context.Background())

	require.NoError(t, err)
	assert.True(t, got.Equal(origin))
}

func TestIntegration_RecordFailover_WritesTheDowntimeAndTheFailover(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(1), uptime.Gap))
	lastAlive := minutes(1.4)

	require.NoError(t, repo.RecordFailover(ctx, &lastAlive, minutes(20), minutes(60)))

	assert.Equal(t, []row{
		{"start", origin}, {"up", minutes(1.4)}, {"down", minutes(20)}, {"failover", minutes(60)},
	}, rows(t, pool))
}

func TestIntegration_RecordFailover_TwiceChangesNothing(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(1), uptime.Gap))
	require.NoError(t, repo.RecordFailover(ctx, nil, minutes(20), minutes(60)))
	before := rows(t, pool)

	require.NoError(t, repo.RecordFailover(ctx, nil, minutes(20), minutes(60)))

	assert.Equal(t, before, rows(t, pool))
}

func TestIntegration_RecordFailover_IgnoresALastAliveOlderThanTheUpRange(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(5), uptime.Gap))
	older := minutes(2)

	require.NoError(t, repo.RecordFailover(ctx, &older, minutes(20), minutes(60)))

	assert.Equal(t, row{"up", minutes(5)}, rows(t, pool)[1])
}

func TestIntegration_RecordFailover_NoDowntimeWhenEC2WasUpBeforeTheLastBeat(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Beat(ctx, minutes(30), uptime.Gap))

	require.NoError(t, repo.RecordFailover(ctx, nil, minutes(20), minutes(60)))

	assert.Equal(t, []row{{"start", origin}, {"up", minutes(30)}, {"failover", minutes(60)}}, rows(t, pool))
}

func TestIntegration_LastAlive(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	empty, err := uptime.LastAlive(ctx, pool)
	require.NoError(t, err)
	assert.Nil(t, empty)

	require.NoError(t, repo.Beat(ctx, minutes(3), uptime.Gap))
	got, err := uptime.LastAlive(ctx, pool)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Equal(minutes(3)))
}
