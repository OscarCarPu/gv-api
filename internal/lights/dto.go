package lights

import (
	"errors"
	"fmt"
	"strings"
)

// RGB is a colour in the 0-255 sRGB space the bulbs speak.
type RGB struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

// State is one bulb as reported to clients.
type State struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
	// When Online is false the remaining fields are the last known values.
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
	// Error is per bulb so one unreachable bulb does not fail the whole request.
	Error     string `json:"error,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
	// Crazy is server state: any manual command ends it.
	Crazy bool `json:"crazy"`
}

// Discovered is a bulb the adapter can see right now. The address is exposed so a person can
// tell nameless lamps apart; it stops being public once the bulb is added.
type Discovered struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	RSSI    int    `json:"rssi"`
	Known   bool   `json:"known"`
	// Advertised GATT services, used to filter bulbs; not sent.
	Services []string `json:"-"`
}

// ProtocolInfo describes one supported bulb family, for the add form.
type ProtocolInfo struct {
	Name              string  `json:"name"`
	Label             string  `json:"label"`
	SupportsColor     bool    `json:"supportsColor"`
	SupportsColorTemp bool    `json:"supportsColorTemp"`
	MinColorTemp      float64 `json:"minColorTemp"`
	MaxColorTemp      float64 `json:"maxColorTemp"`
}

// CreateLightRequest adds a bulb. Fields past the first three override the protocol defaults.
type CreateLightRequest struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Protocol string `json:"protocol"`

	Model             *string        `json:"model,omitempty"`
	SupportsColor     *bool          `json:"supportsColor,omitempty"`
	SupportsColorTemp *bool          `json:"supportsColorTemp,omitempty"`
	MinColorTemp      *float64       `json:"minColorTemp,omitempty"`
	MaxColorTemp      *float64       `json:"maxColorTemp,omitempty"`
	Options           map[string]any `json:"options,omitempty"`
}

// UpdateLightRequest edits a bulb. Omitted fields keep their value; the address cannot change.
type UpdateLightRequest struct {
	Name              *string        `json:"name,omitempty"`
	Model             *string        `json:"model,omitempty"`
	Protocol          *string        `json:"protocol,omitempty"`
	SupportsColor     *bool          `json:"supportsColor,omitempty"`
	SupportsColorTemp *bool          `json:"supportsColorTemp,omitempty"`
	MinColorTemp      *float64       `json:"minColorTemp,omitempty"`
	MaxColorTemp      *float64       `json:"maxColorTemp,omitempty"`
	Options           map[string]any `json:"options,omitempty"`
}

type DiscoveredResponse struct {
	Devices []Discovered `json:"devices"`
}

func (r CreateLightRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf(`%w: "name" is required`, ErrInvalidCommand)
	}
	if strings.TrimSpace(r.Address) == "" {
		return fmt.Errorf(`%w: "address" is required`, ErrInvalidCommand)
	}
	if _, ok := protocolFor(r.Protocol); !ok {
		return fmt.Errorf(`%w: unknown protocol %q`, ErrInvalidCommand, r.Protocol)
	}
	return validateKelvinRange(r.MinColorTemp, r.MaxColorTemp)
}

func (r UpdateLightRequest) Validate() error {
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		return fmt.Errorf(`%w: "name" cannot be empty`, ErrInvalidCommand)
	}
	if r.Protocol != nil {
		if _, ok := protocolFor(*r.Protocol); !ok {
			return fmt.Errorf(`%w: unknown protocol %q`, ErrInvalidCommand, *r.Protocol)
		}
	}
	return validateKelvinRange(r.MinColorTemp, r.MaxColorTemp)
}

// validateKelvinRange checks the pair; a single bound is clamped at apply time.
func validateKelvinRange(minKelvin, maxKelvin *float64) error {
	if minKelvin != nil && maxKelvin != nil && *minKelvin >= *maxKelvin {
		return fmt.Errorf(`%w: "minColorTemp" must be below "maxColorTemp"`, ErrInvalidCommand)
	}
	for _, kelvin := range []*float64{minKelvin, maxKelvin} {
		if kelvin != nil && (*kelvin < 1000 || *kelvin > 20000) {
			return fmt.Errorf(`%w: colour temperatures must be plausible`, ErrInvalidCommand)
		}
	}
	return nil
}

// PublicLight is a registered bulb without any live state or its BLE address.
type PublicLight struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Model             string  `json:"model"`
	SupportsColor     bool    `json:"supportsColor"`
	SupportsColorTemp bool    `json:"supportsColorTemp"`
	MinColorTemp      float64 `json:"minColorTemp"`
	MaxColorTemp      float64 `json:"maxColorTemp"`
}

type StatesResponse struct {
	States []State `json:"states"`
}

// Command is one instruction for a bulb. Type picks which value field applies.
type Command struct {
	Type   string `json:"type"`
	On     *bool  `json:"on,omitempty"`
	Value  *int   `json:"value,omitempty"`
	Color  *RGB   `json:"color,omitempty"`
	Kelvin *int   `json:"kelvin,omitempty"`
}

const (
	CommandPower      = "power"
	CommandBrightness = "brightness"
	CommandColor      = "color"
	CommandColorTemp  = "colorTemp"
	// CommandCrazy is handled by the service; a driver never sees it.
	CommandCrazy = "crazy"
)

var ErrInvalidCommand = errors.New("invalid command")

// Validate checks a command is well-formed. Kelvin is only sanity-checked; the driver clamps
// it to the bulb's range.
func (c Command) Validate() error {
	switch c.Type {
	case CommandPower, CommandCrazy:
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
