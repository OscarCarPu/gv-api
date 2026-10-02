package lights

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Driver applies commands to a bulb. An unreachable bulb is a State with Online false and
// Error set, not an error, so one dead bulb cannot fail a request covering several.
type Driver interface {
	Kind() string
	GetState(ctx context.Context, light Light) State
	Apply(ctx context.Context, light Light, cmd Command) State
	// Discover does return an error: "no adapter" is the whole answer for a scan.
	Discover(ctx context.Context, window time.Duration) ([]Discovered, error)
}

/*
BlueZDriver talks to the bulbs over Bluetooth, through BlueZ on this host. It owns three
things the hardware forces on it:

  - One conversation per bulb: two overlapping GATT writes to one peripheral tend to fail
    both, so a per-address lock makes that structural.
  - Last known values: not every bulb can be read, and a readable one answers nothing when
    written a value it already holds, so replies are that record plus what the bulb confirmed.
  - Letting go: these lamps accept a single central, so a bulb untouched for idleDisconnect
    is dropped rather than locking out its own remote.

A cold call is slow and legitimately so: when BlueZ has dropped an unbonded bulb's object it
must rediscover it (~8s) before it can even connect, and a full read is three round-trips
after that. Warm calls return in well under a second.
*/
type BlueZDriver struct {
	gatt           gatt
	idleDisconnect time.Duration

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per BLE address
	known map[string]State       // per light id, last values seen or set
	used  map[string]time.Time   // per BLE address, when it was last talked to

	stop     chan struct{}
	stopOnce sync.Once
}

// NewBlueZDriver builds a driver over the given adapter (empty means hci0). It opens nothing
// until first use, so the API starts on hosts without Bluetooth.
func NewBlueZDriver(adapter string, connectTimeout, idleDisconnect time.Duration) *BlueZDriver {
	return newDriver(newBluezGATT(adapter, connectTimeout), idleDisconnect)
}

func newDriver(g gatt, idleDisconnect time.Duration) *BlueZDriver {
	if idleDisconnect <= 0 {
		idleDisconnect = 90 * time.Second
	}
	d := &BlueZDriver{
		gatt:           g,
		idleDisconnect: idleDisconnect,
		locks:          map[string]*sync.Mutex{},
		known:          map[string]State{},
		used:           map[string]time.Time{},
		stop:           make(chan struct{}),
	}
	go d.reapIdle()
	return d
}

func (d *BlueZDriver) Kind() string { return "bluez" }

func (d *BlueZDriver) Close() error {
	d.stopOnce.Do(func() { close(d.stop) })
	return d.gatt.Close()
}

func (d *BlueZDriver) GetState(ctx context.Context, light Light) State {
	proto, ok := protocolFor(light.Protocol)
	if !ok {
		return d.failed(light, "no protocol %q configured for this bulb", light.Protocol)
	}

	unlock := d.lockBulb(light.Address)
	defer unlock()

	if err := d.connect(ctx, light); err != nil {
		return d.offline(light, err)
	}
	if !proto.Readable() {
		return d.online(light)
	}

	values, err := proto.Read(ctx, d.gatt, light)
	if err != nil {
		d.drop(light.Address)
		return d.offline(light, err)
	}
	if values.empty() {
		// Connected but silent on every query: online with the last known values.
		slog.Debug("bulb answered no queries", "light", light.ID)
		return d.online(light)
	}
	d.remember(light, values)
	return d.online(light)
}

func (d *BlueZDriver) Apply(ctx context.Context, light Light, cmd Command) State {
	proto, ok := protocolFor(light.Protocol)
	if !ok {
		return d.failed(light, "no protocol %q configured for this bulb", light.Protocol)
	}

	unlock := d.lockBulb(light.Address)
	defer unlock()

	if err := d.connect(ctx, light); err != nil {
		return d.offline(light, err)
	}

	if err := applyCommand(ctx, proto, d.gatt, light, cmd); err != nil {
		if errors.Is(err, errUnsupported) {
			// The model lacks this capability; the bulb itself is fine.
			state := d.online(light)
			state.Error = capabilityMessage(cmd)
			return state
		}
		d.drop(light.Address)
		return d.offline(light, err)
	}

	d.applied(light, cmd)
	return d.online(light)
}

/*
Discover scans for bulbs in range: devices that advertise a supported family's service.
Everything else the radio heard (watches, TVs, tags) is dropped.

Scanning and connecting share one radio, and BlueZ slows every in-flight connection while
discovery is running. That is accepted rather than locked around: a scan is a person standing
in front of the app adding a lamp, which is not the moment to be optimising a poll.
*/
func (d *BlueZDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	found, err := d.gatt.Scan(ctx, window)
	if err != nil {
		slog.Warn("bulb scan failed", "error", err)
		return nil, err
	}
	bulbs := make([]Discovered, 0, len(found))
	for _, device := range found {
		if isBulb(device) {
			bulbs = append(bulbs, device)
		}
	}
	slog.Debug("bulb scan finished", "heard", len(found), "bulbs", len(bulbs))
	return bulbs, nil
}

