package lights

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

/*
BlueZ transport for the bulbs, over D-Bus.

Raw D-Bus rather than a BLE library: BlueZ already does the hard parts (scanning, connecting,
GATT), it is reachable over a socket the container can be handed, and speaking to it needs no
CGO and no privileged capabilities. The whole surface a bulb needs is four calls.

# What the container needs

Only the host's system bus socket:

	volumes:
	  - /run/dbus/system_bus_socket:/run/dbus/system_bus_socket

No host networking, no NET_ADMIN, and no root: BlueZ's default D-Bus policy lets any local uid
send to org.bluez (owning the name and registering agents is what is restricted, and we do
neither).

# Notifications are why this is stateful

BlueZ tears down a StartNotify subscription the moment the D-Bus client that asked for it
drops off the bus, and it delivers values as PropertiesChanged signals rather than as replies.
So a connection is kept for the process's lifetime with a goroutine pumping signals — a
request-scoped bus connection would report success and then never be told anything.
*/

const (
	bluezName    = "org.bluez"
	adapterIface = "org.bluez.Adapter1"
	deviceIface  = "org.bluez.Device1"
	charIface    = "org.bluez.GattCharacteristic1"
	propsIface   = "org.freedesktop.DBus.Properties"

	// How long to scan when BlueZ has no object for a bulb at all. These bulbs are never
	// bonded, so BlueZ forgets them some time after they disconnect and a scan is the only
	// thing that brings the object back.
	discoveryWindow = 8 * time.Second
	// How long to wait for the GATT table after Connect returns. Nothing can be written
	// before it resolves.
	servicesTimeout = 15 * time.Second
	// Pause between connect attempts. The first try on these bulbs routinely dies partway
	// through service discovery; the next one almost always works.
	connectRetries = 3
	retryPause     = 800 * time.Millisecond

	// How long to wait for Powered to come back true after a reset. Measured on the
	// deploy host's adapter, which is unusually slow to act on its own StartDiscovery too.
	adapterResetSettle = 3 * time.Second
)

// errQuiet is not used as a failure: a bulb that answers nothing to a query is normal (these
// lamps stay silent when written a value they already hold), so Query reports it as no bytes
// rather than as an error.

// gatt is the Bluetooth surface a bulb protocol needs.
//
// It is an interface so protocols and the driver can be tested without a radio — the fake in
// the tests implements these four methods and nothing else.
type gatt interface {
	// Connect brings the bulb up and waits for its GATT table. Safe to call when already
	// connected, which is the common case.
	Connect(ctx context.Context, address string) error
	// Write sends a payload to one characteristic, addressed by UUID.
	Write(ctx context.Context, address, charUUID string, payload []byte) error
	// Query writes a frame and waits for the answering notification. It returns nil bytes
	// and no error when the bulb stays quiet, which is a normal outcome rather than a fault.
	Query(ctx context.Context, address, writeUUID, notifyUUID string, payload []byte, timeout time.Duration) ([]byte, error)
	// Scan looks for bulbs in range. This is how a bulb gets added: a person cannot type a
	// BLE address they have no way of knowing.
	Scan(ctx context.Context, window time.Duration) ([]Discovered, error)
	// Disconnect drops the link and any state cached for it. Errors are not worth reporting:
	// the point is that the next call reconnects.
	Disconnect(address string)
	// Close releases the bus connection.
	Close() error
}

// Errors the driver maps to user-facing messages. They are compared with errors.Is, so the
// D-Bus detail (which carries the bulb's address in the object path) stays in the logs.
var (
	errNoBluetooth  = errors.New("bluetooth unavailable on this host")
	errBulbNotFound = errors.New("bulb not found")
	errBulbBusy     = errors.New("bulb refused the connection")
	errNoServices   = errors.New("gatt services never resolved")
	errNoCharUUID   = errors.New("characteristic not found")
)

// bluezGATT is the real transport. The zero value is not usable; see newBluezGATT.
type bluezGATT struct {
	adapter        string
	connectTimeout time.Duration

	mu    sync.Mutex
	conn  *dbus.Conn
	chars map[string]dbus.ObjectPath // "address|uuid" -> characteristic path
	notes map[dbus.ObjectPath]*notifier
	dead  bool
}

