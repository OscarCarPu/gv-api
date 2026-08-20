package uptime

import (
	"context"
	"math"
	"time"

	"gv-api/internal/pipeline"
)

const (
	// defaultLookback is how far back /windows goes when no range is given.
	defaultLookback = 30 * 24 * time.Hour
	// DefaultWindowLimit and MaxWindowLimit bound the window list. A year of real data is
	// ~400 windows per device, so the default covers the whole history in one read while
	// still refusing to stream an unbounded table if the pipeline ever gets noisy.
	DefaultWindowLimit = 1000
	MaxWindowLimit     = 5000
)

type Service struct {
	repo Repository
	// staleAfter is how old the precomputed numbers may be before they stop being
	// presented as current.
	staleAfter time.Duration
}

func NewService(repo Repository, staleAfter time.Duration) *Service {
	return &Service{repo: repo, staleAfter: staleAfter}
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	states, err := s.repo.CurrentStates(ctx)
	if err != nil {
		return Overview{}, err
	}
	aggregations, err := s.repo.Aggregations(ctx)
	if err != nil {
		return Overview{}, err
	}

	stateByDevice := make(map[Device]CurrentState, len(states))
	for _, st := range states {
		stateByDevice[st.Device] = st
	}
	rangesByDevice := make(map[Device]map[Lookback]Aggregation, len(Devices))
	// computedAt is the newest run time present. The rows are written by one dbt run, so
	// in practice they all carry the same value; taking the max avoids reporting a stale
	// half if that ever stops being true.
	var computedAt *time.Time
	for _, a := range aggregations {
		if rangesByDevice[a.Device] == nil {
			rangesByDevice[a.Device] = map[Lookback]Aggregation{}
		}
		rangesByDevice[a.Device][a.Range] = a
		if computedAt == nil || a.RangeEnd.After(*computedAt) {
			end := a.RangeEnd
			computedAt = &end
		}
	}

	out := Overview{
		ComputedAt:        computedAt,
		StaleAfterSeconds: int(s.staleAfter.Seconds()),
		Devices:           make([]DeviceOverview, 0, len(Devices)),
	}
	out.Stale = pipeline.IsStale(computedAt, s.staleAfter)

	// Both devices always appear, in a fixed order, whether or not the pipeline has heard
	// from them. A missing device is a fact worth rendering, not a row to drop.
	for _, device := range Devices {
		entry := DeviceOverview{Device: device, State: StateUnknown, Ranges: []RangeUptime{}}
		if st, ok := stateByDevice[device]; ok {
			entry.State = st.State
			since := st.Since
			entry.Since = &since
		}
		for _, lookback := range Lookbacks {
			a, ok := rangesByDevice[device][lookback]
			if !ok {
				continue
			}
			entry.Ranges = append(entry.Ranges, RangeUptime{
				Range:      lookback,
				Uptime:     a.Uptime,
				RangeStart: a.RangeStart,
				RangeEnd:   a.RangeEnd,
			})
		}
		out.Devices = append(out.Devices, entry)
	}
	return out, nil
}

func (s *Service) Windows(ctx context.Context, q WindowsQuery) (WindowsReport, error) {
	q, err := s.normalize(q)
	if err != nil {
		return WindowsReport{}, err
	}

	stats, err := s.repo.RangeStats(ctx, q.From, q.To, q.Device)
	if err != nil {
		return WindowsReport{}, err
	}
	rows, err := s.repo.Windows(ctx, q.From, q.To, q.Device, q.Limit)
	if err != nil {
		return WindowsReport{}, err
	}

	// One extra row is fetched to detect truncation; it is not part of the answer.
	truncated := len(rows) > q.Limit
	if truncated {
		rows = rows[:q.Limit]
	}

	statByDevice := make(map[Device]RangeStat, len(stats))
	for _, st := range stats {
		statByDevice[st.Device] = st
	}
	windowsByDevice := make(map[Device][]Window, len(Devices))
	for _, row := range rows {
		windowsByDevice[row.Device] = append(windowsByDevice[row.Device], Window{
			State:     row.State,
			StartTime: row.StartTime,
			EndTime:   row.EndTime,
			Seconds:   clippedSeconds(row, q.From, q.To),
		})
	}

	report := WindowsReport{From: q.From, To: q.To, Devices: make([]DeviceWindows, 0, len(Devices))}
	for _, device := range Devices {
		if q.Device != nil && *q.Device != device {
			continue
		}
		entry := DeviceWindows{Device: device, Windows: windowsByDevice[device], Truncated: truncated}
		if entry.Windows == nil {
			entry.Windows = []Window{}
		}
		if st, ok := statByDevice[device]; ok {
			entry.UpSeconds = st.UpSeconds
			entry.DownSeconds = st.DownSeconds
			entry.Outages = st.Outages
			from, to := st.CoveredFrom, st.CoveredTo
			entry.CoveredFrom, entry.CoveredTo = &from, &to
			// Divide by what the windows cover, not by the whole range: before a device's
			// first event there is nothing to call up or down, and charging that gap as
			// downtime would report a young device as mostly dead.
			if covered := st.UpSeconds + st.DownSeconds; covered > 0 {
				// Two decimals, the precision the precomputed rows come in, so a tile
				// showing a custom range next to a lookback shows the same kind of number.
				uptime := math.Round(100*100*st.UpSeconds/covered) / 100
				entry.Uptime = &uptime
			}
		}
		report.Devices = append(report.Devices, entry)
	}
	return report, nil
}

// normalize fills in the defaults and rejects a range that cannot be served. `to` is
// clamped to now: the open window is counted up to `to`, so asking about the future would
// invent uptime that has not happened.
func (s *Service) normalize(q WindowsQuery) (WindowsQuery, error) {
	now := time.Now()
	if q.To.IsZero() || q.To.After(now) {
		q.To = now
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-defaultLookback)
	}
	if !q.From.Before(q.To) {
		return q, ErrInvalidRange
	}
	if q.Limit <= 0 {
		q.Limit = DefaultWindowLimit
	}
	if q.Limit > MaxWindowLimit {
		q.Limit = MaxWindowLimit
	}
	return q, nil
}

// clippedSeconds is how much of a window falls inside [from, to). An open window is
// counted up to `to`, matching what the pipeline assumes: the latest known state persists.
func clippedSeconds(row WindowRow, from, to time.Time) float64 {
	start := row.StartTime
	if start.Before(from) {
		start = from
	}
	end := to
	if row.EndTime != nil && row.EndTime.Before(to) {
		end = *row.EndTime
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start).Seconds()
}
