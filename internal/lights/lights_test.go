package lights

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mustRegistry(t *testing.T, raw string) *Registry {
	t.Helper()
	return ParseRegistry(raw)
}

const oneBulb = `[{"id":"bedroom","name":"Bedroom","model":"LEXMAN","address":"AA:BB:CC:DD:EE:FF",
"protocol":"lexman","supportsColor":false,"supportsColorTemp":true,"minColorTemp":2700,"maxColorTemp":6500}]`

func TestParseRegistry(t *testing.T) {
	reg := mustRegistry(t, oneBulb)

	if got := len(reg.All()); got != 1 {
		t.Fatalf("want 1 bulb, got %d", got)
	}
	light, ok := reg.Get("bedroom")
	if !ok {
		t.Fatal("bedroom not found")
	}
	if light.Address != "AA:BB:CC:DD:EE:FF" || light.Protocol != "lexman" {
		t.Fatalf("unexpected config: %+v", light)
	}
	if *light.SupportsColor {
		t.Error("supportsColor should have been respected as false")
	}
	if *light.MinColorTemp != 2700 || *light.MaxColorTemp != 6500 {
		t.Errorf("kelvin range not carried: %v..%v", *light.MinColorTemp, *light.MaxColorTemp)
	}
}

func TestParseRegistryRejectsBadEntriesWithoutFailing(t *testing.T) {
	// A missing address, a duplicate id and outright garbage must each be skipped while the
	// good entry survives — one bad bulb should never take the section down.
	reg := mustRegistry(t, `[
		{"id":"ok","address":"AA"},
		{"id":"no-address"},
		{"address":"BB"},
		{"id":"ok","address":"CC"}
	]`)
	if got := len(reg.All()); got != 1 {
		t.Fatalf("want 1 surviving bulb, got %d", got)
	}
	if _, ok := reg.Get("ok"); !ok {
		t.Error("the valid entry should have survived")
	}
}

func TestParseRegistryInvalidJSON(t *testing.T) {
	if got := len(ParseRegistry("not json").All()); got != 0 {
		t.Errorf("want no bulbs from invalid JSON, got %d", got)
	}
	if got := len(ParseRegistry("").All()); got != 0 {
		t.Errorf("want no bulbs from empty LIGHTS, got %d", got)
	}
}

func TestPublicHidesAddress(t *testing.T) {
	// The BLE address identifies hardware in the house; it must not reach a client.
	body, err := json.Marshal(mustRegistry(t, oneBulb).Public())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"AA:BB:CC:DD:EE:FF", "address", "protocol"} {
		if contains(string(body), secret) {
			t.Errorf("public payload leaked %q: %s", secret, body)
		}
	}
}

func TestCommandValidate(t *testing.T) {
	on := true
	good := 50
	tooBig := 101
	kelvin := 3000
	silly := 99999

	valid := []Command{
		{Type: CommandPower, On: &on},
		{Type: CommandBrightness, Value: &good},
		{Type: CommandColor, Color: &RGB{R: 1, G: 2, B: 3}},
		{Type: CommandColorTemp, Kelvin: &kelvin},
	}
	for _, cmd := range valid {
		if err := cmd.Validate(); err != nil {
			t.Errorf("%s should be valid: %v", cmd.Type, err)
		}
	}

	invalid := []Command{
		{Type: CommandPower},                      // missing on
		{Type: CommandBrightness},                 // missing value
		{Type: CommandBrightness, Value: &tooBig}, // out of range
		{Type: CommandColor},                      // missing colour
		{Type: CommandColor, Color: &RGB{R: 300}}, // channel out of range
		{Type: CommandColorTemp},                  // missing kelvin
		{Type: CommandColorTemp, Kelvin: &silly},  // implausible
		{Type: "explode", On: &on},                // unknown type
	}
	for _, cmd := range invalid {
		if err := cmd.Validate(); err == nil {
			t.Errorf("%+v should have been rejected", cmd)
		}
	}
}

func TestMockDriverDoesNotInferPowerFromBrightness(t *testing.T) {
	// The regression this guards: brightness and power are separate frames on the real bulb,
	// so dimming one that is off leaves it off. Inferring otherwise made the UI report "on"
	// over a dark room and turned "All on" into a no-op.
	reg := mustRegistry(t, oneBulb)
	light, _ := reg.Get("bedroom")
	driver := NewMockDriver()
	ctx := context.Background()

	off := false
	driver.Apply(ctx, light, Command{Type: CommandPower, On: &off})

	value := 80
	state := driver.Apply(ctx, light, Command{Type: CommandBrightness, Value: &value})
	if state.Power {
		t.Error("brightness must not switch the bulb on")
	}
	if state.Brightness != 80 {
		t.Errorf("brightness not applied: %v", state.Brightness)
	}

	kelvin := 3000
	state = driver.Apply(ctx, light, Command{Type: CommandColorTemp, Kelvin: &kelvin})
	if state.Power {
		t.Error("colour temperature must not switch the bulb on")
	}
}

