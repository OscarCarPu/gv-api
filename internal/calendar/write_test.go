package calendar_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/calendar"
	"gv-api/internal/calendar/google"

	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

// --- creating ------------------------------------------------------------------------

func TestWrite_Create_GoesToGoogleFirstAndMirrors(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	ev, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1,
		Summary:    "Dentist",
		Location:   "Clínica",
		StartsAt:   madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
		EndsAt:     madrid(t, 2026, 8, 20, 18, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)
	require.Equal(t, "Dentist", ev.Summary)
	require.Equal(t, madrid(t, 2026, 8, 20, 17, 0).UTC(), ev.StartsAt.UTC())
	require.True(t, ev.CreatedByGV, "events made here are tagged as ours")
	require.NotEmpty(t, ev.GoogleEventID)

	stored, ok := h.gc.Event("me@example.com", primaryCal, ev.GoogleEventID)
	require.True(t, ok, "the event exists in google, not only locally")
	require.Equal(t, "Dentist", stored.Summary)
	require.Equal(t, gvMarker(stored), "1")
}

func gvMarker(ev google.Event) string {
	if ev.ExtendedProperties == nil || ev.ExtendedProperties.Private == nil {
		return ""
	}
	return ev.ExtendedProperties.Private["gv"]
}

func TestWrite_Create_AllDayDefaultsToOneDay(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	ev, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Festivo", AllDay: true, StartsAt: "2026-08-20",
	})
	require.NoError(t, err)
	require.True(t, ev.AllDay)
	require.Equal(t, madrid(t, 2026, 8, 20, 0, 0).UTC(), ev.StartsAt.UTC())
	require.Equal(t, madrid(t, 2026, 8, 21, 0, 0).UTC(), ev.EndsAt.UTC(),
		"an all-day event with no end covers one day, with an exclusive end")
}

func TestWrite_Create_RefusesReadOnlyCalendarWithoutCallingGoogle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), readerEntry(readOnlyCal, "Shared"))

	_, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 2, Summary: "Nope",
		StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
	})
	require.ErrorIs(t, err, calendar.ErrReadOnly)
	require.Zero(t, h.gc.CallCount("InsertEvent"),
		"a write we know google will refuse is not sent")
}

func TestWrite_Create_ValidatesTimes(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	_, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{CalendarID: 1, Summary: "x"})
	require.ErrorIs(t, err, calendar.ErrInvalidRange)

	_, err = h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "x",
		StartsAt: madrid(t, 2026, 8, 20, 10, 0).Format(time.RFC3339),
		EndsAt:   madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
	})
	require.ErrorIs(t, err, calendar.ErrInvalidRange)

	_, err = h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "x", StartsAt: "20/08/2026",
	})
	require.ErrorIs(t, err, calendar.ErrInvalidRange)
}

// --- updating ------------------------------------------------------------------------

func TestWrite_Update_PatchesOnlyWhatWasSent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Dentist", Description: "second floor",
		StartsAt: madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
		EndsAt:   madrid(t, 2026, 8, 20, 18, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	updated, err := h.svc.UpdateEvent(ctx, created.InstanceID, calendar.UpdateEventRequest{
		Summary: ptr("Dentist (moved)"),
	})
	require.NoError(t, err)
	require.Equal(t, "Dentist (moved)", updated.Summary)
	require.Equal(t, "second floor", updated.Description, "a field not sent is left alone")
	require.Equal(t, created.StartsAt.UTC(), updated.StartsAt.UTC())
}

func TestWrite_Update_MovingTheStartDragsTheEnd(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Call",
		StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
		EndsAt:   madrid(t, 2026, 8, 20, 9, 30).Format(time.RFC3339),
	})
	require.NoError(t, err)

	updated, err := h.svc.UpdateEvent(ctx, created.InstanceID, calendar.UpdateEventRequest{
		StartsAt: ptr(madrid(t, 2026, 8, 20, 11, 0).Format(time.RFC3339)),
	})
	require.NoError(t, err)
	require.Equal(t, madrid(t, 2026, 8, 20, 11, 0).UTC(), updated.StartsAt.UTC())
	require.Equal(t, madrid(t, 2026, 8, 20, 11, 30).UTC(), updated.EndsAt.UTC(),
		"the length is kept when only the start is given")
}