func newBluezGATT(adapter string, connectTimeout time.Duration) *bluezGATT {
	if adapter == "" {
		adapter = "hci0"
	}
	if connectTimeout <= 0 {
		connectTimeout = 20 * time.Second
	}
	return &bluezGATT{
		adapter:        adapter,
		connectTimeout: connectTimeout,
		chars:          map[string]dbus.ObjectPath{},
		notes:          map[dbus.ObjectPath]*notifier{},
	}
}

// bus connects on first use and keeps the connection.
//
// Deliberately lazy: the API must start on a host with no Bluetooth at all (a laptop, CI, a
// server before the dongle arrives) and report it per bulb, rather than refusing to boot over
// a section of the house nobody may be looking at.
func (g *bluezGATT) bus() (*dbus.Conn, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.dead {
		return nil, errNoBluetooth
	}
	if g.conn != nil {
		return g.conn, nil
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		// Worth a log rather than only a per-bulb card: this is the difference between "the
		// lamp is out of range" and "this host cannot do Bluetooth at all", and the causes
		// (socket not mounted, bluetoothd down) are invisible from the UI.
		slog.Warn("cannot reach the system bus — is /run/dbus/system_bus_socket mounted?", "error", err)
		return nil, fmt.Errorf("%w: %w", errNoBluetooth, err)
	}

	signals := make(chan *dbus.Signal, 32)
	conn.Signal(signals)
	go g.pump(signals)

	g.conn = conn
	return conn, nil
}

// pump turns PropertiesChanged signals into notification values. It owns no locks of its own
// beyond the map lookup, so a slow reader cannot stall D-Bus dispatch.
func (g *bluezGATT) pump(signals chan *dbus.Signal) {
	for sig := range signals {
		if sig.Name != propsIface+".PropertiesChanged" || len(sig.Body) < 2 {
			continue
		}
		if iface, _ := sig.Body[0].(string); iface != charIface {
			continue
		}
		changed, ok := sig.Body[1].(map[string]dbus.Variant)
		if !ok {
			continue
		}
		value, ok := changed["Value"]
		if !ok {
			continue
		}
		bytes, ok := value.Value().([]byte)
		if !ok {
			continue
		}

		g.mu.Lock()
		note := g.notes[sig.Path]
		g.mu.Unlock()
		if note != nil {
			note.deliver(bytes)
		}
	}
}

func (g *bluezGATT) devicePath(address string) dbus.ObjectPath {
	mac := strings.ReplaceAll(strings.ToUpper(address), ":", "_")
	return dbus.ObjectPath("/org/bluez/" + g.adapter + "/dev_" + mac)
}

func (g *bluezGATT) adapterPath() dbus.ObjectPath {
	return dbus.ObjectPath("/org/bluez/" + g.adapter)
}

// --- connection ----------------------------------------------------------------------

func (g *bluezGATT) Connect(ctx context.Context, address string) error {
	// Bound the whole thing rather than each step: three attempts of scan-then-wait can
	// otherwise outlast the HTTP request that asked for it, and a caller waiting a minute
	// for a lamp has already given up.
	ctx, cancel := context.WithTimeout(ctx, g.connectTimeout)
	defer cancel()

	var last error
	for attempt := range connectRetries {
		if ctx.Err() != nil {
			return joinTimeout(ctx, last)
		}

		err := g.attemptConnect(ctx, address)
		if err == nil {
			return nil
		}
		last = err
		slog.Debug("bulb connect attempt failed", "attempt", attempt+1, "error", err)

		if errors.Is(err, errBulbNotFound) {
			// BlueZ has no object for this address. Expected rather than exceptional, and a
			// scan is what brings it back — so discover on every attempt, not just the first.
			g.discoverBriefly(ctx)
			continue
		}
		if errors.Is(err, errNoBluetooth) {
			return err // no adapter or no bus; retrying changes nothing
		}
		sleepCtx(ctx, retryPause)
	}
	return joinTimeout(ctx, last)
}

func (g *bluezGATT) attemptConnect(ctx context.Context, address string) error {
	conn, err := g.bus()
	if err != nil {
		return err
	}
	device := conn.Object(bluezName, g.devicePath(address))

	connected, err := boolProp(device, deviceIface, "Connected")
	if err != nil {
		return classifyDBus(err)
	}
	if !connected {
		if call := device.CallWithContext(ctx, deviceIface+".Connect", 0); call.Err != nil {
			return classifyDBus(call.Err)
		}
	}
	if !g.waitForServices(ctx, device) {
		return errNoServices
	}
	return nil
}

