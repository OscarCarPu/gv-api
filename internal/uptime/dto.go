package uptime

import "time"

type Kind string

const (
	KindUp       Kind = "up"
	KindDown     Kind = "down"
	KindFailover Kind = "failover"
)

type Lookback string

const (
	LookbackMonth       Lookback = "month"
	LookbackThreeMonths Lookback = "3 months"
	LookbackYear        Lookback = "year"
	LookbackAll         Lookback = "all"
)

var Lookbacks = []Lookback{LookbackMonth, LookbackThreeMonths, LookbackYear, LookbackAll}

func (l Lookback) Duration() time.Duration {
	switch l {
	case LookbackMonth:
		return 30 * 24 * time.Hour
	case LookbackThreeMonths:
		return 90 * 24 * time.Hour
	case LookbackYear:
		return 365 * 24 * time.Hour
	}
	return 0
}

type Range struct {
	Kind  Kind
	Start time.Time
	End   time.Time
}

type Overview struct {
	UpSince time.Time     `json:"up_since"`
	Ranges  []RangeUptime `json:"ranges"`
}

type RangeUptime struct {
	Range           Lookback  `json:"range"`
	Uptime          *float64  `json:"uptime"`
	UpSeconds       float64   `json:"up_seconds"`
	DownSeconds     float64   `json:"down_seconds"`
	FailoverSeconds float64   `json:"failover_seconds"`
	RangeStart      time.Time `json:"range_start"`
	RangeEnd        time.Time `json:"range_end"`
}

type WindowsQuery struct {
	From  time.Time
	To    time.Time
	Limit int
}

type WindowsReport struct {
	From            time.Time `json:"from"`
	To              time.Time `json:"to"`
	Uptime          *float64  `json:"uptime"`
	UpSeconds       float64   `json:"up_seconds"`
	DownSeconds     float64   `json:"down_seconds"`
	FailoverSeconds float64   `json:"failover_seconds"`
	Outages         int       `json:"outages"`
	Windows         []Window  `json:"windows"`
	Truncated       bool      `json:"truncated"`
}

type Window struct {
	Kind      Kind      `json:"kind"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Seconds   float64   `json:"seconds"`
}