func TestWrite_Update_StaleEtagIsAConflict(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Dentist",
		StartsAt: madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	// Someone edits it from a phone: the stored etag is now behind.
	stored, ok := h.gc.Event("me@example.com", primaryCal, created.GoogleEventID)
	require.True(t, ok)
	stored.Summary = "Dentist (from the phone)"
	h.gc.PutEvent("me@example.com", primaryCal, stored)

	_, err = h.svc.UpdateEvent(ctx, created.InstanceID, calendar.UpdateEventRequest{
		Summary: ptr("Dentist (from gv)"),
	})
	require.ErrorIs(t, err, calendar.ErrConflict,
		"a lost update must be reported, not silently applied over someone else's change")

	inGoogle, _ := h.gc.Event("me@example.com", primaryCal, created.GoogleEventID)
	require.Equal(t, "Dentist (from the phone)", inGoogle.Summary, "the other change survives")
}

func TestWrite_Update_SingleOccurrenceLeavesTheRestAlone(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		EndsAt:     madrid(t, 2026, 8, 17, 9, 30).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)
	require.True(t, series.Recurring)

	// Wednesday only: move it two hours later and rename it.
	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	updated, err := h.svc.UpdateEvent(ctx, ref, calendar.UpdateEventRequest{
		Summary:  ptr("Standup (late)"),
		StartsAt: ptr(madrid(t, 2026, 8, 19, 11, 0).Format(time.RFC3339)),
	})
	require.NoError(t, err)
	require.Equal(t, "Standup (late)", updated.Summary)
	require.Equal(t, madrid(t, 2026, 8, 19, 11, 0).UTC(), updated.StartsAt.UTC())
	require.True(t, updated.IsException)

	week, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 17, 0, 0), To: madrid(t, 2026, 8, 24, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, week, 5, "the series still has five occurrences")
	changed, untouched := 0, 0
	for _, e := range week {
		if e.Summary == "Standup (late)" {
			changed++
			require.Equal(t, 11, e.StartsAt.In(h.loc).Hour())
		} else {
			untouched++
			require.Equal(t, "Standup", e.Summary)
			require.Equal(t, 9, e.StartsAt.In(h.loc).Hour())
		}
	}
	require.Equal(t, 1, changed)
	require.Equal(t, 4, untouched)
}

func TestWrite_Update_ThisAndFollowingSplitsTheSeries(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		EndsAt:     madrid(t, 2026, 8, 17, 9, 30).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)

	// From Wednesday on it is called something else.
	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	updated, err := h.svc.UpdateEvent(ctx, ref, calendar.UpdateEventRequest{
		Summary: ptr("Daily sync"),
		Scope:   calendar.ScopeFollowing,
	})
	require.NoError(t, err)
	require.Equal(t, "Daily sync", updated.Summary)

	week, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 17, 0, 0), To: madrid(t, 2026, 8, 24, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, week, 5, "the split must not lose or duplicate occurrences")

	old, renamed := 0, 0
	for _, e := range week {
		switch e.Summary {
		case "Standup":
			old++
			require.True(t, e.StartsAt.Before(madrid(t, 2026, 8, 19, 0, 0)))
		case "Daily sync":
			renamed++
			require.False(t, e.StartsAt.Before(madrid(t, 2026, 8, 19, 0, 0)))
		default:
			t.Fatalf("unexpected summary %q", e.Summary)
		}
	}
	require.Equal(t, 2, old, "monday and tuesday keep the old name")
	require.Equal(t, 3, renamed, "wednesday onwards is the new series; COUNT was carried over")
}

