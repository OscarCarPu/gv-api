package uptime

import "time"

// Device is one of the two publishers: the home lab and the ESP32 that watches it.
type Device string

const (
	DeviceLab      Device = "lab"
	DeviceWatchdog Device = "watchdog"
)

var Devices = []Device{DeviceLab, DeviceWatchdog}

func ParseDevice(s string) (Device, error) {
	for _, d := range Devices {
		if string(d) == s {
			return d, nil
		}
	}
	return "", ErrUnknownDevice
}

// State is what a device was doing during a window. StateUnknown is what a device with no
// windows reads as.
type State string

const (
	StateUp      State = "up"
	StateDown    State = "down"
	StateUnknown State = "unknown"
)

// Lookback is one of the four precomputed ranges, using the pipeline's own strings.
type Lookback string

const (
	LookbackMonth       Lookback = "month"
	LookbackThreeMonths Lookback = "3 months"
	LookbackYear        Lookback = "year"
	LookbackAll         Lookback = "all"
)

var Lookbacks = []Lookback{LookbackMonth, LookbackThreeMonths, LookbackYear, LookbackAll}

// Overview is where both devices stand now, with the four precomputed percentages for each.
type Overview struct {
	// ComputedAt is when dbt last ran. Null when the pipeline has produced nothing yet.
	ComputedAt *time.Time `json:"computed_at"`
	// Stale says the percentages are older than StaleAfterSeconds.
	Stale             bool             `json:"stale"`
	StaleAfterSeconds int              `json:"stale_after_seconds"`
	Devices           []DeviceOverview `json:"devices"`
}

type DeviceOverview struct {
	Device Device `json:"device"`
	State  State  `json:"state"`
	// Since is when the device entered this state. Events are edge-triggered, so an old value means
	// nothing has changed.
	Since *time.Time `json:"since"`
	// Ranges holds one entry per lookback, in Lookbacks order.
	Ranges []RangeUptime `json:"ranges"`
}

type RangeUptime struct {
	Range  Lookback `json:"range"`
	Uptime float64  `json:"uptime"` // percentage, 0-100
	// RangeStart is floored at the device's first event, so a young device is not reported at ~0%.
	RangeStart time.Time `json:"range_start"`
	RangeEnd   time.Time `json:"range_end"`
}

type WindowsQuery struct {
	Device *Device
	From   time.Time
	To     time.Time
	Limit  int
}

// WindowsReport is the uptime for exactly the queried range, plus the windows themselves.
type WindowsReport struct {
	// From and To are the range actually used, after defaults and clamping to now.
	From    time.Time       `json:"from"`
	To      time.Time       `json:"to"`
	Devices []DeviceWindows `json:"devices"`
}

type DeviceWindows struct {
	Device Device `json:"device"`
	// Uptime is up / (up + down) over the covered part of the range, so time before the first event
	// is not counted as downtime. Null when no window overlaps.
	Uptime      *float64 `json:"uptime"`
	UpSeconds   float64  `json:"up_seconds"`
	DownSeconds float64  `json:"down_seconds"`
	Outages     int      `json:"outages"`
	// CoveredFrom/CoveredTo bound the part of the range the windows actually span.
	CoveredFrom *time.Time `json:"covered_from"`
	CoveredTo   *time.Time `json:"covered_to"`
	// Windows are newest first. The percentages above cover every window, regardless of the limit.
	Windows   []Window `json:"windows"`
	Truncated bool     `json:"truncated"`
}

type Window struct {
	State     State     `json:"state"`
	StartTime time.Time `json:"start_time"`
	// EndTime is null for the open window: the state the device is in now.
	EndTime *time.Time `json:"end_time"`
	// Seconds is the part of the window inside the range, with an open window counted up to To.
	Seconds float64 `json:"seconds"`
}

type CurrentState struct {
	Device Device
	State  State
	Since  time.Time
}

type Aggregation struct {
	Device     Device
	Range      Lookback
	RangeStart time.Time
	RangeEnd   time.Time
	Uptime     float64
}

type RangeStat struct {
	Device      Device
	UpSeconds   float64
	DownSeconds float64
	Outages     int
	CoveredFrom time.Time
	CoveredTo   time.Time
}

type WindowRow struct {
	Device    Device
	State     State
	StartTime time.Time
	EndTime   *time.Time
}
