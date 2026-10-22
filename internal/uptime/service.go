package uptime

import (
	"context"
	"math"
	"time"
)

const (
	BeatEvery = 30 * time.Second
	Gap       = 90 * time.Second

	defaultLookback    = 30 * 24 * time.Hour
	DefaultWindowLimit = 1000
	MaxWindowLimit     = 5000
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	since, err := s.repo.TrackingStart(ctx)
	if err != nil {
		return Overview{}, err
	}
	now := s.now()
	ranges, err := s.repo.Ranges(ctx, since, now)
	if err != nil {
		return Overview{}, err
	}

	out := Overview{UpSince: since, Ranges: make([]RangeUptime, 0, len(Lookbacks))}
	for _, r := range ranges {
		if r.Kind != KindUp {
			out.UpSince = r.End
		}
	}
	for _, l := range Lookbacks {
		from := since
		if d := l.Duration(); d > 0 && now.Add(-d).After(since) {
			from = now.Add(-d)
		}
		t := total(ranges, from, now)
		out.Ranges = append(out.Ranges, RangeUptime{
			Range: l, Uptime: t.percent(),
			UpSeconds: t.up, DownSeconds: t.down, FailoverSeconds: t.failover,
			RangeStart: from, RangeEnd: now,
		})
	}
	return out, nil
}

func (s *Service) Windows(ctx context.Context, q WindowsQuery) (WindowsReport, error) {
	since, err := s.repo.TrackingStart(ctx)
	if err != nil {
		return WindowsReport{}, err
	}
	q, err = normalize(q, s.now(), since)
	if err != nil {
		return WindowsReport{}, err
	}
	ranges, err := s.repo.Ranges(ctx, q.From, q.To)
	if err != nil {
		return WindowsReport{}, err
	}

	t := total(ranges, q.From, q.To)
	report := WindowsReport{
		From: q.From, To: q.To, Uptime: t.percent(),
		UpSeconds: t.up, DownSeconds: t.down, FailoverSeconds: t.failover, Outages: t.outages,
		Windows: []Window{},
	}
	for i := len(ranges) - 1; i >= 0; i-- {
		start, end := clip(ranges[i], q.From, q.To)
		if !end.After(start) {
			continue
		}
		if len(report.Windows) == q.Limit {
			report.Truncated = true
			break
		}
		report.Windows = append(report.Windows, Window{
			Kind: ranges[i].Kind, StartTime: ranges[i].Start, EndTime: ranges[i].End,
			Seconds: end.Sub(start).Seconds(),
		})
	}
	return report, nil
}

func normalize(q WindowsQuery, now, since time.Time) (WindowsQuery, error) {
	if q.To.IsZero() || q.To.After(now) {
		q.To = now
	}
	if q.From.IsZero() {
		q.From = q.To.Add(-defaultLookback)
	}
	if q.From.Before(since) {
		q.From = since
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

type totals struct {
	up, down, failover float64
	outages            int
}

func total(ranges []Range, from, to time.Time) totals {
	var t totals
	for _, r := range ranges {
		start, end := clip(r, from, to)
		seconds := end.Sub(start).Seconds()
		if seconds <= 0 {
			continue
		}
		switch r.Kind {
		case KindUp:
			t.up += seconds
		case KindDown:
			t.down += seconds
			t.outages++
		case KindFailover:
			t.failover += seconds
		}
	}
	return t
}

func (t totals) percent() *float64 {
	covered := t.up + t.down + t.failover
	if covered <= 0 {
		return nil
	}
	v := math.Round(100*100*(t.up+t.failover)/covered) / 100
	return &v
}

func clip(r Range, from, to time.Time) (time.Time, time.Time) {
	start, end := r.Start, r.End
	if start.Before(from) {
		start = from
	}
	if end.After(to) {
		end = to
	}
	return start, end
}