func TestWrite_Update_ScopeValidation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	plain, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "One off",
		StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)

	// Asking to edit "this occurrence" without saying which one is refused, not guessed:
	// guessing wrong there rewrites somebody's whole series.
	_, err = h.svc.UpdateEvent(ctx, series.InstanceID, calendar.UpdateEventRequest{
		Summary: ptr("x"), Scope: calendar.ScopeInstance,
	})
	require.ErrorIs(t, err, calendar.ErrInvalidScope)

	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	_, err = h.svc.UpdateEvent(ctx, ref, calendar.UpdateEventRequest{Scope: "sometimes", Summary: ptr("x")})
	require.ErrorIs(t, err, calendar.ErrInvalidScope)

	// A one-off has no occurrences to scope to.
	_, err = h.svc.UpdateEvent(ctx, plain.InstanceID+"@2026-08-20T07:00:00Z",
		calendar.UpdateEventRequest{Scope: calendar.ScopeFollowing, Summary: ptr("x")})
	require.ErrorIs(t, err, calendar.ErrInvalidScope)

	// The rule itself is a property of the series, so it can only be changed as a whole.
	_, err = h.svc.UpdateEvent(ctx, ref, calendar.UpdateEventRequest{
		Recurrence: &[]string{"RRULE:FREQ=WEEKLY"},
	})
	require.ErrorIs(t, err, calendar.ErrInvalidScope)
}

func TestWrite_Update_EmptyPatchIsRejected(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "x", StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	_, err = h.svc.UpdateEvent(ctx, created.InstanceID, calendar.UpdateEventRequest{})
	require.Error(t, err)
	require.Zero(t, h.gc.CallCount("PatchEvent"))
}

// --- deleting ------------------------------------------------------------------------

func TestWrite_Delete_WholeEvent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Dentist",
		StartsAt: madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	require.NoError(t, h.svc.DeleteEvent(ctx, created.InstanceID, "", ""))
	_, err = h.svc.GetEvent(ctx, created.InstanceID)
	require.ErrorIs(t, err, calendar.ErrNotFound)

	inGoogle, ok := h.gc.Event("me@example.com", primaryCal, created.GoogleEventID)
	require.True(t, ok)
	require.True(t, inGoogle.Cancelled(), "google keeps a cancelled row; we do not")
}

func TestWrite_Delete_SingleOccurrenceLeavesAHole(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		EndsAt:     madrid(t, 2026, 8, 17, 9, 30).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)

	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	require.NoError(t, h.svc.DeleteEvent(ctx, ref, calendar.ScopeInstance, ""))

	// The cancelled override has to be mirrored, or the occurrence comes back on the next
	// expansion.
	_, err = h.svc.SyncCalendar(ctx, 1, "manual")
	require.NoError(t, err)

	week, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 17, 0, 0), To: madrid(t, 2026, 8, 24, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, week, 4)
	for _, e := range week {
		require.NotEqual(t, madrid(t, 2026, 8, 19, 9, 0).UTC(), e.StartsAt.UTC())
	}
}

func TestWrite_Delete_ThisAndFollowingTruncatesTheSeries(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		EndsAt:     madrid(t, 2026, 8, 17, 9, 30).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)

	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	require.NoError(t, h.svc.DeleteEvent(ctx, ref, calendar.ScopeFollowing, ""))

	week, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 17, 0, 0), To: madrid(t, 2026, 8, 24, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, week, 2, "monday and tuesday survive, wednesday onwards is gone")
}

// --- moving --------------------------------------------------------------------------

func TestWrite_Move_WithinTheSameAccount(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), writableEntry("work@x", "Work"))
	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Review",
		StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	res, err := h.svc.MoveEvent(ctx, created.InstanceID, calendar.MoveEventRequest{CalendarID: 2})
	require.NoError(t, err)
	require.False(t, res.Recreated, "a same-account move keeps the event id")
	require.EqualValues(t, 2, res.Event.CalendarID)
	require.Equal(t, created.GoogleEventID, res.Event.GoogleEventID)
	require.Equal(t, 1, h.gc.CallCount("MoveEvent"))
}

