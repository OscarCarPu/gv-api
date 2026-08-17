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

// Light is one bulb as the server knows it: a row in the lights table.
//
// Address, Protocol and Options describe hardware, so they never leave this process — clients
// only ever see the fields PublicLight exposes.
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

	// Options carries model-specific quirks (characteristic UUIDs, keys) so one odd bulb is a
	// row rather than a new protocol.
	Options map[string]any
}

// Public is the client-safe view, with no address and nothing about the wire protocol.
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

// offlineState is the baseline for a bulb that has not answered.
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

/*
slugify turns a name into an id.

Ids are slugs rather than numbers because they are what appears in URLs, in logs and as the
key three clients hold their local state under — "bedroom" reads better than "7" in all
three. It is assigned once at creation and never follows a rename, so a client mid-command
never has the ground move under it.

Accents are folded rather than dropped: "Salón" must become "salon", not "saln".
*/
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
		// Every name was punctuation or a script we cannot fold. The bulb still needs an id,
		// and the caller makes it unique.
		return "light"
	}
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug
}

// foldedLatin covers the accented letters a Spanish or Galician room name actually uses.
// A general Unicode normaliser would be a dependency for this one table.
var foldedLatin = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

// normalizeAddress makes BLE addresses comparable: BlueZ and every scanner report them
// uppercase and colon-separated, but people paste them in all sorts of ways.
func normalizeAddress(address string) string {
	return strings.ToUpper(strings.TrimSpace(address))
}
