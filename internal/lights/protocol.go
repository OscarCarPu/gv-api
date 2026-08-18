package lights

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

/*
Per-model wire protocols: which GATT characteristic to write to, and what bytes mean "on",
"60% brightness", "this warm". Every bulb in the registry names one and the driver dispatches
on that name.

To add a model, find its address and GATT table with any BLE scanner (`bluetoothctl scan le`, then
`gatt list-attributes`), then implement this interface and register it in protocols below.
The characteristic you want is almost always the single write handle on a vendor service —
a 128-bit UUID that is not of the standard 0000xxxx-0000-1000-8000-00805f9b34fb form.
*/

// errUnsupported marks a capability the hardware does not have, as opposed to a failure.
// The driver reports it on the bulb's card while leaving the bulb online: nothing is wrong
// with it, it simply cannot do that.
var errUnsupported = errors.New("unsupported by this bulb")

// readback is what a bulb managed to say about itself. Every field is optional: these lamps
// answer nothing at all to a query for a value they already hold, so an absent field means
// "unchanged", never "off" or "zero".
type readback struct {
	Power      *bool
	Brightness *float64
	ColorTemp  *float64
	Mode       string
}

// empty reports whether the bulb told us nothing, in which case the driver keeps what it
// last knew rather than publishing a half-empty state.
func (r readback) empty() bool {
	return r.Power == nil && r.Brightness == nil && r.ColorTemp == nil
}

// protocol is how one bulb family speaks.
//
// Implementations get the bulb's row, so a per-bulb quirk (a different characteristic UUID,
// a key) is a value in its options rather than a code change. The driver holds a per-address
// lock around every call, so implementations need no locking of their own.
type protocol interface {
	Name() string
	// Info describes the model for the add-a-bulb form, so nobody has to know a lamp's kelvin
	// range to register it.
	Info() ProtocolInfo
	// Readable is false for bulbs that cannot be asked their current settings; the driver
	// then answers reads from what it last wrote, which is all anyone can do.
	Readable() bool
	Read(ctx context.Context, g gatt, light Light) (readback, error)
	SetPower(ctx context.Context, g gatt, light Light, on bool) error
	SetBrightness(ctx context.Context, g gatt, light Light, value int) error
	SetColor(ctx context.Context, g gatt, light Light, color RGB) error
	SetColorTemp(ctx context.Context, g gatt, light Light, kelvin int) error
}

var protocols = map[string]protocol{
	"lexman": lexman{},
}

func protocolFor(name string) (protocol, bool) {
	p, ok := protocols[name]
	return p, ok
}

// protocolInfos lists the supported models, in a stable order so the form's select does not
// shuffle between requests.
func protocolInfos() []ProtocolInfo {
	infos := make([]ProtocolInfo, 0, len(protocols))
	for _, p := range protocols {
		infos = append(infos, p.Info())
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Label < infos[j].Label })
	return infos
}

// applyCommand routes a validated command to the protocol. The command has already been
// checked by Command.Validate, so the pointers are safe to dereference.
func applyCommand(ctx context.Context, p protocol, g gatt, light Light, cmd Command) error {
	switch cmd.Type {
	case CommandPower:
		return p.SetPower(ctx, g, light, *cmd.On)
	case CommandBrightness:
		return p.SetBrightness(ctx, g, light, *cmd.Value)
	case CommandColor:
		return p.SetColor(ctx, g, light, *cmd.Color)
	case CommandColorTemp:
		return p.SetColorTemp(ctx, g, light, *cmd.Kelvin)
	default:
		return fmt.Errorf("%w: %s", ErrInvalidCommand, cmd.Type)
	}
}

// --- Lexman / Adeo ZBEK-13 ------------------------------------------------------------

/*
lexman speaks to the Adeo/LEXMAN ZBEK-13 tunable-white bulb (Leroy Merlin's Enki range).

Frame format from https://github.com/davidsmfreire/lexman-ble — the UUIDs there match this
bulb exactly. Every frame is written to a101 and answered by a notification on a102:

	ping         set —                             query 00:00:20:01:02:00:02
	switch       set 00:00:10:01:03:{0}:00:00      query 00:00:10:02
	brightness   set 00:00:11:01:03:{0}:00:00      query 00:00:11:02
	temperature  set 00:00:12:01:04:{1}:{0}:00:00  query 00:00:12:02

{0} is the low byte and {1} the high byte of the argument.

Two behaviours worth knowing, both observed on the real bulb:
  - Writing a value it already holds produces no notification at all.
  - Some temperature steps stay silent mid-transition, though a follow-up query confirms the
    value did land.

So a missing notification is never treated as failure — only the query path cares about
replies, and it falls back to the last known value.

Despite the ZB in the model name (it speaks Zigbee too) this is plain BLE with no pairing:
Connect is enough. It accepts a single central at a time, so while the Enki app is connected
on a phone the bulb stops advertising and is invisible here.
*/
type lexman struct{}

const (
	lexmanWriteUUID  = "0000a101-1115-1000-0001-617573746f6d"
	lexmanNotifyUUID = "0000a102-1115-1000-0001-617573746f6d"

	// The bulb's own brightness scale. The API speaks 0-100 and converts at this boundary.
	lexmanBrightnessMax = 254

	// Vendor range: 153 mireds = coolest, 454 = warmest. The kelvin labels come from the
	// vendor's stated 2700K-6500K span, so the mapping is linear in kelvin rather than a true
	// reciprocal (1e6/454 would be ~2200K). It round-trips exactly, which is what matters for
	// the slider; if the rendered colour ever disagrees with the number, this is the one place
	// to change.
	lexmanMiredCool, lexmanMiredWarm   = 153, 454
	lexmanKelvinCool, lexmanKelvinWarm = 6500, 2700

	// How long to wait for a query's notification before falling back to the last known value.
	lexmanQueryTimeout = 1200 * time.Millisecond
)