func TestWrite_Move_AcrossAccountsRecreates(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.connect(t, "work@example.com", writableEntry("work@example.com", "Work"))

	created, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Review", Location: "Sala 2",
		StartsAt: madrid(t, 2026, 8, 20, 9, 0).Format(time.RFC3339),
		EndsAt:   madrid(t, 2026, 8, 20, 10, 0).Format(time.RFC3339),
	})
	require.NoError(t, err)

	res, err := h.svc.MoveEvent(ctx, created.InstanceID, calendar.MoveEventRequest{CalendarID: 2})
	require.NoError(t, err)
	require.True(t, res.Recreated, "there is no cross-account move in the api; it is recreated")
	require.EqualValues(t, 2, res.Event.CalendarID)
	require.NotEqual(t, created.GoogleEventID, res.Event.GoogleEventID, "a recreated event has a new id")
	require.Equal(t, "Review", res.Event.Summary)
	require.Equal(t, "Sala 2", res.Event.Location)
	require.Equal(t, created.StartsAt.UTC(), res.Event.StartsAt.UTC())
	require.Zero(t, h.gc.CallCount("MoveEvent"))

	gone, ok := h.gc.Event("me@example.com", primaryCal, created.GoogleEventID)
	require.True(t, ok)
	require.True(t, gone.Cancelled(), "the original is removed from the source calendar")
}

func TestWrite_Move_RefusesAnOccurrence(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), writableEntry("work@x", "Work"))
	series, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Standup",
		StartsAt:   madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339),
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	require.NoError(t, err)

	ref := series.InstanceID + "@" + madrid(t, 2026, 8, 19, 9, 0).UTC().Format(time.RFC3339)
	_, err = h.svc.MoveEvent(ctx, ref, calendar.MoveEventRequest{CalendarID: 2})
	require.ErrorIs(t, err, calendar.ErrInvalidScope)
}

func TestWrite_DerivedEventsAreReadOnly(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	// Birthdays are generated by Google from contacts; it rejects edits to them.
	h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:        "bday",
		Summary:   "Someone's birthday",
		EventType: "birthday",
		Start:     &google.EventDateTime{Date: "2026-08-20"},
		End:       &google.EventDateTime{Date: "2026-08-21"},
	})
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	events, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 20, 0, 0), To: madrid(t, 2026, 8, 21, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.False(t, events[0].Editable, "the client is told up front that it cannot edit this")

	_, err = h.svc.UpdateEvent(ctx, events[0].InstanceID, calendar.UpdateEventRequest{Summary: ptr("nope")})
	require.ErrorIs(t, err, calendar.ErrReadOnly)
	require.Zero(t, h.gc.CallCount("PatchEvent"))
}

func TestWrite_UpstreamRefusalDoesNotStoreAnything(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	h.gc.FailWith = &google.APIError{Status: 500, Message: "backend error"}
	_, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Dentist",
		StartsAt: madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
	})
	require.ErrorIs(t, err, calendar.ErrUpstream)
	require.Zero(t, h.repo.eventCount(1), "nothing is stored for an event google never accepted")
}

func TestWrite_WriteWhenTheGrantDiesParksTheAccount(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	h.gc.FailWith = &google.APIError{Status: 401, Reason: "authError", Message: "Invalid Credentials"}
	_, err := h.svc.CreateEvent(ctx, calendar.CreateEventRequest{
		CalendarID: 1, Summary: "Dentist",
		StartsAt: madrid(t, 2026, 8, 20, 17, 0).Format(time.RFC3339),
	})
	require.ErrorIs(t, err, calendar.ErrNeedsReauth)

	accounts, err := h.svc.ListAccounts(ctx)
	require.NoError(t, err)
	require.Equal(t, "needs_reauth", accounts[0].Status)
}
