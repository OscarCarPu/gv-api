package lights

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// bedroom is the bulb most tests drive: the real one, tunable white only.
func bulb(t *testing.T) Light {
	t.Helper()
	return Light{
		ID:                "bedroom",
		Name:              "Bedroom",
		Model:             "LEXMAN",
		Address:           "AA:BB:CC:DD:EE:FF",
		Protocol:          "lexman",
		SupportsColor:     false,
		SupportsColorTemp: true,
		MinColorTemp:      2700,
		MaxColorTemp:      6500,
	}
}

func TestPublicHidesAddress(t *testing.T) {
	// The BLE address identifies hardware in the house; it must not reach a client.
	body, err := json.Marshal(bulb(t).Public())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"AA:BB:CC:DD:EE:FF", "address", "protocol"} {
		if contains(string(body), secret) {
			t.Errorf("public payload leaked %q: %s", secret, body)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Bedroom":             "bedroom",
		"  Living room  ":     "living-room",
		"Salón":               "salon",
		"Habitación de Óscar": "habitacion-de-oscar",
		"Lámpara #2":          "lampara-2",
		"---":                 "light", // nothing usable, but a bulb still needs an id
		"":                    "light",
	}
	for name, want := range cases {
		if got := slugify(name); got != want {
			t.Errorf("slugify(%q) = %q, want %q", name, got, want)
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
	light := bulb(t)
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
	repo := newFakeRepo(bulb(t))
	driver := &countingDriver{inner: NewMockDriver()}
	svc := NewService(repo, driver, 5*time.Second, 0, 0)
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
	repo := newFakeRepo(bulb(t))
	driver := &countingDriver{inner: NewMockDriver(), delay: 50 * time.Millisecond}
	svc := NewService(repo, driver, 0, 0, 0) // no cache, so only the in-flight join can dedupe
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
	svc := NewService(newFakeRepo(bulb(t)), NewMockDriver(), 0, 0, 0)
	if _, err := svc.State(context.Background(), "nope", false); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	on := true
	if _, err := svc.Send(context.Background(), "nope", Command{Type: CommandPower, On: &on}); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// --- BlueZ driver ---------------------------------------------------------------------

func TestBlueZDriverDoesNotLeakTheBulbAddress(t *testing.T) {
	// BlueZ's errors carry the D-Bus object path, which embeds the bulb's MAC. That string
	// is handed to every client, including a phone on the open internet — it means nothing to
	// a person and names hardware in the house. The meaning is kept; the address is not.
	light := bulb(t)
	radio := &fakeGATT{connectErr: fmt.Errorf("%w: %s", errBulbNotFound,
		`Object /org/bluez/hci0/dev_AA_BB_CC_DD_EE_FF does not exist`)}

	state := newDriver(radio, time.Minute).GetState(context.Background(), light)

	if state.Online {
		t.Fatal("want offline")
	}
	for _, leak := range []string{"AA:BB:CC:DD:EE:FF", "AA_BB", "/org/bluez", "hci0"} {
		if contains(state.Error, leak) {
			t.Errorf("error leaked %q: %s", leak, state.Error)
		}
	}
	if state.Error == "" {
		t.Error("want some explanation of what went wrong")
	}
}

func TestBlueZDriverReportsUnreachableAsOfflineNotError(t *testing.T) {
	// An unreachable bulb must degrade to one offline card, never to a failed request — it
	// cannot be allowed to blank a page covering several.
	light := bulb(t)
	radio := &fakeGATT{connectErr: errBulbBusy}

	state := newDriver(radio, time.Minute).GetState(context.Background(), light)

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

func TestBlueZDriverKeepsWhatItLastSetWhenTheBulbStaysQuiet(t *testing.T) {
	// These bulbs answer nothing when written a value they already hold, so a read that comes
	// back empty must not blank the card — what we last set is the best answer there is.
	light := bulb(t)
	radio := &fakeGATT{} // every query times out, i.e. returns no bytes
	driver := newDriver(radio, time.Minute)
	ctx := context.Background()

	value := 35
	driver.Apply(ctx, light, Command{Type: CommandBrightness, Value: &value})

	state := driver.GetState(ctx, light)
	if !state.Online {
		t.Fatal("a silent bulb we connected to is still online")
	}
	if state.Brightness != 35 {
		t.Errorf("want the last value we set, got %v", state.Brightness)
	}
}

func TestBlueZDriverReadbackWins(t *testing.T) {
	// When the bulb does answer, it is the authority — someone may have used the wall remote.
	light := bulb(t)
	radio := &fakeGATT{replies: map[byte][]byte{
		0x10: {0x00, 0x00, 0x10, 0x03, 0x02, 0x01, 0x01},       // on
		0x11: {0x00, 0x00, 0x11, 0x03, 0x02, 0x7F, 0x7F},       // 127/254 -> 50%
		0x12: {0x00, 0x00, 0x12, 0x03, 0x04, 0x01, 0x2C, 0, 0}, // 300 mireds
	}}
	driver := newDriver(radio, time.Minute)
	ctx := context.Background()

	value := 35
	driver.Apply(ctx, light, Command{Type: CommandBrightness, Value: &value})
	state := driver.GetState(ctx, light)

	if !state.Power {
		t.Error("want the bulb's own answer: on")
	}
	if state.Brightness != 50 {
		t.Errorf("want the bulb's own brightness (50), got %v", state.Brightness)
	}
	if state.ColorTemp != 4640 {
		t.Errorf("want 300 mireds as 4640K, got %v", state.ColorTemp)
	}
}

func TestBlueZDriverUnsupportedCommandLeavesTheBulbOnline(t *testing.T) {
	// Asking a tunable-white bulb for red is a limit, not a fault: the card says so and the
	// bulb keeps working.
	light := bulb(t)
	driver := newDriver(&fakeGATT{}, time.Minute)

	state := driver.Apply(context.Background(), light, Command{Type: CommandColor, Color: &RGB{R: 255}})

	if !state.Online {
		t.Error("an unsupported command must not mark the bulb offline")
	}
	if state.Error == "" {
		t.Error("want an explanation on the card")
	}
	if state.Mode == "color" {
		t.Error("a command the bulb refused must not change its state")
	}
}

func TestBlueZDriverSerialisesOneBulb(t *testing.T) {
	// Two overlapping GATT writes to one peripheral tend to fail both, so the driver must
	// hold the bulb rather than trust every caller to take turns.
	light := bulb(t)
	radio := &fakeGATT{}
	driver := newDriver(radio, time.Minute)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 12 {
		value := i * 8
		wg.Go(func() {
			driver.Apply(ctx, light, Command{Type: CommandBrightness, Value: &value})
		})
	}
	wg.Wait()

	if got := radio.maxConcurrent.Load(); got > 1 {
		t.Errorf("want one conversation at a time, saw %d overlapping", got)
	}
}

func TestBlueZDriverUnknownProtocolIsAConfigError(t *testing.T) {
	light := Light{ID: "odd", Address: "AA:BB", Protocol: "nope"}

	state := newDriver(&fakeGATT{}, time.Minute).GetState(context.Background(), light)

	if state.Online {
		t.Error("want offline for a bulb no protocol can drive")
	}
	if !contains(state.Error, "nope") {
		t.Errorf("the message should name the protocol that is missing: %q", state.Error)
	}
}

// --- lexman protocol ------------------------------------------------------------------

func TestLexmanFrames(t *testing.T) {
	light := bulb(t)
	ctx := context.Background()

	cases := []struct {
		name string
		do   func(g gatt) error
		want []byte
	}{
		{"power on", func(g gatt) error { return lexman{}.SetPower(ctx, g, light, true) },
			[]byte{0x00, 0x00, 0x10, 0x01, 0x03, 0x01, 0x00, 0x00}},
		{"power off", func(g gatt) error { return lexman{}.SetPower(ctx, g, light, false) },
			[]byte{0x00, 0x00, 0x10, 0x01, 0x03, 0x00, 0x00, 0x00}},
		{"brightness 100", func(g gatt) error { return lexman{}.SetBrightness(ctx, g, light, 100) },
			[]byte{0x00, 0x00, 0x11, 0x01, 0x03, 0xFE, 0x00, 0x00}},
		// 0% must still be a legal step: off belongs to the switch command, and a raw 0 would
		// read back as "off" and confuse the next poll.
		{"brightness 0", func(g gatt) error { return lexman{}.SetBrightness(ctx, g, light, 0) },
			[]byte{0x00, 0x00, 0x11, 0x01, 0x03, 0x01, 0x00, 0x00}},
		{"warmest", func(g gatt) error { return lexman{}.SetColorTemp(ctx, g, light, 2700) },
			[]byte{0x00, 0x00, 0x12, 0x01, 0x04, 0x01, 0xC6, 0x00, 0x00}}, // 454 mireds
		{"coolest", func(g gatt) error { return lexman{}.SetColorTemp(ctx, g, light, 6500) },
			[]byte{0x00, 0x00, 0x12, 0x01, 0x04, 0x00, 0x99, 0x00, 0x00}}, // 153 mireds
	}

	for _, tc := range cases {
		radio := &fakeGATT{}
		if err := tc.do(radio); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := radio.lastWrite(); !bytes.Equal(got, tc.want) {
			t.Errorf("%s frame = % x, want % x", tc.name, got, tc.want)
		}
	}
}

func TestLexmanKelvinRoundTrips(t *testing.T) {
	/*
		The bulb works in mireds, and one step is ~13K, so a kelvin value cannot come back as
		itself — 3400K settling at 3390K is quantisation, not drift.

		What must hold is that the round trip stays inside the settle tolerance. If it did not,
		every temperature write would read back as "wrong", and the settle loop would chase its
		own rounding until it ran out of attempts.
	*/
	_, tolerance, _ := settleTarget(Command{Type: CommandColorTemp, Kelvin: ptr(3000)})
	for kelvin := lexmanKelvinWarm; kelvin <= lexmanKelvinCool; kelvin += 10 {
		got := miredToKelvin(kelvinToMired(kelvin))
		if drift := got - kelvin; drift > int(tolerance) || drift < -int(tolerance) {
			t.Fatalf("%dK round-tripped as %dK, outside the %.0fK settle tolerance", kelvin, got, tolerance)
		}
	}
	// On the bulb's own scale it is exact, which is what keeps the slider from twitching when
	// a value is read straight back.
	for mired := lexmanMiredCool; mired <= lexmanMiredWarm; mired++ {
		if got := kelvinToMired(miredToKelvin(mired)); got != mired {
			t.Fatalf("%d mireds round-tripped as %d", mired, got)
		}
	}
	// Out of range clamps to what the bulb can actually do rather than failing.
	if got := kelvinToMired(1000); got != lexmanMiredWarm {
		t.Errorf("below range should clamp warm, got %d mireds", got)
	}
	if got := kelvinToMired(9000); got != lexmanMiredCool {
		t.Errorf("above range should clamp cool, got %d mireds", got)
	}
}

func TestLexmanReadIsPartial(t *testing.T) {
	// A bulb that answers only some queries must yield only those fields: an absent answer
	// means "unchanged", never zero.
	radio := &fakeGATT{replies: map[byte][]byte{
		0x11: {0x00, 0x00, 0x11, 0x03, 0x02, 0xFE, 0xFE},
	}}

	values, err := lexman{}.Read(context.Background(), radio, bulb(t))
	if err != nil {
		t.Fatal(err)
	}
	if values.Power != nil {
		t.Error("power was never answered; it must stay unknown")
	}
	if values.Brightness == nil || *values.Brightness != 100 {
		t.Errorf("want 100%% brightness, got %v", values.Brightness)
	}
	if values.ColorTemp != nil {
		t.Error("temperature was never answered; it must stay unknown")
	}
}

func TestLexmanReadEmptyWhenTheBulbSaysNothing(t *testing.T) {
	values, err := lexman{}.Read(context.Background(), &fakeGATT{}, bulb(t))
	if err != nil {
		t.Fatal(err)
	}
	if !values.empty() {
		t.Errorf("want an empty readback, got %+v", values)
	}
}

func TestLexmanUsesPerBulbCharacteristicOverride(t *testing.T) {
	// A near-identical bulb with different UUIDs should be an env edit, not a new protocol.
	light := Light{
		ID: "odd", Address: "AA:BB", Protocol: "lexman",
		Options: map[string]any{"writeChar": "0000beef-0000-1000-8000-00805f9b34fb"},
	}

	radio := &fakeGATT{}
	if err := (lexman{}).SetPower(context.Background(), radio, light, true); err != nil {
		t.Fatal(err)
	}
	if radio.lastUUID != "0000beef-0000-1000-8000-00805f9b34fb" {
		t.Errorf("want the overridden characteristic, wrote to %q", radio.lastUUID)
	}
}

// --- managing which bulbs exist --------------------------------------------------------

func TestCreateFillsInWhatTheModelCanDo(t *testing.T) {
	// Nobody adding a lamp to a bedroom knows its kelvin range. Naming the model is enough.
	repo := newFakeRepo()
	svc := NewService(repo, NewMockDriver(), 0, 0, 0)

	light, err := svc.Create(context.Background(), CreateLightRequest{
		Name: "Bedroom", Address: "aa:bb:cc:dd:ee:ff", Protocol: "lexman",
	})
	if err != nil {
		t.Fatal(err)
	}
	if light.ID != "bedroom" {
		t.Errorf("want an id slugged from the name, got %q", light.ID)
	}
	if light.SupportsColor || !light.SupportsColorTemp {
		t.Errorf("capabilities should come from the protocol: %+v", light)
	}
	if light.MinColorTemp != 2700 || light.MaxColorTemp != 6500 {
		t.Errorf("kelvin range should come from the protocol: %+v", light)
	}
	// Addresses are compared against scan results, which BlueZ reports uppercase.
	if stored := repo.lights[0].Address; stored != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("address should be normalised, stored %q", stored)
	}
}

func TestCreateGivesTwoBulbsOfTheSameNameDifferentIds(t *testing.T) {
	// Two lamps called "Lamp" is an ordinary house. Two rows with one id is not, and renaming
	// the lamp is not the user's problem to solve.
	svc := NewService(newFakeRepo(), NewMockDriver(), 0, 0, 0)
	ctx := context.Background()

	first, err := svc.Create(ctx, CreateLightRequest{Name: "Lamp", Address: "AA:01", Protocol: "lexman"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create(ctx, CreateLightRequest{Name: "Lamp", Address: "AA:02", Protocol: "lexman"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "lamp" || second.ID != "lamp-2" {
		t.Errorf("want lamp and lamp-2, got %q and %q", first.ID, second.ID)
	}
}

func TestCreateRejectsTheSameBulbTwice(t *testing.T) {
	// Two cards for one lamp would fight over a link that takes one conversation at a time.
	svc := NewService(newFakeRepo(), NewMockDriver(), 0, 0, 0)
	ctx := context.Background()

	if _, err := svc.Create(ctx, CreateLightRequest{Name: "Lamp", Address: "AA:01", Protocol: "lexman"}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(ctx, CreateLightRequest{Name: "Other", Address: "aa:01", Protocol: "lexman"})
	if !errors.Is(err, ErrDuplicateAddress) {
		t.Errorf("want ErrDuplicateAddress for the same bulb in another case, got %v", err)
	}
}

func TestCreateRejectsNonsense(t *testing.T) {
	svc := NewService(newFakeRepo(), NewMockDriver(), 0, 0, 0)
	ctx := context.Background()

	bad := []CreateLightRequest{
		{Address: "AA:01", Protocol: "lexman"},             // no name
		{Name: "Lamp", Protocol: "lexman"},                 // no address
		{Name: "Lamp", Address: "AA:01", Protocol: "nope"}, // no such protocol
		{Name: "Lamp", Address: "AA:01", Protocol: "lexman", MinColorTemp: ptr(9000.0), MaxColorTemp: ptr(3000.0)},
	}
	for _, req := range bad {
		if _, err := svc.Create(ctx, req); !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%+v should have been rejected, got %v", req, err)
		}
	}
}

func TestUpdateKeepsWhatWasNotSent(t *testing.T) {
	repo := newFakeRepo(bulb(t))
	svc := NewService(repo, NewMockDriver(), 0, 0, 0)

	light, err := svc.Update(context.Background(), "bedroom", UpdateLightRequest{Name: ptr("Bedside")})
	if err != nil {
		t.Fatal(err)
	}
	if light.Name != "Bedside" {
		t.Errorf("rename did not take: %+v", light)
	}
	if light.ID != "bedroom" {
		t.Errorf("the id must survive a rename — clients hold state under it: %+v", light)
	}
	if light.MaxColorTemp != 6500 || light.SupportsColorTemp != true {
		t.Errorf("omitted fields should be untouched: %+v", light)
	}
}

func TestRenameDoesNotLeaveTheOldNameInTheCache(t *testing.T) {
	// The name travels inside State, so a cached read would keep showing the old one for as
	// long as the TTL lasts.
	repo := newFakeRepo(bulb(t))
	svc := NewService(repo, NewMockDriver(), time.Minute, 0, 0)
	ctx := context.Background()

	if _, err := svc.State(ctx, "bedroom", false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, "bedroom", UpdateLightRequest{Name: ptr("Bedside")}); err != nil {
		t.Fatal(err)
	}

	state, err := svc.State(ctx, "bedroom", false)
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != "Bedside" {
		t.Errorf("want the new name, got %q", state.Name)
	}
}

func TestDeleteRemovesTheBulb(t *testing.T) {
	repo := newFakeRepo(bulb(t))
	svc := NewService(repo, NewMockDriver(), 0, 0, 0)
	ctx := context.Background()

	if err := svc.Delete(ctx, "bedroom"); err != nil {
		t.Fatal(err)
	}
	states, err := svc.States(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 0 {
		t.Errorf("want no bulbs left, got %d", len(states))
	}
	if err := svc.Delete(ctx, "bedroom"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting it twice should be a 404, got %v", err)
	}
}

func TestDiscoverMarksBulbsAlreadyAdded(t *testing.T) {
	// Without this the add screen offers a bulb that is already on the page, and the only
	// feedback is a duplicate-address error after the fact.
	repo := newFakeRepo(Light{ID: "known", Address: "00:11:22:33:44:55", Protocol: "lexman"})
	svc := NewService(repo, NewMockDriver(), 0, 0, 0)

	devices, err := svc.Discover(context.Background(), 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) < 2 {
		t.Fatalf("want the mock's devices, got %d", len(devices))
	}
	if !devices[0].Known {
		t.Errorf("the registered bulb should be marked as added: %+v", devices[0])
	}
	if devices[1].Known {
		t.Errorf("an unregistered bulb should not be: %+v", devices[1])
	}
}

func TestProtocolsDescribeTheModelsOnOffer(t *testing.T) {
	svc := NewService(newFakeRepo(), NewMockDriver(), 0, 0, 0)

	infos := svc.Protocols()
	if len(infos) == 0 {
		t.Fatal("want at least the one bulb family this house has")
	}
	for _, info := range infos {
		if info.Name == "" || info.Label == "" {
			t.Errorf("a model with no name cannot be offered in a form: %+v", info)
		}
		if _, ok := protocolFor(info.Name); !ok {
			t.Errorf("%q is offered but nothing can drive it", info.Name)
		}
	}
}

// --- helpers ---

// fakeRepo is the lights table, in memory, including the two unique constraints that shape
// the service's behaviour.
type fakeRepo struct {
	mu     sync.Mutex
	lights []Light
}

func newFakeRepo(lights ...Light) *fakeRepo { return &fakeRepo{lights: lights} }

func (r *fakeRepo) List(context.Context) ([]Light, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Light(nil), r.lights...), nil
}

func (r *fakeRepo) Get(_ context.Context, id string) (Light, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, light := range r.lights {
		if light.ID == id {
			return light, nil
		}
	}
	return Light{}, ErrNotFound
}

func (r *fakeRepo) Create(_ context.Context, light Light) (Light, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.lights {
		if existing.Address == light.Address {
			return Light{}, ErrDuplicateAddress
		}
		if existing.ID == light.ID {
			return Light{}, errDuplicateID
		}
	}
	r.lights = append(r.lights, light)
	return light, nil
}

func (r *fakeRepo) Update(_ context.Context, light Light) (Light, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.lights {
		if existing.ID == light.ID {
			light.Address = existing.Address // not updatable, as in Postgres
			r.lights[i] = light
			return light, nil
		}
	}
	return Light{}, ErrNotFound
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, light := range r.lights {
		if light.ID == id {
			r.lights = append(r.lights[:i], r.lights[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

/*
fakeGATT stands in for the radio.

It answers queries from a table keyed by the command byte (frame[2] — 0x10 switch, 0x11
brightness, 0x12 temperature) and returns nothing for anything else, which is exactly what a
real bulb does when written a value it already holds. It also tracks how many calls are ever
in flight at once, so the per-bulb serialisation can be asserted rather than assumed.
*/
type fakeGATT struct {
	connectErr error
	replies    map[byte][]byte

	mu       sync.Mutex
	writes   [][]byte
	lastUUID string

	active        atomic.Int32
	maxConcurrent atomic.Int32
}

func (f *fakeGATT) enter() func() {
	inFlight := f.active.Add(1)
	for {
		peak := f.maxConcurrent.Load()
		if inFlight <= peak || f.maxConcurrent.CompareAndSwap(peak, inFlight) {
			break
		}
	}
	// Widen the window: without it every call is over before the next one starts and an
	// overlap could never be observed, passing the test for the wrong reason.
	time.Sleep(time.Millisecond)
	return func() { f.active.Add(-1) }
}

func (f *fakeGATT) Connect(_ context.Context, _ string) error {
	defer f.enter()()
	return f.connectErr
}

func (f *fakeGATT) Write(_ context.Context, _, charUUID string, payload []byte) error {
	defer f.enter()()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastUUID = charUUID
	f.writes = append(f.writes, payload)
	return nil
}

func (f *fakeGATT) Query(
	ctx context.Context, address, writeUUID, _ string, payload []byte, _ time.Duration,
) ([]byte, error) {
	if err := f.Write(ctx, address, writeUUID, payload); err != nil {
		return nil, err
	}
	if len(payload) < 3 {
		return nil, nil
	}
	return f.replies[payload[2]], nil
}

func (f *fakeGATT) Scan(_ context.Context, _ time.Duration) ([]Discovered, error) {
	return []Discovered{{Address: "AA:BB:CC:DD:EE:FF", Name: "Bedroom", RSSI: -50}}, nil
}

func (f *fakeGATT) Disconnect(string) {}

func (f *fakeGATT) Close() error { return nil }

func (f *fakeGATT) lastWrite() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) == 0 {
		return nil
	}
	return f.writes[len(f.writes)-1]
}

type countingDriver struct {
	inner *MockDriver
	delay time.Duration
	reads atomic.Int32
}

// Discover is the mock's, unchanged: none of these doubles is about scanning.
func (d *countingDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	return d.inner.Discover(ctx, window)
}

func (d *countingDriver) Kind() string { return "counting" }

func (d *countingDriver) GetState(ctx context.Context, light Light) State {
	d.reads.Add(1)
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return d.inner.GetState(ctx, light)
}

func (d *countingDriver) Apply(ctx context.Context, light Light, cmd Command) State {
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

// Discover is the mock's, unchanged: none of these doubles is about scanning.
func (d *driftingDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	return d.inner.Discover(ctx, window)
}

func (d *driftingDriver) Kind() string { return "drifting" }

func (d *driftingDriver) GetState(ctx context.Context, light Light) State {
	d.reads.Add(1)
	state := d.inner.GetState(ctx, light)
	if int(d.applies.Load()) <= d.driftUntil {
		state.Brightness += d.drift
		state.ColorTemp += d.drift
	}
	return state
}

func (d *driftingDriver) Apply(ctx context.Context, light Light, cmd Command) State {
	d.applies.Add(1)
	return d.inner.Apply(ctx, light, cmd)
}

func TestServiceReAppliesWhenTheBulbDrifts(t *testing.T) {
	// The point of settling: the client asked for 50, the lamp sat on 60, and nobody should
	// have to nudge it by hand.
	repo := newFakeRepo(bulb(t))
	driver := &driftingDriver{inner: NewMockDriver(), drift: 10, driftUntil: 1}
	svc := NewService(repo, driver, 0, 2, time.Millisecond)

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
	repo := newFakeRepo(bulb(t))
	driver := &driftingDriver{inner: NewMockDriver(), drift: 0}
	svc := NewService(repo, driver, 0, 2, time.Millisecond)

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
	repo := newFakeRepo(bulb(t))
	driver := &driftingDriver{inner: NewMockDriver(), drift: 25, driftUntil: 999}
	svc := NewService(repo, driver, 0, 2, time.Millisecond)

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
	repo := newFakeRepo(bulb(t))
	driver := &driftingDriver{inner: NewMockDriver(), drift: 10, driftUntil: 999}
	svc := NewService(repo, driver, 0, 2, time.Millisecond)

	on := true
	if _, err := svc.Send(context.Background(), "bedroom", Command{Type: CommandPower, On: &on}); err != nil {
		t.Fatal(err)
	}
	if got := driver.reads.Load(); got != 0 {
		t.Errorf("power should not trigger a verification read, got %d reads", got)
	}
}

func TestServiceSettlingDisabled(t *testing.T) {
	repo := newFakeRepo(bulb(t))
	driver := &driftingDriver{inner: NewMockDriver(), drift: 25, driftUntil: 999}
	svc := NewService(repo, driver, 0, 0, time.Millisecond)

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
	repo := newFakeRepo(bulb(t))
	svc := NewService(repo, &offlineOnReadDriver{inner: NewMockDriver()}, 0, 2, time.Millisecond)

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

// Discover is the mock's, unchanged: none of these doubles is about scanning.
func (d *offlineOnReadDriver) Discover(ctx context.Context, window time.Duration) ([]Discovered, error) {
	return d.inner.Discover(ctx, window)
}

func (d *offlineOnReadDriver) Kind() string { return "offline-on-read" }

func (d *offlineOnReadDriver) GetState(_ context.Context, light Light) State {
	return offlineState(light, "gone", time.Now().UnixMilli())
}

func (d *offlineOnReadDriver) Apply(ctx context.Context, light Light, cmd Command) State {
	return d.inner.Apply(ctx, light, cmd)
}

func ptr[T any](v T) *T { return &v }
