package lights

import (
	"strings"
	"unicode"
)

// Kelvin bounds used when neither the bulb's row nor its protocol states its own.
const (
	DefaultMinKelvin = 2200
	DefaultMaxKelvin = 6500
)

// Light is one row of the lights table. Address, Protocol and Options never leave this process;
// clients see PublicLight.
type Light struct {
	ID       string
	Name     string
	Model    string
	Address  string // BLE address
	Protocol string // which wire format drives it, e.g. "lexman"

	SupportsColor     bool
	SupportsColorTemp bool
	MinColorTemp      float64
	MaxColorTemp      float64

	// Options carries model-specific quirks (characteristic UUIDs, keys).
	Options map[string]any
}

// Public is the client-safe view.
func (l Light) Public() PublicLight {
	return PublicLight{
		ID:                l.ID,
		Name:              l.Name,
		Model:             l.Model,
		SupportsColor:     l.SupportsColor,
		SupportsColorTemp: l.SupportsColorTemp,
		MinColorTemp:      l.MinColorTemp,
		MaxColorTemp:      l.MaxColorTemp,
	}
}

func publicLights(lights []Light) []PublicLight {
	out := make([]PublicLight, 0, len(lights))
	for _, l := range lights {
		out = append(out, l.Public())
	}
	return out
}

func offlineState(l Light, errMsg string, now int64) State {
	mode := "white"
	if l.SupportsColor {
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
		ColorTemp:         l.MinColorTemp,
		SupportsColor:     l.SupportsColor,
		SupportsColorTemp: l.SupportsColorTemp,
		MinColorTemp:      l.MinColorTemp,
		MaxColorTemp:      l.MaxColorTemp,
		Error:             errMsg,
		UpdatedAt:         now,
	}
}

// slugify turns a name into an id, folding accents: "Salón" -> "salon".
func slugify(name string) string {
	var b strings.Builder
	lastDash := true // leading dashes are dropped

	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if folded, ok := foldedLatin[r]; ok {
			r = folded
		}
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "light"
	}
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug
}

var foldedLatin = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

// normalizeAddress makes BLE addresses comparable: uppercase, colon-separated.
func normalizeAddress(address string) string {
	return strings.ToUpper(strings.TrimSpace(address))
}
