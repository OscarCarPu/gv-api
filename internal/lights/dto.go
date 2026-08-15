package lights

import (
	"errors"
	"fmt"
)

// RGB is a colour in the 0-255 sRGB space the bulbs speak.
type RGB struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

// State is one bulb as reported to clients.
//
// Field names are camelCase rather than the snake_case used elsewhere in this API: this shape
// is passed through verbatim by the bridge daemon at one end and consumed by TypeScript at the
// other, and renaming in the middle would only add a translation layer to get wrong.
type State struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	// Online is false when the bridge could not reach the bulb. The remaining fields are then
	// the last known values, not live ones.
	Online     bool    `json:"online"`
	Power      bool    `json:"power"`
	Brightness float64 `json:"brightness"` // 0-100, normalised from the bulb's own scale
	Mode       string  `json:"mode"`       // "color" | "white"
	Color      RGB     `json:"color"`
	ColorTemp  float64 `json:"colorTemp"`
	// Capabilities, so a client can hide controls a bulb does not have.
	SupportsColor     bool    `json:"supportsColor"`
	SupportsColorTemp bool    `json:"supportsColorTemp"`
	MinColorTemp      float64 `json:"minColorTemp"`
	MaxColorTemp      float64 `json:"maxColorTemp"`
	// Error is per-bulb, so one unreachable bulb is shown on its own card rather than
	// failing the whole request.
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

// PublicLight is a configured bulb without any live state or its BLE address.
type PublicLight struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Model             string  `json:"model"`
	SupportsColor     bool    `json:"supportsColor"`
	SupportsColorTemp bool    `json:"supportsColorTemp"`
	MinColorTemp      float64 `json:"minColorTemp"`
	MaxColorTemp      float64 `json:"maxColorTemp"`
}

// StatesResponse wraps the list so the payload stays an object and can grow later.
type StatesResponse struct {
	States []State `json:"states"`
}

// Command is one instruction for a bulb. Exactly one of the value fields applies, chosen by
// Type; the rest are ignored. Validate before handing it to a driver.
type Command struct {
	Type   string `json:"type"`
	On     *bool  `json:"on,omitempty"`
	Value  *int   `json:"value,omitempty"`
	Color  *RGB   `json:"color,omitempty"`
	Kelvin *int   `json:"kelvin,omitempty"`
}

// Command type values.
const (
	CommandPower      = "power"
	CommandBrightness = "brightness"
	CommandColor      = "color"
	CommandColorTemp  = "colorTemp"
)

// ErrInvalidCommand is returned by Validate; the handler maps it to 400.
var ErrInvalidCommand = errors.New("invalid command")

// Validate checks a command is well-formed and in range.
//
// Done here rather than in a driver so every driver — including the out-of-process bridge —
// can trust what it receives. Kelvin is only sanity-checked; the per-bulb range is clamped by
// the driver, which knows the bulb.
func (c Command) Validate() error {
	switch c.Type {
	case CommandPower:
		if c.On == nil {
			return fmt.Errorf(`%w: "on" must be a boolean`, ErrInvalidCommand)
		}
	case CommandBrightness:
		if c.Value == nil || *c.Value < 0 || *c.Value > 100 {
			return fmt.Errorf(`%w: "value" must be between 0 and 100`, ErrInvalidCommand)
		}
	case CommandColor:
		if c.Color == nil {
			return fmt.Errorf(`%w: "color" is required`, ErrInvalidCommand)
		}
		for name, v := range map[string]int{"r": c.Color.R, "g": c.Color.G, "b": c.Color.B} {
			if v < 0 || v > 255 {
				return fmt.Errorf(`%w: "color.%s" must be between 0 and 255`, ErrInvalidCommand, name)
			}
		}
	case CommandColorTemp:
		if c.Kelvin == nil || *c.Kelvin < 1000 || *c.Kelvin > 20000 {
			return fmt.Errorf(`%w: "kelvin" must be a plausible colour temperature`, ErrInvalidCommand)
		}
	default:
		return fmt.Errorf("%w: unknown command type %q", ErrInvalidCommand, c.Type)
	}
	return nil
}
