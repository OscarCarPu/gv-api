package lights

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// Kelvin bounds used when a bulb's entry does not state its own.
const (
	DefaultMinKelvin = 2200
	DefaultMaxKelvin = 6500
)

// Config is one bulb as configured server-side.
//
// Address and Options identify hardware, so they never leave this process: clients only ever
// see the fields PublicLight exposes.
type Config struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	Address string `json:"address"` // BLE MAC the bridge connects to
	// Protocol names the wire format the bridge dispatches on, e.g. "lexman" or "mock".
	Protocol          string         `json:"protocol"`
	SupportsColor     *bool          `json:"supportsColor"`
	SupportsColorTemp *bool          `json:"supportsColorTemp"`
	MinColorTemp      *float64       `json:"minColorTemp"`
	MaxColorTemp      *float64       `json:"maxColorTemp"`
	Options           map[string]any `json:"options,omitempty"`
}

// Registry is the set of configured bulbs, resolved once at startup.
type Registry struct {
	lights []Config
	byID   map[string]Config
}

// ParseRegistry reads the LIGHTS env value: a JSON array of bulbs, so adding one is an env
// change and a restart rather than a code edit.
//
//	LIGHTS=[{"id":"bedroom","name":"Bedroom","address":"AA:BB:...","protocol":"lexman", ...}]
//
// Malformed entries are skipped with a warning rather than failing startup: one bad bulb
// should not take the whole API down.
func ParseRegistry(raw string) *Registry {
	reg := &Registry{byID: map[string]Config{}}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return reg
	}

	var parsed []Config
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		slog.Error("LIGHTS is not valid JSON — no bulbs configured", "error", err)
		return reg
	}

	for i, light := range parsed {
		light.ID = strings.TrimSpace(light.ID)
		light.Address = strings.TrimSpace(light.Address)
		if light.ID == "" || light.Address == "" {
			slog.Warn("LIGHTS entry needs both id and address — skipped", "index", i)
			continue
		}
		if _, dup := reg.byID[light.ID]; dup {
			slog.Warn("LIGHTS duplicate id — skipped", "index", i, "id", light.ID)
			continue
		}
		reg.byID[normalize(&light)] = light
		reg.lights = append(reg.lights, light)
	}
	return reg
}

// normalize fills in everything optional so drivers can read a complete Config, and returns
// the id for map insertion.
func normalize(l *Config) string {
	if l.Name == "" {
		l.Name = l.ID
	}
	if l.Model == "" {
		l.Model = "BLE bulb"
	}
	if l.Protocol == "" {
		l.Protocol = "generic"
	}
	// Capabilities default to present: a bulb that lacks one says so explicitly.
	if l.SupportsColor == nil {
		l.SupportsColor = boolPtr(true)
	}
	if l.SupportsColorTemp == nil {
		l.SupportsColorTemp = boolPtr(true)
	}
	if l.MinColorTemp == nil {
		l.MinColorTemp = floatPtr(DefaultMinKelvin)
	}
	if l.MaxColorTemp == nil {
		l.MaxColorTemp = floatPtr(DefaultMaxKelvin)
	}
	return l.ID
}

// All returns every configured bulb.
func (r *Registry) All() []Config { return r.lights }

// Get looks up one bulb; ok is false when the id is unknown.
func (r *Registry) Get(id string) (Config, bool) {
	l, ok := r.byID[id]
	return l, ok
}

// Public returns the client-safe view, with no addresses.
func (r *Registry) Public() []PublicLight {
	out := make([]PublicLight, 0, len(r.lights))
	for _, l := range r.lights {
		out = append(out, PublicLight{
			ID:                l.ID,
			Name:              l.Name,
			Model:             l.Model,
			SupportsColor:     *l.SupportsColor,
			SupportsColorTemp: *l.SupportsColorTemp,
			MinColorTemp:      *l.MinColorTemp,
			MaxColorTemp:      *l.MaxColorTemp,
		})
	}
	return out
}

// offlineState is the baseline for a bulb that has never answered.
func offlineState(l Config, errMsg string, now int64) State {
	mode := "white"
	if *l.SupportsColor {
		mode = "color"
	}
	return State{
		ID:                l.ID,
		Name:              l.Name,
		Model:             l.Model,
		Online:            false,
		Power:             false,
		Brightness:        0,
		Mode:              mode,
		Color:             RGB{R: 255, G: 255, B: 255},
		ColorTemp:         *l.MinColorTemp,
		SupportsColor:     *l.SupportsColor,
		SupportsColorTemp: *l.SupportsColorTemp,
		MinColorTemp:      *l.MinColorTemp,
		MaxColorTemp:      *l.MaxColorTemp,
		Error:             errMsg,
		UpdatedAt:         now,
	}
}

func boolPtr(v bool) *bool        { return &v }
func floatPtr(v float64) *float64 { return &v }
