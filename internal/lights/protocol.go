package lights

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
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

// errUnsupported marks a capability the hardware lacks; the bulb stays online.
var errUnsupported = errors.New("unsupported by this bulb")

// readback is what a bulb said about itself. These lamps stay silent when queried for a value
// they already hold, so an absent field means "unchanged", never zero.
type readback struct {
	Power      *bool
	Brightness *float64
	ColorTemp  *float64
	Mode       string
}

func (r readback) empty() bool {
	return r.Power == nil && r.Brightness == nil && r.ColorTemp == nil
}

// protocol is how one bulb family speaks. Per-bulb quirks live in the row's options. The driver
// holds a per-address lock around every call.
type protocol interface {
	Name() string
	// Info describes the model for the add-a-bulb form.
	Info() ProtocolInfo
	// Readable is false for bulbs that cannot report their settings; reads then use the last write.
	Readable() bool
	// Advertises is the GATT service UUID this family advertises; scans filter on it.
	Advertises() string
	Read(ctx context.Context, g gatt, light Light) (readback, error)
	SetPower(ctx context.Context, g gatt, light Light, on bool) error
	SetBrightness(ctx context.Context, g gatt, light Light, value int) error
	SetColor(ctx context.Context, g gatt, light Light, color RGB) error
	SetColorTemp(ctx context.Context, g gatt, light Light, kelvin int) error
}

var protocols = map[string]protocol{
	"lexman": lexman{},
}

func isBulb(device Discovered) bool {
	for _, p := range protocols {
		want := p.Advertises()
		for _, have := range device.Services {
			if strings.EqualFold(have, want) {
				return true
			}
		}
	}
	return false
}

func protocolFor(name string) (protocol, bool) {
	p, ok := protocols[name]
	return p, ok
}

func protocolInfos() []ProtocolInfo {
	infos := make([]ProtocolInfo, 0, len(protocols))
	for _, p := range protocols {
		infos = append(infos, p.Info())
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Label < infos[j].Label })
	return infos
}

// applyCommand routes a command already checked by Command.Validate.
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

	// The API speaks 0-100 and converts at this boundary.
	lexmanBrightnessMax = 254

	// 153 mireds = coolest, 454 = warmest, mapped linearly onto the vendor's 2700K-6500K label so it
	// round-trips exactly (a true reciprocal would give ~2200K).
	lexmanMiredCool, lexmanMiredWarm   = 153, 454
	lexmanKelvinCool, lexmanKelvinWarm = 6500, 2700

	lexmanQueryTimeout = 1200 * time.Millisecond
)

func (lexman) Name() string   { return "lexman" }
func (lexman) Readable() bool { return true }

// The advertised service, not the vendor one frames go to, which only exists once connected.
func (lexman) Advertises() string { return "0000a100-0000-1000-8000-00805f9b34fb" }

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
	// 0 reads back as off; off belongs to the switch command.
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
	// Snap to 10K so a 2800K write does not read back as 2801K.
	snapped := int(math.Round(kelvin/10)) * 10
	return clampInt(snapped, lexmanKelvinWarm, lexmanKelvinCool)
}

func optionString(light Light, key, fallback string) string {
	if raw, ok := light.Options[key]; ok {
		if value, ok := raw.(string); ok && value != "" {
			return value
		}
	}
	return fallback
}
