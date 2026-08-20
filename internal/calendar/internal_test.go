package calendar

import (
	"strings"
	"testing"
	"time"

	"gv-api/internal/calendar/google"

	"github.com/stretchr/testify/require"
)

type (
	googleEvent = google.Event
	googleDT    = google.EventDateTime
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

func at(t *testing.T, loc *time.Location, y int, m time.Month, d, hh, mm int) time.Time {
	t.Helper()
	return time.Date(y, m, d, hh, mm, 0, 0, loc)
}

// --- recurrence expansion ------------------------------------------------------------

func TestExpandSeries_KeepsLocalTimeAcrossDST(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	master := EventRecord{
		StartsAt:   at(t, loc, 2026, 3, 23, 9, 0),
		EndsAt:     at(t, loc, 2026, 3, 23, 10, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=4"},
	}

	occ, err := expandSeries(master, nil, "Europe/Madrid",
		at(t, loc, 2026, 3, 1, 0, 0), at(t, loc, 2026, 5, 1, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 4)

	// The 29th of March is when Spain moves to summer time. Expanding in UTC would make every
	// occurrence after it happen an hour earlier in local terms.
	for _, o := range occ {
		require.Equal(t, 9, o.Start.In(loc).Hour(), "a 9am weekly stays at 9am")
		require.Equal(t, 10, o.End.In(loc).Hour())
	}
	require.Equal(t, "CET", tzAbbrev(occ[0].Start.In(loc)))
	require.Equal(t, "CEST", tzAbbrev(occ[1].Start.In(loc)))
}

func tzAbbrev(t time.Time) string {
	name, _ := t.Zone()
	return name
}

func TestExpandSeries_AllDayCrossesTheDSTDayCorrectly(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	// A daily all-day event over the change to summer time: that local day is 23 hours long,
	// so advancing by a fixed 24h would leave the end an hour past midnight.
	master := EventRecord{
		AllDay:     true,
		StartsAt:   at(t, loc, 2026, 3, 28, 0, 0),
		EndsAt:     at(t, loc, 2026, 3, 29, 0, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=3"},
	}

	occ, err := expandSeries(master, nil, "Europe/Madrid",
		at(t, loc, 2026, 3, 1, 0, 0), at(t, loc, 2026, 4, 15, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 3)
	for _, o := range occ {
		require.Equal(t, 0, o.Start.In(loc).Hour(), "an all-day event starts at midnight")
		require.Equal(t, 0, o.End.In(loc).Hour(), "and ends at midnight")
		require.Equal(t, o.Start.In(loc).Day()+1, o.End.In(loc).Day())
	}
}

func TestExpandSeries_ExdateWithDateOnlyValue(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	// Google writes EXDATE as a bare date for all-day series, which the rule parser will not
	// take; it has to be normalised first.
	master := EventRecord{
		AllDay:     true,
		StartsAt:   at(t, loc, 2026, 8, 17, 0, 0),
		EndsAt:     at(t, loc, 2026, 8, 18, 0, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5", "EXDATE;VALUE=DATE:20260819"},
	}

	occ, err := expandSeries(master, nil, "Europe/Madrid",
		at(t, loc, 2026, 8, 1, 0, 0), at(t, loc, 2026, 9, 1, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 4)
	for _, o := range occ {
		require.NotEqual(t, 19, o.Start.In(loc).Day())
	}
}

func TestExpandSeries_CancelledOverrideIsAHole(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	master := EventRecord{
		ID:         1,
		StartsAt:   at(t, loc, 2026, 8, 17, 9, 0),
		EndsAt:     at(t, loc, 2026, 8, 17, 9, 30),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=3"},
	}
	cancelledAt := at(t, loc, 2026, 8, 18, 9, 0)
	overrides := []EventRecord{{
		ID:               2,
		Status:           "cancelled",
		MasterID:         ptrTo(int32(1)),
		RecurringEventID: ptrTo("series"),
		OriginalStartsAt: &cancelledAt,
		StartsAt:         cancelledAt,
		EndsAt:           cancelledAt.Add(30 * time.Minute),
	}}

	occ, err := expandSeries(master, overrides, "Europe/Madrid",
		at(t, loc, 2026, 8, 17, 0, 0), at(t, loc, 2026, 8, 21, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 2)
	for _, o := range occ {
		require.False(t, o.Start.Equal(cancelledAt))
	}
}

func TestExpandSeries_OverrideMovedOutOfWindowDisappears(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	master := EventRecord{
		ID:         1,
		StartsAt:   at(t, loc, 2026, 8, 17, 9, 0),
		EndsAt:     at(t, loc, 2026, 8, 17, 10, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=3"},
	}
	slot := at(t, loc, 2026, 8, 18, 9, 0)
	moved := at(t, loc, 2026, 9, 10, 9, 0)
	overrides := []EventRecord{{
		ID:               2,
		Status:           "confirmed",
		MasterID:         ptrTo(int32(1)),
		RecurringEventID: ptrTo("series"),
		OriginalStartsAt: &slot,
		StartsAt:         moved,
		EndsAt:           moved.Add(time.Hour),
	}}

	// The week the occurrence used to be in.
	occ, err := expandSeries(master, overrides, "Europe/Madrid",
		at(t, loc, 2026, 8, 17, 0, 0), at(t, loc, 2026, 8, 21, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 2)

	// The week it went to: it shows up there, still identified by its original slot.
	occ, err = expandSeries(master, overrides, "Europe/Madrid",
		at(t, loc, 2026, 9, 7, 0, 0), at(t, loc, 2026, 9, 14, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 1)
	require.True(t, occ[0].Start.Equal(moved))
	require.True(t, occ[0].OriginalStart.Equal(slot))
}

func TestExpandSeries_IncludesAnOccurrenceStillRunningAtTheWindowStart(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	master := EventRecord{
		StartsAt:   at(t, loc, 2026, 8, 17, 23, 0),
		EndsAt:     at(t, loc, 2026, 8, 18, 1, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=3"},
	}
	// A window starting at midnight on the 18th: the occurrence that began at 23:00 the night
	// before is still going.
	occ, err := expandSeries(master, nil, "Europe/Madrid",
		at(t, loc, 2026, 8, 18, 0, 0), at(t, loc, 2026, 8, 19, 0, 0))
	require.NoError(t, err)
	require.Len(t, occ, 2)
	require.True(t, occ[0].Start.Equal(at(t, loc, 2026, 8, 17, 23, 0)))
}

func TestExpandSeries_UnparseableRuleIsAnError(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	master := EventRecord{
		StartsAt:   at(t, loc, 2026, 8, 17, 9, 0),
		EndsAt:     at(t, loc, 2026, 8, 17, 10, 0),
		StartTZ:    "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=NONSENSE"},
	}
	_, err := expandSeries(master, nil, "Europe/Madrid",
		at(t, loc, 2026, 8, 1, 0, 0), at(t, loc, 2026, 9, 1, 0, 0))
	require.Error(t, err)
}

func TestExpandSeries_UnknownZoneFallsBackInsteadOfFailing(t *testing.T) {
	master := EventRecord{
		StartsAt:   time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		EndsAt:     time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
		StartTZ:    "Mars/Olympus",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=2"},
	}
	occ, err := expandSeries(master, nil, "Also/Nonsense",
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, occ, 2)
}

func ptrTo[T any](v T) *T { return &v }

// --- rule rewriting ------------------------------------------------------------------

func TestEndSeriesBefore_SetsUntilAndDropsCount(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	lines, err := endSeriesBefore(
		[]string{"RRULE:FREQ=DAILY;COUNT=5", "EXDATE;TZID=Europe/Madrid:20260818T090000"},
		at(t, loc, 2026, 8, 19, 9, 0), EventRecord{StartTZ: "Europe/Madrid"}, loc)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "UNTIL=20260819T065959Z", "UNTIL is inclusive, so it lands a second early")
	require.NotContains(t, lines[0], "COUNT", "a rule cannot be bounded by both COUNT and UNTIL")
	require.Equal(t, "EXDATE;TZID=Europe/Madrid:20260818T090000", lines[1], "other lines are untouched")
}

func TestEndSeriesBefore_RequiresARule(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	_, err := endSeriesBefore(nil, time.Now(), EventRecord{}, loc)
	require.ErrorIs(t, err, ErrInvalidScope)

	_, err = endSeriesBefore([]string{"EXDATE;VALUE=DATE:20260101"}, time.Now(), EventRecord{}, loc)
	require.ErrorIs(t, err, ErrInvalidScope)
}

func TestRemainingRecurrence_CarriesOverWhatIsLeftOfTheCount(t *testing.T) {
	lines, err := remainingRecurrence([]string{"RRULE:FREQ=DAILY;COUNT=5"}, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"RRULE:FREQ=DAILY;COUNT=3"}, lines)

	// A rule with no COUNT is carried over as it is, and the original's exclusions are not:
	// they belong to slots that stayed with the old series.
	lines, err = remainingRecurrence([]string{"RRULE:FREQ=WEEKLY;BYDAY=MO", "EXDATE;VALUE=DATE:20260101"}, 3)
	require.NoError(t, err)
	require.Equal(t, []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"}, lines)

	// A split past the end of a counted series still leaves a usable rule.
	lines, err = remainingRecurrence([]string{"RRULE:FREQ=DAILY;COUNT=2"}, 5)
	require.NoError(t, err)
	require.Equal(t, []string{"RRULE:FREQ=DAILY;COUNT=1"}, lines)
}

func TestRewriteRRule_PreservesOrderAndOtherParts(t *testing.T) {
	out := rewriteRRule("RRULE:FREQ=WEEKLY;BYDAY=MO,WE;COUNT=10;WKST=MO",
		map[string]string{"UNTIL": "20260819T065959Z"}, []string{"COUNT"})
	require.True(t, strings.HasPrefix(out, "RRULE:FREQ=WEEKLY;BYDAY=MO,WE;WKST=MO"))
	require.Contains(t, out, "UNTIL=20260819T065959Z")
	require.NotContains(t, out, "COUNT")
}

func TestNormalizeRecurrenceLine_OnlyTouchesDateOnlyValues(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	require.Equal(t, "EXDATE;TZID=Europe/Madrid:20260101T000000,20260102T000000",
		normalizeRecurrenceLine("EXDATE;VALUE=DATE:20260101,20260102", loc))
	require.Equal(t, "RRULE:FREQ=DAILY", normalizeRecurrenceLine("RRULE:FREQ=DAILY", loc))
	require.Equal(t, "EXDATE;TZID=Europe/Madrid:20260101T090000",
		normalizeRecurrenceLine("EXDATE;TZID=Europe/Madrid:20260101T090000", loc))
}

// --- references ----------------------------------------------------------------------

func TestParseEventRef(t *testing.T) {
	id, start, err := parseEventRef("12")
	require.NoError(t, err)
	require.EqualValues(t, 12, id)
	require.Nil(t, start)

	id, start, err = parseEventRef("12@2026-08-20T07:00:00Z")
	require.NoError(t, err)
	require.EqualValues(t, 12, id)
	require.NotNil(t, start)
	require.Equal(t, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC), *start)

	// An offset is accepted and normalised to UTC, so the same slot always has one name.
	_, start, err = parseEventRef("12@2026-08-20T09:00:00+02:00")
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC), *start)

	for _, bad := range []string{"", "abc", "0", "-1", "12@yesterday", "12@"} {
		_, _, err := parseEventRef(bad)
		require.Error(t, err, "%q should not parse", bad)
	}
}

func TestInstanceRef_RoundTrips(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	original := at(t, loc, 2026, 8, 20, 9, 0)
	ref := instanceRef(7, original)
	require.Equal(t, "7@2026-08-20T07:00:00Z", ref)

	id, start, err := parseEventRef(ref)
	require.NoError(t, err)
	require.EqualValues(t, 7, id)
	require.True(t, start.Equal(original))
}

func TestResolveScope(t *testing.T) {
	scope, err := resolveScope("", false, false)
	require.NoError(t, err)
	require.Equal(t, ScopeAll, scope)

	scope, err = resolveScope("", true, true)
	require.NoError(t, err)
	require.Equal(t, ScopeInstance, scope, "naming an occurrence defaults to editing that occurrence")

	scope, err = resolveScope("", true, false)
	require.NoError(t, err)
	require.Equal(t, ScopeAll, scope, "a non-recurring event has nothing to scope")

	_, err = resolveScope(ScopeInstance, false, true)
	require.ErrorIs(t, err, ErrInvalidScope)
	_, err = resolveScope(ScopeFollowing, true, false)
	require.ErrorIs(t, err, ErrInvalidScope)
	_, err = resolveScope("everything", true, true)
	require.ErrorIs(t, err, ErrInvalidScope)
}

// --- token encryption ----------------------------------------------------------------

func TestTokenCipher_RoundTrip(t *testing.T) {
	c, err := newTokenCipher("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)

	sealed, err := c.seal("1//refresh-token-value")
	require.NoError(t, err)
	require.NotContains(t, string(sealed), "refresh-token-value", "the stored bytes must not be the token")

	opened, err := c.open(sealed)
	require.NoError(t, err)
	require.Equal(t, "1//refresh-token-value", opened)

	// The nonce is per call, so the same token seals differently every time.
	again, err := c.seal("1//refresh-token-value")
	require.NoError(t, err)
	require.NotEqual(t, sealed, again)

	empty, err := c.seal("")
	require.NoError(t, err)
	require.Nil(t, empty)
	opened, err = c.open(nil)
	require.NoError(t, err)
	require.Empty(t, opened)
}

func TestTokenCipher_RejectsBadKeysAndTamperedData(t *testing.T) {
	_, err := newTokenCipher("not-hex")
	require.ErrorContains(t, err, "hex")
	_, err = newTokenCipher("abcd")
	require.ErrorContains(t, err, "32 bytes")

	c, err := newTokenCipher("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	sealed, err := c.seal("secret")
	require.NoError(t, err)

	// A different key cannot read it, and the message says what to check first.
	other, err := newTokenCipher("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	require.NoError(t, err)
	_, err = other.open(sealed)
	require.ErrorContains(t, err, "GOOGLE_TOKEN_KEY")

	sealed[len(sealed)-1] ^= 0xff
	_, err = c.open(sealed)
	require.Error(t, err, "GCM must reject tampered ciphertext")

	_, err = c.open([]byte{1, 2, 3})
	require.ErrorContains(t, err, "truncated")
}

// --- google event mapping ------------------------------------------------------------

func TestEventParams_CancelledOverrideKeepsTimesFromWhatIsStored(t *testing.T) {
	loc := mustLoc(t, "Europe/Madrid")
	svc := &Service{loc: loc}
	cal := CalendarRecord{ID: 1, TimeZone: "Europe/Madrid"}

	slot := at(t, loc, 2026, 8, 18, 9, 0)
	existing := &EventRecord{
		StartsAt: slot, EndsAt: slot.Add(30 * time.Minute), StartTZ: "Europe/Madrid",
	}
	// This is all Google sends for a cancelled occurrence on an incremental sync.
	params, ok := svc.eventParams(cal, googleCancelledOverride(slot), existing)
	require.True(t, ok)
	require.Equal(t, "cancelled", params.Status)
	require.True(t, params.StartsAt.Equal(slot), "the stored times anchor a stripped-down cancellation")
	require.NotNil(t, params.OriginalStartsAt)

	// With nothing stored, the original start is enough to place it.
	params, ok = svc.eventParams(cal, googleCancelledOverride(slot), nil)
	require.True(t, ok)
	require.True(t, params.StartsAt.Equal(slot))
}

func TestEventParams_DropsAnEventWithNothingToPlaceItBy(t *testing.T) {
	svc := &Service{loc: time.UTC}
	_, ok := svc.eventParams(CalendarRecord{ID: 1, TimeZone: "UTC"}, googleEmptyCancelled(), nil)
	require.False(t, ok, "a row with no time is not renderable and would break the range index")
}

func googleCancelledOverride(slot time.Time) googleEvent {
	return googleEvent{
		ID:                "series_cancelled",
		Status:            "cancelled",
		RecurringEventID:  "series",
		OriginalStartTime: &googleDT{DateTime: slot.Format(time.RFC3339)},
	}
}

func googleEmptyCancelled() googleEvent {
	return googleEvent{ID: "gone", Status: "cancelled", RecurringEventID: "series"}
}