func TestServiceCachesReadsAndForceBypasses(t *testing.T) {
	reg := mustRegistry(t, oneBulb)
	driver := &countingDriver{inner: NewMockDriver()}
	svc := NewService(reg, driver, 5*time.Second, 0, 0)
	ctx := context.Background()

	svc.States(ctx, false)
	svc.States(ctx, false)
	if got := driver.reads.Load(); got != 1 {
		t.Errorf("second read should have been cached; driver saw %d reads", got)
	}

	svc.States(ctx, true)
	if got := driver.reads.Load(); got != 2 {
		t.Errorf("force should bypass the cache; driver saw %d reads", got)
	}
}

func TestServiceCollapsesConcurrentReadsOfOneBulb(t *testing.T) {
	// Two overlapping GATT reads of one peripheral tend to fail both, so callers arriving
	// while a read is open must wait on it rather than start a second.
	reg := mustRegistry(t, oneBulb)
	driver := &countingDriver{inner: NewMockDriver(), delay: 50 * time.Millisecond}
	svc := NewService(reg, driver, 0, 0, 0) // no cache, so only the in-flight join can dedupe
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { svc.States(ctx, false) })
	}
	wg.Wait()

	if got := driver.reads.Load(); got != 1 {
		t.Errorf("want 1 driver read for 8 concurrent callers, got %d", got)
	}
}

func TestServiceUnknownIDIsNotFound(t *testing.T) {
	svc := NewService(mustRegistry(t, oneBulb), NewMockDriver(), 0, 0, 0)
	if _, err := svc.State(context.Background(), "nope", false); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	on := true
	if _, err := svc.Send(context.Background(), "nope", Command{Type: CommandPower, On: &on}); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestBridgeDriverReportsUnreachableAsOfflineNotError(t *testing.T) {
	// A dead bridge must degrade to "every bulb offline", never to a failed request — one
	// unreachable bulb cannot be allowed to blank a page covering several.
	reg := mustRegistry(t, oneBulb)
	light, _ := reg.Get("bedroom")

	driver := NewBridgeDriver("http://127.0.0.1:1", "", 500*time.Millisecond)
	state := driver.GetState(context.Background(), light)

	if state.Online {
		t.Error("want offline")
	}
	if state.Error == "" {
		t.Error("want an error message explaining why")
	}
	// Identity and capabilities still come through, so the card renders.
	if state.ID != "bedroom" || state.Name != "Bedroom" || state.MaxColorTemp != 6500 {
		t.Errorf("identity/capabilities lost while offline: %+v", state)
	}
}

func TestBridgeDriverOverridesIdentityFromConfig(t *testing.T) {
	// The bridge knows the wire protocol, not which bulb this is. If it echoes a stale name
	// or the wrong capabilities, config must win.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "wrong", "name": "Wrong", "power": true, "brightness": 42.0,
			"supportsColor": true, "minColorTemp": 1000.0, "maxColorTemp": 9000.0,
		})
	}))
	defer server.Close()

	reg := mustRegistry(t, oneBulb)
	light, _ := reg.Get("bedroom")
	state := NewBridgeDriver(server.URL, "", time.Second).GetState(context.Background(), light)

	if state.ID != "bedroom" || state.Name != "Bedroom" {
		t.Errorf("identity should come from config: %+v", state)
	}
	if state.SupportsColor || state.MinColorTemp != 2700 || state.MaxColorTemp != 6500 {
		t.Errorf("capabilities should come from config: %+v", state)
	}
	// Actual bulb state does come from the bridge.
	if !state.Power || state.Brightness != 42 {
		t.Errorf("bulb state should come from the bridge: %+v", state)
	}
	if !state.Online {
		t.Error("a 200 from the bridge means online")
	}
}

func TestBridgeDriverSendsToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"power": false})
	}))
	defer server.Close()

	reg := mustRegistry(t, oneBulb)
	light, _ := reg.Get("bedroom")
	NewBridgeDriver(server.URL, "s3cret", time.Second).GetState(context.Background(), light)

	if gotAuth != "Bearer s3cret" {
		t.Errorf("want bearer token, got %q", gotAuth)
	}
}

// --- helpers ---

type countingDriver struct {
	inner *MockDriver
	delay time.Duration
	reads atomic.Int32
}

func (d *countingDriver) Kind() string { return "counting" }

func (d *countingDriver) GetState(ctx context.Context, light Config) State {
	d.reads.Add(1)
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return d.inner.GetState(ctx, light)
}

func (d *countingDriver) Apply(ctx context.Context, light Config, cmd Command) State {
	return d.inner.Apply(ctx, light, cmd)
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		})()
}

// --- settling -------------------------------------------------------------------------

