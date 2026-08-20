package uptime

import "time"

// Device is one of exactly two publishers: the home lab and the ESP32 that watches it.
// Typed here rather than constrained in a database gv-api does not own.
type Device string

const (
	DeviceLab      Device = "lab"
	DeviceWatchdog Device = "watchdog"
)

// Devices is the order every response lists them in.
var Devices = []Device{DeviceLab, DeviceWatchdog}

func ParseDevice(s string) (Device, error) {
	for _, d := range Devices {
		if string(d) == s {
			return d, nil
		}
	}
	return "", ErrUnknownDevice
}

// State is what a device was doing during a window. StateUnknown is not published by
// anything: it is what a device with no windows at all reads as, so a device the pipeline
// has never heard from still appears instead of silently vanishing.
type State string

const (
	StateUp      State = "up"
	StateDown    State = "down"
	StateUnknown State = "unknown"
)

// Lookback is one of the four precomputed ranges. The values are the pipeline's own
// strings, kept verbatim so both sides name the same thing.
type Lookback string

const (
	LookbackMonth       Lookback = "month"
	LookbackThreeMonths Lookback = "3 months"
	LookbackYear        Lookback = "year"
	LookbackAll         Lookback = "all"
)

// Lookbacks is the order the ranges are reported in, shortest first.
var Lookbacks = []Lookback{LookbackMonth, LookbackThreeMonths, LookbackYear, LookbackAll}

// Overview is the dashboard read: where both devices stand now, and the four percentages
// the pipeline precomputed for each.
type Overview struct {
	// ComputedAt is when dbt last ran, not now. Null when the pipeline has produced
	// nothing yet.
	ComputedAt *time.Time `json:"computed_at"`
	// Stale says the percentages are older than StaleAfterSeconds, so they describe a
	// past run rather than the present. dbt is a batch job: treat them as a snapshot.
	Stale             bool             `json:"stale"`
	StaleAfterSeconds int              `json:"stale_after_seconds"`
	Devices           []DeviceOverview `json:"devices"`
}

type DeviceOverview struct {
	Device Device `json:"device"`
	State  State  `json:"state"`
	// Since is the start of the open window: when the device entered this state, which is
	// also the last thing the pipeline heard about it. Events are edge-triggered, so an
	// old value means "nothing has changed", not "nothing is alive".
	Since *time.Time `json:"since"`
	// Ranges holds one entry per lookback, in Lookbacks order. Empty for a device the
	// pipeline has no aggregation rows for.
	Ranges []RangeUptime `json:"ranges"`
}

type RangeUptime struct {
	Range  Lookback `json:"range"`
	Uptime float64  `json:"uptime"` // percentage, 0-100
	// RangeStart is floored at the device's first event, so a young device reports its
	// real history instead of ~0%. Read it rather than assuming now - lookback.
	RangeStart time.Time `json:"range_start"`
	RangeEnd   time.Time `json:"range_end"`
}

// WindowsQuery is a request for state changes over an arbitrary range.
type WindowsQuery struct {
	Device *Device
	From   time.Time
	To     time.Time
	Limit  int
}

// WindowsReport answers WindowsQuery: the percentage for exactly that range, computed
// from the windows rather than read off a precomputed row, plus the windows themselves.
type WindowsReport struct {
	// From and To are the range actually used, after defaults and the clamp to now.
	From    time.Time       `json:"from"`
	To      time.Time       `json:"to"`
	Devices []DeviceWindows `json:"devices"`
}

type DeviceWindows struct {
	Device Device `json:"device"`
	// Uptime is up / (up + down) over the covered part of the range, not over the whole
	// range: before a device's first event there is nothing to call up or down, and
	// dividing by the full range would report the gap as downtime. Null when no window
	// overlaps at all.
	Uptime      *float64 `json:"uptime"`
	UpSeconds   float64  `json:"up_seconds"`
	DownSeconds float64  `json:"down_seconds"`
	// Outages counts the down windows overlapping the range.
	Outages int `json:"outages"`
	// CoveredFrom/CoveredTo bound the part of the range the windows actually span.
	CoveredFrom *time.Time `json:"covered_from"`
	CoveredTo   *time.Time `json:"covered_to"`
	// Windows are newest first, so a truncated list keeps the recent history. The
	// percentages above are computed in the database over every overlapping window and
	// are unaffected by the limit.
	Windows   []Window `json:"windows"`
	Truncated bool     `json:"truncated"`
}

type Window struct {
	State     State     `json:"state"`
	StartTime time.Time `json:"start_time"`
	// EndTime is null for the open window: the state the device is in now.
	EndTime *time.Time `json:"end_time"`
	// Seconds is the part of the window inside the queried range, with an open window
	// counted up to To — the pipeline's own assumption that the latest known state
	// persists.
	Seconds float64 `json:"seconds"`
}

// --- rows as the pipeline stores them ---

// CurrentState is the open window of one device.
type CurrentState struct {
	Device Device
	State  State
	Since  time.Time
}

// Aggregation is one row of marts.uptime_aggregations.
type Aggregation struct {
	Device     Device
	Range      Lookback
	RangeStart time.Time
	RangeEnd   time.Time
	Uptime     float64
}

// RangeStat is the clipped up/down split of one device over a queried range.
type RangeStat struct {
	Device      Device
	UpSeconds   float64
	DownSeconds float64
	Outages     int
	CoveredFrom time.Time
	CoveredTo   time.Time
}

// WindowRow is one row of marts.uptime_windows.
type WindowRow struct {
	Device    Device
	State     State
	StartTime time.Time
	EndTime   *time.Time
}
