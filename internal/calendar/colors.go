package calendar

import "sort"

/*
Google's own calendar colours are unusable here.

Every primary calendar comes back as the same pale cyan (#9fe1e7) and every holiday calendar as
the same green, so four connected accounts are indistinguishable, and a pastel picked for a white
UI disappears on a dark one. So gv assigns the colour instead: Google's value is still stored and
returned as background_color, but `color` — what the clients paint with — comes from this palette.

The palette is mid-tone on purpose: saturated enough to stand out on a dark background, dark
enough to carry light text. Which of the two a client writes on top is the client's decision,
computed from the colour's luminance.
*/
var calendarPalette = []string{
	"#3b82f6", // blue
	"#ef4444", // red
	"#10b981", // emerald
	"#a855f7", // purple
	"#f97316", // orange
	"#06b6d4", // cyan
	"#ec4899", // pink
	"#84cc16", // lime
	"#6366f1", // indigo
	"#14b8a6", // teal
	"#eab308", // yellow
	"#8b5cf6", // violet
}

/*
assignColors gives every calendar a colour, in place.

Assignment follows creation order (the row id), not a hash of the calendar's name: a hash is
stable but collides, and two calendars sharing a colour is exactly the problem being fixed. Going
by id keeps every existing calendar's colour when a new one appears — it simply takes the next
slot — which a hash would also do but only by luck.

An explicit override always wins: it is the user saying they want that colour there.
*/
func assignColors(views []CalendarView) {
	ordered := make([]int, len(views))
	for i := range views {
		ordered[i] = i
	}
	sort.SliceStable(ordered, func(a, b int) bool {
		return views[ordered[a]].ID < views[ordered[b]].ID
	})
	for slot, index := range ordered {
		views[index].AssignedColor = calendarPalette[slot%len(calendarPalette)]
	}
}

// paletteAt is the colour a calendar in the given position gets. Exported for the tests, which
// assert the assignment rather than restating the palette.
func paletteAt(position int) string {
	return calendarPalette[position%len(calendarPalette)]
}