type backgroundKey struct{}

// withBackground marks work nobody is waiting on, such as the poller, so it does not keep
// bulbs connected and lock out their physical remote.
func withBackground(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

func isBackground(ctx context.Context) bool {
	background, _ := ctx.Value(backgroundKey{}).(bool)
	return background
}

// connect brings the bulb up and stamps it as used so the idle sweep leaves it alone.
// Background connects are not stamped.
func (d *BlueZDriver) connect(ctx context.Context, light Light) error {
	if light.Address == "" {
		return errors.New("bulb has no address")
	}
	if err := d.gatt.Connect(ctx, light.Address); err != nil {
		return err
	}
	d.mu.Lock()
	if !isBackground(ctx) {
		d.used[light.Address] = time.Now()
	} else if _, inUse := d.used[light.Address]; !inUse {
		d.used[light.Address] = time.Now().Add(-d.idleDisconnect)
	}
	d.mu.Unlock()
	return nil
}

func (d *BlueZDriver) drop(address string) {
	d.mu.Lock()
	delete(d.used, address)
	d.mu.Unlock()
	d.gatt.Disconnect(address)
}

/*
reapIdle releases bulbs nobody has used lately.

Reads a person is waiting on count as use, so a client forcing reads faster than
idleDisconnect keeps its bulbs connected. The service's own background poll does not (see
withBackground), and ordinary client polling is answered from its cache, so an open Lights tab
no longer holds the links.
*/
func (d *BlueZDriver) reapIdle() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			for _, address := range d.idleAddresses() {
				lock := d.bulbLock(address)
				if !lock.TryLock() {
					continue // in use right now; next sweep will get it
				}
				d.mu.Lock()
				delete(d.used, address)
				d.mu.Unlock()
				d.gatt.Disconnect(address)
				lock.Unlock()
				slog.Debug("bulb idle, disconnected", "address", address)
			}
		}
	}
}

func (d *BlueZDriver) idleAddresses() []string {
	cutoff := time.Now().Add(-d.idleDisconnect)

	d.mu.Lock()
	defer d.mu.Unlock()
	var idle []string
	for address, used := range d.used {
		if used.Before(cutoff) {
			idle = append(idle, address)
		}
	}
	return idle
}

func (d *BlueZDriver) bulbLock(address string) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	lock, ok := d.locks[address]
	if !ok {
		lock = &sync.Mutex{}
		d.locks[address] = lock
	}
	return lock
}

func (d *BlueZDriver) lockBulb(address string) func() {
	lock := d.bulbLock(address)
	lock.Lock()
	return lock.Unlock
}

// baseline returns what we last knew about a bulb. Callers hold the bulb's lock.
func (d *BlueZDriver) baseline(light Light) State {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.known[light.ID]
	if !ok {
		state = offlineState(light, "", time.Now().UnixMilli())
		state.Brightness = 60
		state.ColorTemp = light.MinColorTemp
	}
	state.ID = light.ID
	state.Name = light.Name
	state.Model = light.Model
	state.SupportsColor = light.SupportsColor
	state.SupportsColorTemp = light.SupportsColorTemp
	state.MinColorTemp = light.MinColorTemp
	state.MaxColorTemp = light.MaxColorTemp
	state.Error = ""
	state.UpdatedAt = time.Now().UnixMilli()
	return state
}

func (d *BlueZDriver) store(state State) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.known[state.ID] = state
}

func (d *BlueZDriver) remember(light Light, values readback) {
	state := d.baseline(light)
	if values.Power != nil {
		state.Power = *values.Power
	}
	if values.Brightness != nil {
		state.Brightness = *values.Brightness
	}
	if values.ColorTemp != nil {
		state.ColorTemp = *values.ColorTemp
	}
	if values.Mode != "" {
		state.Mode = values.Mode
	}
	d.store(state)
}

/*
applied records what a command changed.

It records only that, and deliberately does not infer that setting brightness or colour turns
the bulb on: those are separate frames on the hardware, and a dimmed bulb that is off stays
off. Inferring it made the UI report "on" while the room stayed dark — and, worse, made "All
on" a no-op, because every bulb already looked on.
*/
func (d *BlueZDriver) applied(light Light, cmd Command) {
	state := d.baseline(light)
	switch cmd.Type {
	case CommandPower:
		state.Power = *cmd.On
	case CommandBrightness:
		state.Brightness = float64(clampInt(*cmd.Value, 0, 100))
	case CommandColor:
		state.Color = RGB{
			R: clampInt(cmd.Color.R, 0, 255),
			G: clampInt(cmd.Color.G, 0, 255),
			B: clampInt(cmd.Color.B, 0, 255),
		}
		state.Mode = "color"
	case CommandColorTemp:
		state.ColorTemp = float64(clampInt(*cmd.Kelvin, int(light.MinColorTemp), int(light.MaxColorTemp)))
		state.Mode = "white"
	}
	d.store(state)
}