func (g *bluezGATT) waitForServices(ctx context.Context, device dbus.BusObject) bool {
	deadline := time.Now().Add(servicesTimeout)
	for time.Now().Before(deadline) {
		resolved, err := boolProp(device, deviceIface, "ServicesResolved")
		if err != nil {
			return false
		}
		if resolved {
			return true
		}
		if !sleepCtx(ctx, 250*time.Millisecond) {
			return false
		}
	}
	return false
}

// discoverBriefly nudges BlueZ into noticing a bulb it has no object for yet.
func (g *bluezGATT) discoverBriefly(ctx context.Context) {
	conn, err := g.bus()
	if err != nil {
		return
	}
	adapter := conn.Object(bluezName, g.adapterPath())
	g.ensureNotStuck(ctx, adapter)
	if call := adapter.CallWithContext(ctx, adapterIface+".StartDiscovery", 0); call.Err != nil {
		return // already discovering, or no adapter — the next connect reports it
	}
	sleepCtx(ctx, discoveryWindow)
	g.stopDiscovery(ctx, adapter)
}

/*
ensureNotStuck clears a Discovering flag nobody is going to turn off.

Measured against the deploy host's adapter: StartDiscovery can take 5-6s to actually take
effect, and once it has, StopDiscovery still answers "no discovery started" — the radio is
scanning with nothing left able to stop it, which fails every scan and connect after it the
same way. A power cycle is the one thing that reliably clears this on that hardware, so it is
the recovery path rather than a longer StopDiscovery timeout.
*/
func (g *bluezGATT) ensureNotStuck(ctx context.Context, adapter dbus.BusObject) {
	discovering, err := boolProp(adapter, adapterIface, "Discovering")
	if err != nil || !discovering {
		return
	}
	g.resetAdapter(ctx, adapter)
}

// stopDiscovery is StopDiscovery plus the same power-cycle fallback: leaving Discovering true
// after a failed stop would just hand the next caller the same stuck adapter.
func (g *bluezGATT) stopDiscovery(ctx context.Context, adapter dbus.BusObject) {
	// Fresh context: the caller's may already be done, and leaving the adapter scanning
	// burns power and slows every later connect.
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = adapter.CallWithContext(stopCtx, adapterIface+".StopDiscovery", 0).Err
	g.ensureNotStuck(stopCtx, adapter)
}

// resetAdapter power-cycles the radio via its Powered property.
func (g *bluezGATT) resetAdapter(ctx context.Context, adapter dbus.BusObject) {
	_ = adapter.SetProperty(adapterIface+".Powered", false)
	sleepCtx(ctx, 1*time.Second)
	_ = adapter.SetProperty(adapterIface+".Powered", true)

	deadline := time.Now().Add(adapterResetSettle)
	for time.Now().Before(deadline) {
		if powered, err := boolProp(adapter, adapterIface, "Powered"); err == nil && powered {
			return
		}
		if !sleepCtx(ctx, 250*time.Millisecond) {
			return
		}
	}
}

/*
Scan lists what the adapter can hear.

BlueZ answers with everything it has ever seen on this adapter, not just what is advertising
right now, so the list is filtered to devices with a live RSSI. A lamp already paired to a
phone's vendor app will not appear at all — these bulbs stop advertising while another
central holds them, which is worth knowing before concluding a bulb is broken.
*/
func (g *bluezGATT) Scan(ctx context.Context, window time.Duration) ([]Discovered, error) {
	conn, err := g.bus()
	if err != nil {
		return nil, err
	}
	adapter := conn.Object(bluezName, g.adapterPath())
	g.ensureNotStuck(ctx, adapter)

	if call := adapter.CallWithContext(ctx, adapterIface+".StartDiscovery", 0); call.Err != nil {
		return nil, classifyDBus(call.Err)
	}
	sleepCtx(ctx, window)
	g.stopDiscovery(ctx, adapter)

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	call := conn.Object(bluezName, "/").CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0)
	if call.Err != nil {
		return nil, classifyDBus(call.Err)
	}
	if err := call.Store(&objects); err != nil {
		return nil, err
	}

	prefix := string(g.adapterPath()) + "/dev_"
	found := make([]Discovered, 0, 8)
	for path, interfaces := range objects {
		if !strings.HasPrefix(string(path), prefix) {
			continue
		}
		device, ok := interfaces[deviceIface]
		if !ok {
			continue
		}
		address, _ := device["Address"].Value().(string)
		if address == "" {
			continue
		}
		name, _ := device["Name"].Value().(string)
		if name == "" {
			name, _ = device["Alias"].Value().(string)
		}
		rssi, hasRSSI := device["RSSI"].Value().(int16)
		if !hasRSSI {
			continue // remembered from an earlier session, not in range now
		}
		found = append(found, Discovered{Address: address, Name: name, RSSI: int(rssi)})
	}

	// Strongest first: the bulb someone is standing next to is the one they mean.
	sort.Slice(found, func(i, j int) bool { return found[i].RSSI > found[j].RSSI })
	return found, nil
}

