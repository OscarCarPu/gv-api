package lights

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Driver applies commands to a bulb.
//
// Implementations must not return an error for an unreachable bulb: they return a State with
// Online false and Error set, so one dead bulb cannot fail a request covering several. An
// error is reserved for the driver itself being unusable.
type Driver interface {
	Kind() string
	GetState(ctx context.Context, light Light) State
	Apply(ctx context.Context, light Light, cmd Command) State
	// Discover lists bulbs in range, so adding one is picking it off a list rather than
	// typing a BLE address. Unlike the two above, this one does return an error: it is a
	// deliberate action with a person waiting on the answer, and "no adapter" is the whole
	// story rather than one card's worth of it.
	Discover(ctx context.Context, window time.Duration) ([]Discovered, error)
}

// --- BlueZ driver --------------------------------------------------------------------

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

// NewBlueZDriver builds a driver over the given adapter (empty means hci0). A zero timeout or
// idle window uses the defaults.
//
// It opens nothing: the D-Bus connection is made on first use, so the API still starts on a
// host with no Bluetooth and reports the trouble per bulb instead of refusing to boot.
func NewBlueZDriver(adapter string, connectTimeout, idleDisconnect time.Duration) *BlueZDriver {
	return newDriver(newBluezGATT(adapter, connectTimeout), idleDisconnect)
}

// newDriver is the seam the tests use, with a fake gatt in place of the radio.
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

// Close stops the idle sweep and releases the bus. Bulbs are left connected: the host is
// shutting down anyway, and BlueZ drops the links with it.
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
		// Nothing to ask: the bulb only takes orders. What we last set is the best answer
		// there is, and it is a true one as long as nobody used the physical remote.
		return d.online(light)
	}

	values, err := proto.Read(ctx, d.gatt, light)
	if err != nil {
		d.drop(light.Address)
		return d.offline(light, err)
	}
	if values.empty() {
		// The bulb stayed quiet on every query. It is there — the connection worked — so
		// report it online with what we last knew rather than a blank card.
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
			// A capability this model does not have. The bulb is fine, so it stays online and
			// keeps its state; only the card says why nothing happened.
			state := d.online(light)
			state.Error = capabilityMessage(cmd)
			return state
		}
		// Drop the link so the next call reconnects: a half-dead one never recovers.
		d.drop(light.Address)
		return d.offline(light, err)
	}

	d.applied(light, cmd)
	return d.online(light)
}

/*
Discover scans for bulbs in range.

Scanning and connecting share one radio, and BlueZ slows every in-flight connection while
discovery is running. That is accepted rather than locked around: a scan is a person standing
in front of the app adding a lamp, which is not the moment to be optimising a poll.
*/
func (d *BlueZDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	found, err := d.gatt.Scan(ctx, window)
	if err != nil {
		// The client only gets the summarised reason, and a scan that finds nothing is the
		// hardest thing here to diagnose from the outside.
		slog.Warn("bulb scan failed", "error", err)
		return nil, err
	}
	slog.Debug("bulb scan finished", "found", len(found))
	return found, nil
}

// connect brings the bulb up and records that it was used, which is what keeps the idle sweep
// from disconnecting a bulb mid-conversation.
func (d *BlueZDriver) connect(ctx context.Context, light Light) error {
	if light.Address == "" {
		return errors.New("bulb has no address")
	}
	if err := d.gatt.Connect(ctx, light.Address); err != nil {
		return err
	}
	d.mu.Lock()
	d.used[light.Address] = time.Now()
	d.mu.Unlock()
	return nil
}

// drop forgets a link after a failure so the next call starts clean.
func (d *BlueZDriver) drop(address string) {
	d.mu.Lock()
	delete(d.used, address)
	d.mu.Unlock()
	d.gatt.Disconnect(address)
}

/*
reapIdle releases bulbs nobody has used lately.

Worth knowing when reading polling code elsewhere: any client polling faster than
idleDisconnect keeps its bulbs connected indefinitely, because every poll is a real read.
That is the intended trade — an open Lights tab means someone is using the lights — but it is
why the remote on the wall stops working while the tab is open.
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

// --- per-bulb serialisation ----------------------------------------------------------

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

// --- last known values ---------------------------------------------------------------

// baseline returns what we last knew about a bulb, seeded from config the first time.
// Callers hold the bulb's lock.
func (d *BlueZDriver) baseline(light Light) State {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.known[light.ID]
	if !ok {
		state = offlineState(light, "", time.Now().UnixMilli())
		state.Brightness = 60
		state.ColorTemp = light.MinColorTemp
	}
	// Identity and capabilities follow config, so an env edit shows up without a restart of
	// anything but the API itself.
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

// remember folds a readback into what we know.
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

// --- replies -------------------------------------------------------------------------

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

	// baseline carries the last known settings and clears Online/Error, so this reports the
	// attempt without recording it: the bulb's settings are still whatever we last set.
	state := d.baseline(light)
	state.Online = false
	state.Error = bleErrorMessage(err)
	return state
}

// failed reports a configuration mistake rather than a hardware one.
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
		return "bulb not found — is it powered and in range?"
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

// --- mock driver ---------------------------------------------------------------------

// MockDriver keeps bulb state in memory and touches no hardware. It is the default, so the
// Domotics UI is workable in development and on any host without a Bluetooth adapter.
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
	// Take the time a real scan would, so the UI's waiting state is exercised rather than
	// skipped past.
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
	// Deliberately does not infer Power from a brightness or colour command: on the real
	// hardware those are separate frames, and a dimmed bulb that is off stays off. Guessing
	// otherwise made the UI claim "on" over a dark room.
	state.UpdatedAt = time.Now().UnixMilli()
	d.states[light.ID] = state
	return state
}

// current must be called with the lock held.
func (d *MockDriver) current(light Light) State {
	state, ok := d.states[light.ID]
	if !ok {
		state = offlineState(light, "", time.Now().UnixMilli())
		state.Brightness = 60
		state.ColorTemp = (light.MinColorTemp + light.MaxColorTemp) / 2
		d.states[light.ID] = state
	}
	// Identity and capabilities follow config, so an env edit shows up without clearing state.
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