// driftingDriver accepts a value but reports back something a little off, for the first
// driftUntil applications — the behaviour the real bulbs show.
type driftingDriver struct {
	inner      *MockDriver
	drift      float64
	driftUntil int
	applies    atomic.Int32
	reads      atomic.Int32
}

func (d *driftingDriver) Kind() string { return "drifting" }

func (d *driftingDriver) GetState(ctx context.Context, light Config) State {
	d.reads.Add(1)
	state := d.inner.GetState(ctx, light)
	if int(d.applies.Load()) <= d.driftUntil {
		state.Brightness += d.drift
		state.ColorTemp += d.drift
	}
	return state
}

func (d *driftingDriver) Apply(ctx context.Context, light Config, cmd Command) State {
	d.applies.Add(1)
	return d.inner.Apply(ctx, light, cmd)
}

func TestServiceReAppliesWhenTheBulbDrifts(t *testing.T) {
	// The point of settling: the client asked for 50, the lamp sat on 60, and nobody should
	// have to nudge it by hand.
	reg := mustRegistry(t, oneBulb)
	driver := &driftingDriver{inner: NewMockDriver(), drift: 10, driftUntil: 1}
	svc := NewService(reg, driver, 0, 2, time.Millisecond)

	value := 50
	state, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandBrightness, Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	if driver.applies.Load() < 2 {
		t.Errorf("expected a re-apply after the drift, got %d applies", driver.applies.Load())
	}
	if state.Brightness != 50 {
		t.Errorf("want the bulb settled on 50, got %v", state.Brightness)
	}
}

func TestServiceDoesNotReApplyWhenTheValueHolds(t *testing.T) {
	// No drift means no extra BLE traffic: settling must be free when nothing is wrong.
	reg := mustRegistry(t, oneBulb)
	driver := &driftingDriver{inner: NewMockDriver(), drift: 0}
	svc := NewService(reg, driver, 0, 2, time.Millisecond)

	value := 50
	if _, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandBrightness, Value: &value}); err != nil {
		t.Fatal(err)
	}
	if got := driver.applies.Load(); got != 1 {
		t.Errorf("want exactly 1 apply when the value holds, got %d", got)
	}
}

func TestServiceGivesUpOnAStubbornBulb(t *testing.T) {
	// A lamp that never takes the value must not spin forever — bounded attempts, then report
	// whatever it is actually doing.
	reg := mustRegistry(t, oneBulb)
	driver := &driftingDriver{inner: NewMockDriver(), drift: 25, driftUntil: 999}
	svc := NewService(reg, driver, 0, 2, time.Millisecond)

	value := 50
	if _, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandBrightness, Value: &value}); err != nil {
		t.Fatal(err)
	}
	// One initial apply plus at most the configured retries.
	if got := driver.applies.Load(); got > 3 {
		t.Errorf("settling should be bounded, got %d applies", got)
	}
}

func TestServiceDoesNotSettlePower(t *testing.T) {
	// Power is a boolean the bulb either took or did not; there is no "near enough" to chase,
	// so it must cost no extra reads.
	reg := mustRegistry(t, oneBulb)
	driver := &driftingDriver{inner: NewMockDriver(), drift: 10, driftUntil: 999}
	svc := NewService(reg, driver, 0, 2, time.Millisecond)

	on := true
	if _, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandPower, On: &on}); err != nil {
		t.Fatal(err)
	}
	if got := driver.reads.Load(); got != 0 {
		t.Errorf("power should not trigger a verification read, got %d reads", got)
	}
}

func TestServiceSettlingDisabled(t *testing.T) {
	reg := mustRegistry(t, oneBulb)
	driver := &driftingDriver{inner: NewMockDriver(), drift: 25, driftUntil: 999}
	svc := NewService(reg, driver, 0, 0, time.Millisecond)

	value := 50
	if _, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandBrightness, Value: &value}); err != nil {
		t.Fatal(err)
	}
	if got := driver.applies.Load(); got != 1 {
		t.Errorf("with settling off, want 1 apply, got %d", got)
	}
}

func TestServiceStopsSettlingIfTheBulbGoesOffline(t *testing.T) {
	// Losing the bulb mid-correction must surface as offline, not as the value we hoped for.
	reg := mustRegistry(t, oneBulb)
	svc := NewService(reg, &offlineOnReadDriver{inner: NewMockDriver()}, 0, 2, time.Millisecond)

	value := 50
	state, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandBrightness, Value: &value})
	if err != nil {
		t.Fatal(err)
	}
	if state.Online {
		t.Error("want the offline read reported")
	}
}

type offlineOnReadDriver struct{ inner *MockDriver }

func (d *offlineOnReadDriver) Kind() string { return "offline-on-read" }

func (d *offlineOnReadDriver) GetState(_ context.Context, light Config) State {
	return offlineState(light, "gone", time.Now().UnixMilli())
}

func (d *offlineOnReadDriver) Apply(ctx context.Context, light Config, cmd Command) State {
	return d.inner.Apply(ctx, light, cmd)
}