func (lexman) Name() string   { return "lexman" }
func (lexman) Readable() bool { return true }

func (lexman) Info() ProtocolInfo {
	return ProtocolInfo{
		Name:              "lexman",
		Label:             "LEXMAN / Adeo ZBEK-13 (tunable white)",
		SupportsColor:     false,
		SupportsColorTemp: true,
		MinColorTemp:      lexmanKelvinWarm,
		MaxColorTemp:      lexmanKelvinCool,
	}
}

// writeUUID and notifyUUID let a single odd bulb override the characteristics from its own
// options without a new protocol.
func (lexman) writeUUID(light Light) string {
	return optionString(light, "writeChar", lexmanWriteUUID)
}

func (lexman) notifyUUID(light Light) string {
	return optionString(light, "notifyChar", lexmanNotifyUUID)
}

func (p lexman) send(ctx context.Context, g gatt, light Light, frame ...byte) error {
	return g.Write(ctx, light.Address, p.writeUUID(light), frame)
}

func (p lexman) query(ctx context.Context, g gatt, light Light, frame ...byte) ([]byte, error) {
	return g.Query(ctx, light.Address, p.writeUUID(light), p.notifyUUID(light), frame, lexmanQueryTimeout)
}

func (p lexman) SetPower(ctx context.Context, g gatt, light Light, on bool) error {
	value := byte(0)
	if on {
		value = 1
	}
	return p.send(ctx, g, light, 0x00, 0x00, 0x10, 0x01, 0x03, value, 0x00, 0x00)
}

func (p lexman) SetBrightness(ctx context.Context, g gatt, light Light, value int) error {
	// 0 reads back as "off" rather than "dimmest", and off belongs to the switch command.
	raw := max((clampInt(value, 0, 100)*lexmanBrightnessMax+50)/100, 1)
	return p.send(ctx, g, light, 0x00, 0x00, 0x11, 0x01, 0x03, byte(raw), 0x00, 0x00)
}

func (p lexman) SetColor(_ context.Context, _ gatt, _ Light, _ RGB) error {
	return fmt.Errorf("%w: the ZBEK-13 is tunable white only, it has no RGB", errUnsupported)
}

func (p lexman) SetColorTemp(ctx context.Context, g gatt, light Light, kelvin int) error {
	mired := kelvinToMired(kelvin)
	return p.send(ctx, g, light,
		0x00, 0x00, 0x12, 0x01, 0x04, byte(mired>>8&0xFF), byte(mired&0xFF), 0x00, 0x00)
}

func (p lexman) Read(ctx context.Context, g gatt, light Light) (readback, error) {
	state := readback{Mode: "white"}

	// switch -> 00:00:10:03:02:{0}:{0}
	reply, err := p.query(ctx, g, light, 0x00, 0x00, 0x10, 0x02)
	if err != nil {
		return readback{}, err
	}
	if len(reply) >= 6 {
		power := reply[5] != 0
		state.Power = &power
	}

	// brightness -> 00:00:11:03:02:{0}:{0}
	reply, err = p.query(ctx, g, light, 0x00, 0x00, 0x11, 0x02)
	if err != nil {
		return readback{}, err
	}
	if len(reply) >= 6 {
		brightness := float64((int(reply[5])*100 + lexmanBrightnessMax/2) / lexmanBrightnessMax)
		state.Brightness = &brightness
	}

	// temperature -> 00:00:12:03:04:{1}:{0}:{1}:{0}
	reply, err = p.query(ctx, g, light, 0x00, 0x00, 0x12, 0x02)
	if err != nil {
		return readback{}, err
	}
	if len(reply) >= 7 {
		kelvin := float64(miredToKelvin(int(reply[5])<<8 | int(reply[6])))
		state.ColorTemp = &kelvin
	}

	return state, nil
}

func kelvinToMired(kelvin int) int {
	span := float64(lexmanMiredWarm-lexmanMiredCool) / float64(lexmanKelvinWarm-lexmanKelvinCool)
	mired := int(math.Round(float64(kelvin-lexmanKelvinCool)*span)) + lexmanMiredCool
	return clampInt(mired, lexmanMiredCool, lexmanMiredWarm)
}

func miredToKelvin(mired int) int {
	span := float64(lexmanKelvinWarm-lexmanKelvinCool) / float64(lexmanMiredWarm-lexmanMiredCool)
	kelvin := float64(mired-lexmanMiredCool)*span + lexmanKelvinCool
	// Snap to 10K. One mired step is ~13K, so the extra digits are quantisation noise, and
	// without this a 2800K write reads back as 2801K and the slider twitches.
	snapped := int(math.Round(kelvin/10)) * 10
	return clampInt(snapped, lexmanKelvinWarm, lexmanKelvinCool)
}

// optionString reads a per-bulb option from its row, falling back to the protocol's
// own default.
func optionString(light Light, key, fallback string) string {
	if raw, ok := light.Options[key]; ok {
		if value, ok := raw.(string); ok && value != "" {
			return value
		}
	}
	return fallback
}
