package calendar

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssignColors_FollowsCreationOrder(t *testing.T) {
	views := []CalendarView{
		{CalendarRecord: CalendarRecord{ID: 20}},
		{CalendarRecord: CalendarRecord{ID: 1}},
		{CalendarRecord: CalendarRecord{ID: 7}},
	}
	assignColors(views)

	// Whatever order the query returned them in, the colour follows the id.
	require.Equal(t, paletteAt(2), views[0].AssignedColor)
	require.Equal(t, paletteAt(0), views[1].AssignedColor)
	require.Equal(t, paletteAt(1), views[2].AssignedColor)
}

func TestAssignColors_DistinctUpToThePaletteThenRepeats(t *testing.T) {
	views := make([]CalendarView, len(calendarPalette)+2)
	for i := range views {
		views[i] = CalendarView{CalendarRecord: CalendarRecord{ID: int32(i + 1)}}
	}
	assignColors(views)

	seen := map[string]int{}
	for _, v := range views[:len(calendarPalette)] {
		seen[v.AssignedColor]++
	}
	require.Len(t, seen, len(calendarPalette), "the first N calendars all differ")
	// Past the palette it wraps rather than running out.
	require.Equal(t, views[0].AssignedColor, views[len(calendarPalette)].AssignedColor)
}

func TestDisplayColor_Precedence(t *testing.T) {
	view := CalendarView{
		CalendarRecord: CalendarRecord{BackgroundColor: "#9fe1e7"},
		AssignedColor:  "#3b82f6",
	}
	require.Equal(t, "#3b82f6", view.DisplayColor())

	view.ColorOverride = "#ff00ff"
	require.Equal(t, "#ff00ff", view.DisplayColor())

	// With nothing assigned yet (a path that reads a record directly), google's value is better
	// than nothing.
	bare := CalendarView{CalendarRecord: CalendarRecord{BackgroundColor: "#9fe1e7"}}
	require.Equal(t, "#9fe1e7", bare.DisplayColor())
}

func TestPalette_IsDarkEnoughForLightText(t *testing.T) {
	// Every colour has to carry white text on a chip; a pastel like google's #9fe1e7 does not,
	// which is what made the old ones unreadable.
	for _, hex := range calendarPalette {
		require.Less(t, relativeLuminance(t, hex), 0.62, "%s is too light to paint white text on", hex)
		require.Greater(t, relativeLuminance(t, hex), 0.05, "%s is too dark to read on a dark page", hex)
	}
}

// relativeLuminance is the WCAG formula, used only to keep the palette honest.
func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	require.Len(t, hex, 7)
	channel := func(offset int) float64 {
		value, err := strconv.ParseInt(hex[offset:offset+2], 16, 32)
		require.NoError(t, err)
		c := float64(value) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(1) + 0.7152*channel(3) + 0.0722*channel(5)
}