func (d *BlueZDriver) online(light Light) State {
	state := d.baseline(light)
	state.Online = true
	d.store(state)
	return state
}

/*
offline reports a bulb we could not talk to, keeping its last known settings.

The message is deliberately vague about the cause. BlueZ's own errors carry the D-Bus object
path, which embeds the bulb's MAC — handing that to every client is both meaningless to a
person and needless exposure of the house's hardware. Clients get the meaning; the detail
stays in the log.
*/
func (d *BlueZDriver) offline(light Light, err error) State {
	slog.Warn("bulb unreachable", "light", light.ID, "error", err)

	// Report the attempt without recording it.
	state := d.baseline(light)
	state.Online = false
	state.Error = bleErrorMessage(err)
	return state
}

func (d *BlueZDriver) failed(light Light, format string, args ...any) State {
	return offlineState(light, fmt.Sprintf(format, args...), time.Now().UnixMilli())
}

func bleErrorMessage(err error) string {
	switch {
	case errors.Is(err, errNoBluetooth):
		return "no Bluetooth on this host"
	case errors.Is(err, errBulbBusy):
		return "bulb busy — another app may be connected to it"
	case errors.Is(err, errBulbNotFound):
		return "bulb not found — is it powered and in range? If so, switch it off and on"
	case errors.Is(err, errNoServices):
		return "bulb connected but never answered"
	case errors.Is(err, errNoCharUUID):
		return "bulb does not have the expected controls"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "bulb did not answer in time"
	default:
		return "bluetooth error"
	}
}

// capabilityMessage explains a command the hardware cannot take, in the user's terms.
func capabilityMessage(cmd Command) string {
	switch cmd.Type {
	case CommandColor:
		return "this bulb is tunable white only"
	case CommandColorTemp:
		return "this bulb has a fixed colour temperature"
	default:
		return "this bulb does not support that"
	}
}

// MockDriver keeps bulb state in memory and touches no hardware.
type MockDriver struct {
	mu     sync.Mutex
	states map[string]State
}

func NewMockDriver() *MockDriver {
	return &MockDriver{states: map[string]State{}}
}

func (d *MockDriver) Kind() string { return "mock" }

/*
Discover invents a couple of bulbs.

The add-a-bulb screen is the one part of this section that cannot be exercised without
hardware, so the mock answers it too: the flow — scan, pick, name, save — is developable on a
laptop, and only the last hop to a real lamp is not.
*/
func (d *MockDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	select {
	case <-time.After(min(window, 2*time.Second)):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []Discovered{
		{Address: "00:11:22:33:44:55", Name: "Mock CCT bulb", RSSI: -52},
		{Address: "00:11:22:33:44:66", Name: "Mock lamp", RSSI: -78},
	}, nil
}

func (d *MockDriver) GetState(_ context.Context, light Light) State {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current(light)
}

func (d *MockDriver) Apply(_ context.Context, light Light, cmd Command) State {
	d.mu.Lock()
	defer d.mu.Unlock()

	state := d.current(light)
	switch cmd.Type {
	case CommandPower:
		state.Power = *cmd.On
	case CommandBrightness:
		state.Brightness = float64(clampInt(*cmd.Value, 0, 100))
	case CommandColor:
		state.Color = RGB{
			R: clampInt(cmd.Color.R, 0, 255),
			G: clampInt(cmd.Color.G, 0, 255),
			B: clampInt(cmd.Color.B, 0, 255),
		}
		state.Mode = "color"
	case CommandColorTemp:
		state.ColorTemp = float64(clampInt(*cmd.Kelvin, int(light.MinColorTemp), int(light.MaxColorTemp)))
		state.Mode = "white"
	}
	// Power is never inferred from brightness or colour: on the hardware they are separate frames.
	state.UpdatedAt = time.Now().UnixMilli()
	d.states[light.ID] = state
	return state
}

func (d *MockDriver) current(light Light) State {
	state, ok := d.states[light.ID]
	if !ok {
		state = offlineState(light, "", time.Now().UnixMilli())
		state.Brightness = 60
		state.ColorTemp = (light.MinColorTemp + light.MaxColorTemp) / 2
		d.states[light.ID] = state
	}
	state.Online = true
	state.Name = light.Name
	state.Model = light.Model
	state.SupportsColor = light.SupportsColor
	state.SupportsColorTemp = light.SupportsColorTemp
	state.MinColorTemp = light.MinColorTemp
	state.MaxColorTemp = light.MaxColorTemp
	state.Error = ""
	return state
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