func (g *bluezGATT) Disconnect(address string) {
	g.forget(address)

	conn, err := g.bus()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Object(bluezName, g.devicePath(address)).
		CallWithContext(ctx, deviceIface+".Disconnect", 0).Err
}

// forget drops everything cached about a link: characteristic paths are only valid while the
// device object exists, and a stale one is a write into nowhere.
func (g *bluezGATT) forget(address string) {
	prefix := string(g.devicePath(address)) + "/"

	g.mu.Lock()
	defer g.mu.Unlock()
	for key := range g.chars {
		if strings.HasPrefix(key, address+"|") {
			delete(g.chars, key)
		}
	}
	for path := range g.notes {
		if strings.HasPrefix(string(path), prefix) {
			delete(g.notes, path)
		}
	}
}

func (g *bluezGATT) Close() error {
	g.mu.Lock()
	conn := g.conn
	g.conn = nil
	g.dead = true
	g.mu.Unlock()

	if conn == nil {
		return nil
	}
	return conn.Close()
}

// --- characteristics -----------------------------------------------------------------

/*
characteristic resolves a characteristic's object path by UUID.

Deliberately not hardcoded: BlueZ's serviceXXXX/charXXXX numbering is a cache artefact,
stable most of the time and silently different after an adapter reset. Looking it up by UUID
costs one GetManagedObjects and never rots; the result is cached until the link drops.
*/
func (g *bluezGATT) characteristic(ctx context.Context, address, uuid string) (dbus.ObjectPath, error) {
	key := address + "|" + strings.ToLower(uuid)

	g.mu.Lock()
	if path, ok := g.chars[key]; ok {
		g.mu.Unlock()
		return path, nil
	}
	g.mu.Unlock()

	conn, err := g.bus()
	if err != nil {
		return "", err
	}

	var objects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	call := conn.Object(bluezName, "/").CallWithContext(ctx, "org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0)
	if call.Err != nil {
		return "", classifyDBus(call.Err)
	}
	if err := call.Store(&objects); err != nil {
		return "", err
	}

	prefix := string(g.devicePath(address)) + "/"
	want := strings.ToLower(uuid)
	for path, interfaces := range objects {
		if !strings.HasPrefix(string(path), prefix) {
			continue
		}
		props, ok := interfaces[charIface]
		if !ok {
			continue
		}
		if got, _ := props["UUID"].Value().(string); strings.EqualFold(got, want) {
			g.mu.Lock()
			g.chars[key] = path
			g.mu.Unlock()
			return path, nil
		}
	}
	return "", fmt.Errorf("%w: %s", errNoCharUUID, uuid)
}

/*
Write sends a payload to one characteristic.

The write type is left to BlueZ on purpose. Naming one the characteristic does not support
makes BlueZ reject the call outright — the Lexman's a101 advertises plain write only — and
letting BlueZ pick works for every bulb tried so far.
*/
func (g *bluezGATT) Write(ctx context.Context, address, charUUID string, payload []byte) error {
	path, err := g.characteristic(ctx, address, charUUID)
	if err != nil {
		return err
	}
	conn, err := g.bus()
	if err != nil {
		return err
	}
	call := conn.Object(bluezName, path).CallWithContext(
		ctx, charIface+".WriteValue", 0, payload, map[string]dbus.Variant{},
	)
	if call.Err != nil {
		return classifyDBus(call.Err)
	}
	return nil
}

func (g *bluezGATT) Query(
	ctx context.Context,
	address, writeUUID, notifyUUID string,
	payload []byte,
	timeout time.Duration,
) ([]byte, error) {
	notifyPath, err := g.characteristic(ctx, address, notifyUUID)
	if err != nil {
		return nil, err
	}
	note, err := g.subscribe(ctx, notifyPath)
	if err != nil {
		return nil, err
	}

	// Arm before writing so a notification left over from an earlier query cannot satisfy
	// this one.
	ready := note.arm()
	if err := g.Write(ctx, address, writeUUID, payload); err != nil {
		return nil, err
	}

	select {
	case <-ready:
		return note.value(), nil
	case <-time.After(timeout):
		// Silence is a real answer here: these bulbs say nothing when written a value they
		// already hold. The caller falls back to what it last knew.
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// subscribe turns on notifications for a characteristic, once per link.
func (g *bluezGATT) subscribe(ctx context.Context, path dbus.ObjectPath) (*notifier, error) {
	g.mu.Lock()
	if note, ok := g.notes[path]; ok {
		g.mu.Unlock()
		return note, nil
	}
	g.mu.Unlock()

	conn, err := g.bus()
	if err != nil {
		return nil, err
	}
	object := conn.Object(bluezName, path)

	notifying, err := boolProp(object, charIface, "Notifying")
	if err != nil {
		return nil, classifyDBus(err)
	}
	if !notifying {
		if call := object.CallWithContext(ctx, charIface+".StartNotify", 0); call.Err != nil {
			return nil, classifyDBus(call.Err)
		}
	}
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(propsIface),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		return nil, err
	}

	note := newNotifier()
	g.mu.Lock()
	// Another goroutine may have won the race while we were on the bus; one notifier per
	// path or a delivery could land on an object nobody is waiting on.
	if existing, ok := g.notes[path]; ok {
		note = existing
	} else {
		g.notes[path] = note
	}
	g.mu.Unlock()
	return note, nil
}

// notifier holds the latest value seen on one characteristic and lets a reader wait for the
// next one. Deliveries arrive on the D-Bus pump goroutine; readers are HTTP handlers.
type notifier struct {
	mu    sync.Mutex
	last  []byte
	ready chan struct{}
}

func newNotifier() *notifier { return &notifier{ready: make(chan struct{})} }

// arm clears the previous value and returns the channel closed by the next delivery.
func (n *notifier) arm() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.last = nil
	n.ready = make(chan struct{})
	return n.ready
}

func (n *notifier) deliver(value []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.last = value
	select {
	case <-n.ready: // already closed by an earlier delivery in this window
	default:
		close(n.ready)
	}
}

func (n *notifier) value() []byte {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.last
}

// --- small helpers -------------------------------------------------------------------

func boolProp(object dbus.BusObject, iface, name string) (bool, error) {
	variant, err := object.GetProperty(iface + "." + name)
	if err != nil {
		return false, err
	}
	value, _ := variant.Value().(bool)
	return value, nil
}

// classifyDBus maps BlueZ's error names onto the handful of causes worth telling a user
// apart. The original is wrapped, so logs keep the detail.
func classifyDBus(err error) error {
	var dbusErr dbus.Error
	if !errors.As(err, &dbusErr) {
		return err
	}
	message := strings.ToLower(dbusErr.Error())

	switch dbusErr.Name {
	case "org.freedesktop.DBus.Error.UnknownObject", "org.freedesktop.DBus.Error.UnknownMethod":
		return fmt.Errorf("%w: %w", errBulbNotFound, err)
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.bluez.Error.NotReady":
		// bluetoothd is not running, or the adapter is off.
		return fmt.Errorf("%w: %w", errNoBluetooth, err)
	case "org.bluez.Error.DoesNotExist":
		return fmt.Errorf("%w: %w", errBulbNotFound, err)
	case "org.bluez.Error.InProgress", "org.bluez.Error.AlreadyConnected":
		return fmt.Errorf("%w: %w", errBulbBusy, err)
	}

	switch {
	case strings.Contains(message, "does not exist"), strings.Contains(message, "no such device"):
		return fmt.Errorf("%w: %w", errBulbNotFound, err)
	case strings.Contains(message, "abort-by-local"), strings.Contains(message, "connection refused"),
		strings.Contains(message, "br-connection-page-timeout"), strings.Contains(message, "not connected"):
		// One central at a time: the vendor app on a phone will hold the bulb and it then
		// stops advertising entirely.
		return fmt.Errorf("%w: %w", errBulbBusy, err)
	}
	return err
}

// sleepCtx waits unless the context ends first; false means it ended.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// joinTimeout prefers the specific failure over "deadline exceeded", which says nothing about
// the bulb.
func joinTimeout(ctx context.Context, last error) error {
	if last != nil {
		return last
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return errBulbNotFound
}
