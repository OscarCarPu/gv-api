package uptime

import (
	"context"
	"time"

	"gv-api/internal/pipeline"

	"github.com/jackc/pgx/v5"
)

type Repository interface {
	CurrentStates(ctx context.Context) ([]CurrentState, error)
	Aggregations(ctx context.Context) ([]Aggregation, error)
	RangeStats(ctx context.Context, from, to time.Time, device *Device) ([]RangeStat, error)
	Windows(ctx context.Context, from, to time.Time, device *Device, limit int) ([]WindowRow, error)
}

// PipelineRepository reads the two watchdog marts over the shared pipeline connection.
// Hand-written SQL rather than sqlc: sqlc generates from db/migrations, and this schema is
// not in it — dbt owns these relations. The device, state and "time" columns are TEXT with a
// closed value set that dbt enforces with accepted_values tests rather than a database enum;
// the ::text casts cost nothing and keep the scans working if that ever changes.
type PipelineRepository struct {
	db *pipeline.DB
}

func NewRepository(db *pipeline.DB) *PipelineRepository {
	return &PipelineRepository{db: db}
}

func (r *PipelineRepository) CurrentStates(ctx context.Context) ([]CurrentState, error) {
	// One open window per device, and that window is the device's state right now. Its
	// start_time is also the newest event the pipeline has for the device, since windows
	// alternate and same-state events collapse into the running one.
	const query = `
		SELECT device::text, state::text, start_time
		FROM marts.uptime_windows
		WHERE end_time IS NULL
		ORDER BY device`

	return pipeline.Collect(ctx, r.db, func(rows pgx.Rows) (CurrentState, error) {
		var s CurrentState
		err := rows.Scan(&s.Device, &s.State, &s.Since)
		return s, err
	}, query)
}

func (r *PipelineRepository) Aggregations(ctx context.Context) ([]Aggregation, error) {
	// Eight rows, two devices by four lookbacks. `time` is a type keyword, hence the
	// quoting.
	const query = `
		SELECT device::text, "time"::text, range_start, range_end, uptime
		FROM marts.uptime_aggregations`

	return pipeline.Collect(ctx, r.db, func(rows pgx.Rows) (Aggregation, error) {
		var a Aggregation
		err := rows.Scan(&a.Device, &a.Range, &a.RangeStart, &a.RangeEnd, &a.Uptime)
		return a, err
	}, query)
}

func (r *PipelineRepository) RangeStats(ctx context.Context, from, to time.Time, device *Device) ([]RangeStat, error) {
	// Windows are clipped to the range instead of being counted whole, and the open one is
	// counted up to `to` — the same assumption the precomputed rows make, that the latest
	// known state persists. Aggregated here rather than in Go so the numbers hold whatever
	// limit the window list is fetched with.
	const query = `
		SELECT device,
		       coalesce(sum(seconds) FILTER (WHERE state = 'up'), 0)   AS up_seconds,
		       coalesce(sum(seconds) FILTER (WHERE state = 'down'), 0) AS down_seconds,
		       count(*) FILTER (WHERE state = 'down')                  AS outages,
		       min(clip_start) AS covered_from,
		       max(clip_end)   AS covered_to
		FROM (
		    SELECT device::text AS device,
		           state::text AS state,
		           greatest(start_time, $1::timestamptz) AS clip_start,
		           least(coalesce(end_time, $2::timestamptz), $2::timestamptz) AS clip_end,
		           extract(epoch FROM least(coalesce(end_time, $2::timestamptz), $2::timestamptz)
		                             - greatest(start_time, $1::timestamptz)) AS seconds
		    FROM marts.uptime_windows
		    WHERE start_time < $2::timestamptz
		      AND coalesce(end_time, $2::timestamptz) > $1::timestamptz
		      AND ($3::text IS NULL OR device::text = $3::text)
		) clipped
		GROUP BY device
		ORDER BY device`

	return pipeline.Collect(ctx, r.db, func(rows pgx.Rows) (RangeStat, error) {
		var s RangeStat
		err := rows.Scan(&s.Device, &s.UpSeconds, &s.DownSeconds, &s.Outages, &s.CoveredFrom, &s.CoveredTo)
		return s, err
	}, query, from, to, deviceFilter(device))
}

func (r *PipelineRepository) Windows(ctx context.Context, from, to time.Time, device *Device, limit int) ([]WindowRow, error) {
	// Newest first: when the limit cuts the list it is the distant past that goes, not the
	// outage from this morning. One row over the limit is fetched so the caller can tell
	// "exactly full" from "there is more".
	const query = `
		SELECT device::text, state::text, start_time, end_time
		FROM marts.uptime_windows
		WHERE start_time < $2::timestamptz
		  AND coalesce(end_time, $2::timestamptz) > $1::timestamptz
		  AND ($3::text IS NULL OR device::text = $3::text)
		ORDER BY start_time DESC, device
		LIMIT $4`

	return pipeline.Collect(ctx, r.db, func(rows pgx.Rows) (WindowRow, error) {
		var w WindowRow
		err := rows.Scan(&w.Device, &w.State, &w.StartTime, &w.EndTime)
		return w, err
	}, query, from, to, deviceFilter(device), limit+1)
}

// deviceFilter turns an optional device into the nullable text parameter the queries take.
func deviceFilter(device *Device) *string {
	if device == nil {
		return nil
	}
	s := string(*device)
	return &s
}
