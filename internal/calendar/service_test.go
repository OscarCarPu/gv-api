package calendar_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"gv-api/internal/calendar"
	"gv-api/internal/calendar/google"

	"github.com/stretchr/testify/require"
)

// A fixed key: these tests care that tokens survive a round trip through the cipher, not
// which key was used.
const testTokenKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const (
	primaryCal  = "me@example.com"
	readOnlyCal = "shared@group.calendar.google.com"
	holidayCal  = "es.spain#holiday@group.v.calendar.google.com"
)

type harness struct {
	svc  *calendar.Service
	repo *fakeRepo
	gc   *google.Fake
	loc  *time.Location
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)

	gc := google.NewFake()
	repo := newFakeRepo()
	svc, err := calendar.NewService(repo, gc, calendar.Config{
		ClientID:       "client",
		ClientSecret:   "secret",
		RedirectURL:    "https://api.example/calendar/google/callback",
		WebAppURL:      "https://app.example",
		TokenKey:       testTokenKey,
		StateSecret:    []byte("state-secret"),
		WebhookEnabled: true,
		WebhookURL:     "https://api.example/calendar/google/webhook",
	}, loc)
	require.NoError(t, err)
	return &harness{svc: svc, repo: repo, gc: gc, loc: loc}
}

// connect runs the real consent flow against the fake, so every test starts from the state a
// user would actually be in.
func (h *harness) connect(t *testing.T, email string, calendars ...google.CalendarListEntry) {
	t.Helper()
	code := h.gc.AddAccount(email, calendars...)
	out, err := h.svc.AuthURL(context.Background())
	require.NoError(t, err)
	parsed, err := url.Parse(out.URL)
	require.NoError(t, err)
	state := parsed.Query().Get("state")
	require.NotEmpty(t, state)

	redirect, err := h.svc.HandleCallback(context.Background(), code, state)
	require.NoError(t, err)
	require.Contains(t, redirect, "https://app.example/calendar?connected=")
}

func writableEntry(id, name string) google.CalendarListEntry {
	return google.CalendarListEntry{
		ID: id, Summary: name, TimeZone: "Europe/Madrid",
		AccessRole: "owner", Primary: id == primaryCal, BackgroundColor: "#3366cc",
	}
}

func readerEntry(id, name string) google.CalendarListEntry {
	return google.CalendarListEntry{ID: id, Summary: name, TimeZone: "Europe/Madrid", AccessRole: "reader"}
}

func madrid(t *testing.T, y int, m time.Month, d, hh, mm int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Madrid")
	require.NoError(t, err)
	return time.Date(y, m, d, hh, mm, 0, 0, loc)
}

func timedEvent(id, summary string, start, end time.Time) google.Event {
	return google.Event{
		ID:      id,
		Summary: summary,
		Start:   &google.EventDateTime{DateTime: start.Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		End:     &google.EventDateTime{DateTime: end.Format(time.RFC3339), TimeZone: "Europe/Madrid"},
	}
}

// --- connecting ----------------------------------------------------------------------

func TestService_Connect_StoresGrantAndImportsCalendars(t *testing.T) {
	h := newHarness(t)
	h.connect(t, "me@example.com",
		writableEntry(primaryCal, "Personal"),
		readerEntry(readOnlyCal, "Shared"),
		readerEntry(holidayCal, "Holidays in Spain"),
	)

	accounts, err := h.svc.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, "me@example.com", accounts[0].Email)
	require.Equal(t, "connected", accounts[0].Status)
	require.Equal(t, 3, accounts[0].Calendars)

	cals, err := h.svc.ListCalendars(context.Background())
	require.NoError(t, err)
	require.Len(t, cals, 3)

	byGoogleID := map[string]calendar.Calendar{}
	for _, c := range cals {
		byGoogleID[c.GoogleCalendarID] = c
	}
	require.True(t, byGoogleID[primaryCal].Writable, "owner role is writable")
	require.False(t, byGoogleID[readOnlyCal].Writable, "reader role is not writable")
	require.True(t, byGoogleID[primaryCal].SyncEnabled)
	require.False(t, byGoogleID[holidayCal].SyncEnabled,
		"the holidays calendar starts disabled: the initial import cannot be bounded by date")
}

func TestService_Connect_RefusesWhenGoogleReturnsNoRefreshToken(t *testing.T) {
	h := newHarness(t)
	// A consent that hands back no refresh token is useless in an hour; it must not be stored
	// as if it had worked.
	h.gc.AddAccount("me@example.com", writableEntry(primaryCal, "Personal"))
	out, err := h.svc.AuthURL(context.Background())
	require.NoError(t, err)
	state := mustState(t, out.URL)

	_, err = h.svc.HandleCallback(context.Background(), "unknown-code", state)
	require.Error(t, err)
	accounts, listErr := h.svc.ListAccounts(context.Background())
	require.NoError(t, listErr)
	require.Empty(t, accounts)
}

func TestService_Callback_RejectsForgedState(t *testing.T) {
	h := newHarness(t)
	code := h.gc.AddAccount("me@example.com", writableEntry(primaryCal, "Personal"))

	_, err := h.svc.HandleCallback(context.Background(), code, "not.a.state")
	require.ErrorIs(t, err, calendar.ErrInvalidState)
}

func TestService_NotConfigured_StillAnswers(t *testing.T) {
	loc := time.UTC
	svc, err := calendar.NewService(newFakeRepo(), google.NewFake(), calendar.Config{}, loc)
	require.NoError(t, err)
	require.False(t, svc.Configured())

	// Reads work and are empty; anything needing Google says so instead of failing obscurely.
	events, err := svc.ListEvents(context.Background(), calendar.EventsQuery{
		From: time.Now(), To: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.Empty(t, events)

	_, err = svc.AuthURL(context.Background())
	require.ErrorIs(t, err, calendar.ErrNotConfigured)
	_, err = svc.SyncAll(context.Background(), "manual")
	require.ErrorIs(t, err, calendar.ErrNotConfigured)
}

func TestService_NewService_RequiresTokenKeyWhenConfigured(t *testing.T) {
	_, err := calendar.NewService(newFakeRepo(), google.NewFake(), calendar.Config{
		ClientID: "id", ClientSecret: "secret",
	}, time.UTC)
	require.ErrorContains(t, err, "GOOGLE_TOKEN_KEY")
}

func mustState(t *testing.T, authURL string) string {
	t.Helper()
	parsed, err := url.Parse(authURL)
	require.NoError(t, err)
	return parsed.Query().Get("state")
}

// --- syncing -------------------------------------------------------------------------

func TestService_Sync_FullThenIncremental(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))

	res, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 1, res.Upserted)
	require.Equal(t, "full", h.repo.lastRun().Kind)

	// Nothing changed: an incremental pass costs one listing and stores nothing.
	res, err = h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Zero(t, res.Upserted)
	require.Equal(t, "incremental", h.repo.lastRun().Kind)

	// One new event: only that one comes back.
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("b", "Lunch", madrid(t, 2026, 8, 20, 14, 0), madrid(t, 2026, 8, 20, 15, 0)))
	res, err = h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 1, res.Upserted)
	require.Equal(t, 2, h.repo.eventCount(1))
}

func TestService_Sync_Paginates(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PageSize = 2

	for i := 0; i < 5; i++ {
		start := madrid(t, 2026, 8, 20+i, 9, 0)
		h.gc.PutEvent("me@example.com", primaryCal,
			timedEvent(string(rune('a'+i)), "Event", start, start.Add(time.Hour)))
	}

	res, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 5, res.Upserted)
	require.EqualValues(t, 3, h.repo.lastRun().Pages, "5 events at 2 per page is 3 pages")
}

func TestService_Sync_DeletionRemovesTheRow(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	created := h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 1, h.repo.eventCount(1))

	h.gc.DeleteEventDirect("me@example.com", primaryCal, created.ID)
	res, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 1, res.Deleted)
	require.Zero(t, h.repo.eventCount(1))
}

func TestService_Sync_ExpiredTokenTriggersFullResync(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	// Google forgets every token it handed out: the local copy has to be rebuilt.
	h.gc.ExpireSyncTokens()
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("b", "Lunch", madrid(t, 2026, 8, 20, 14, 0), madrid(t, 2026, 8, 20, 15, 0)))

	res, err := h.svc.SyncCalendar(ctx, 1, "manual")
	require.NoError(t, err)
	require.Equal(t, 2, res.Upserted, "both events come back on the rebuild")
	require.Equal(t, 2, h.repo.eventCount(1))
	require.Equal(t, "full", h.repo.lastRun().Kind)
	require.Nil(t, h.repo.lastRun().Error)
}

func TestService_Sync_InvalidGrantParksTheAccount(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	// Expire the cached access token so the next pass has to refresh, and make the refresh
	// fail the way a revoked grant does.
	require.NoError(t, h.repo.UpdateAccountAccessToken(ctx, 1, nil, time.Now().Add(-time.Hour)))
	h.gc.SetInvalidGrant("me@example.com", true)

	_, err = h.svc.SyncCalendar(ctx, 1, "poll")
	require.ErrorIs(t, err, calendar.ErrNeedsReauth)

	accounts, err := h.svc.ListAccounts(ctx)
	require.NoError(t, err)
	require.Equal(t, "needs_reauth", accounts[0].Status)
	require.NotNil(t, accounts[0].LastSyncError)

	// A parked account is skipped rather than retried into the ground.
	res, err := h.svc.SyncAll(ctx, "poll")
	require.NoError(t, err)
	require.Len(t, res.Errors, 1)
	require.Zero(t, res.Calendars)
}

func TestService_Sync_RecordsFailureInTheRunLog(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	h.gc.FailWith = &google.APIError{Status: 500, Message: "backend error"}
	_, err := h.svc.SyncCalendar(ctx, 1, "poll")
	require.Error(t, err)

	run := h.repo.lastRun()
	require.NotNil(t, run.Error)
	require.Contains(t, *run.Error, "500")

	cals, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.NotNil(t, cals[0].Sync.LastSyncError, "a failing calendar says so in its status")
}

func TestService_Sync_DisabledCalendarIsNotTouched(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), readerEntry(holidayCal, "Holidays"))
	h.gc.PutEvent("me@example.com", holidayCal,
		timedEvent("h1", "Fiesta", madrid(t, 2026, 8, 15, 0, 0), madrid(t, 2026, 8, 16, 0, 0)))

	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Zero(t, h.repo.eventCount(2), "a disabled calendar is never listed")
}

func TestService_DisablingACalendarDropsItsEventsAndChannel(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.NoError(t, h.svc.EnsureWatches(ctx))
	require.Len(t, h.gc.Channels(), 1)

	disabled := false
	cal, err := h.svc.UpdateCalendar(ctx, 1, calendar.UpdateCalendarRequest{SyncEnabled: &disabled})
	require.NoError(t, err)
	require.False(t, cal.SyncEnabled)
	require.Zero(t, h.repo.eventCount(1), "stale events must not linger looking current")
	require.False(t, cal.Sync.WatchActive)
	require.Empty(t, h.gc.Channels(), "the push channel is stopped, not left running")
}

// --- recurring series ----------------------------------------------------------------

func TestService_Sync_CancelledOccurrenceSurvivesAsAHole(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	master := h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:         "series",
		Summary:    "Daily standup",
		Start:      &google.EventDateTime{DateTime: madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		End:        &google.EventDateTime{DateTime: madrid(t, 2026, 8, 17, 9, 30).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	// Tuesday is cancelled: Google keeps a cancelled override, and so must we.
	h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:                "series_20260818T070000Z",
		Status:            "cancelled",
		RecurringEventID:  master.ID,
		OriginalStartTime: &google.EventDateTime{DateTime: madrid(t, 2026, 8, 18, 9, 0).Format(time.RFC3339)},
	})

	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.Equal(t, 2, h.repo.eventCount(1), "the master and the cancelled override are both stored")

	events, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 17, 0, 0), To: madrid(t, 2026, 8, 23, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, events, 4, "5 daily occurrences minus the cancelled one")
	for _, e := range events {
		require.NotEqual(t, madrid(t, 2026, 8, 18, 9, 0), e.StartsAt, "the cancelled day must not appear")
	}
}

func TestService_ListEvents_OverrideMovedIntoAndOutOfTheWindow(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	master := h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:         "series",
		Summary:    "Weekly",
		Start:      &google.EventDateTime{DateTime: madrid(t, 2026, 8, 17, 9, 0).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		End:        &google.EventDateTime{DateTime: madrid(t, 2026, 8, 17, 10, 0).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		Recurrence: []string{"RRULE:FREQ=WEEKLY;COUNT=4"},
	})
	// The 24th is moved a week later, to the 31st.
	h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:                "series_moved",
		Summary:           "Weekly (moved)",
		RecurringEventID:  master.ID,
		OriginalStartTime: &google.EventDateTime{DateTime: madrid(t, 2026, 8, 24, 9, 0).Format(time.RFC3339)},
		Start:             &google.EventDateTime{DateTime: madrid(t, 2026, 8, 31, 12, 0).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
		End:               &google.EventDateTime{DateTime: madrid(t, 2026, 8, 31, 13, 0).Format(time.RFC3339), TimeZone: "Europe/Madrid"},
	})
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	// The week it was moved out of shows nothing.
	week, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 24, 0, 0), To: madrid(t, 2026, 8, 25, 0, 0),
	})
	require.NoError(t, err)
	require.Empty(t, week, "an occurrence moved out of the window must not still appear in it")

	// The week it was moved into shows it, at its new time, with its original slot recorded.
	week, err = h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 31, 0, 0), To: madrid(t, 2026, 9, 1, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, week, 2, "the moved occurrence plus the series' own 31st")
	var moved calendar.Event
	for _, e := range week {
		if e.IsException {
			moved = e
		}
	}
	require.Equal(t, "Weekly (moved)", moved.Summary)
	require.Equal(t, madrid(t, 2026, 8, 31, 12, 0).UTC(), moved.StartsAt.UTC())
	require.NotNil(t, moved.OriginalStartsAt)
	require.Equal(t, madrid(t, 2026, 8, 24, 9, 0).UTC(), moved.OriginalStartsAt.UTC())
	require.Contains(t, moved.InstanceID, "@2026-08-24T07:00:00Z",
		"an occurrence is addressed by the slot it came from, not where it landed")
}

func TestService_ListEvents_AllDayAndRangeEdges(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	h.gc.PutEvent("me@example.com", primaryCal, google.Event{
		ID:      "trip",
		Summary: "Trip",
		Start:   &google.EventDateTime{Date: "2026-08-20"},
		End:     &google.EventDateTime{Date: "2026-08-23"}, // exclusive, like Google
	})
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	events, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 22, 0, 0), To: madrid(t, 2026, 8, 23, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "a multi-day event shows on every day it covers")
	require.True(t, events[0].AllDay)
	require.Equal(t, madrid(t, 2026, 8, 20, 0, 0).UTC(), events[0].StartsAt.UTC())

	// The day after it ends is outside: Google's end date is exclusive and so is ours.
	events, err = h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 23, 0, 0), To: madrid(t, 2026, 8, 24, 0, 0),
	})
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestService_ListEvents_RejectsAbsurdRanges(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.ListEvents(context.Background(), calendar.EventsQuery{
		From: madrid(t, 2026, 8, 20, 0, 0), To: madrid(t, 2026, 8, 20, 0, 0),
	})
	require.ErrorIs(t, err, calendar.ErrInvalidRange)

	_, err = h.svc.ListEvents(context.Background(), calendar.EventsQuery{
		From: madrid(t, 2020, 1, 1, 0, 0), To: madrid(t, 2030, 1, 1, 0, 0),
	})
	require.ErrorIs(t, err, calendar.ErrInvalidRange)
}

func TestService_ListEvents_FiltersByCalendarAndVisibility(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), writableEntry("other@x", "Other"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "One", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 10, 0)))
	h.gc.PutEvent("me@example.com", "other@x",
		timedEvent("b", "Two", madrid(t, 2026, 8, 20, 11, 0), madrid(t, 2026, 8, 20, 12, 0)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	day := calendar.EventsQuery{From: madrid(t, 2026, 8, 20, 0, 0), To: madrid(t, 2026, 8, 21, 0, 0)}
	all, err := h.svc.ListEvents(ctx, day)
	require.NoError(t, err)
	require.Len(t, all, 2)

	only := day
	only.CalendarIDs = []int32{1}
	filtered, err := h.svc.ListEvents(ctx, only)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "One", filtered[0].Summary)

	hidden := false
	_, err = h.svc.UpdateCalendar(ctx, 2, calendar.UpdateCalendarRequest{Visible: &hidden})
	require.NoError(t, err)
	visibleOnly := day
	visibleOnly.VisibleOnly = true
	shown, err := h.svc.ListEvents(ctx, visibleOnly)
	require.NoError(t, err)
	require.Len(t, shown, 1, "a hidden calendar drops out of a visible-only query")
}

// --- push notifications --------------------------------------------------------------

func TestService_EnsureWatches_CreatesAndRenews(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	require.NoError(t, h.svc.EnsureWatches(ctx))
	channels := h.gc.Channels()
	require.Len(t, channels, 1)
	var firstID string
	for id := range channels {
		firstID = id
	}

	// A second pass changes nothing while the channel is comfortably alive.
	require.NoError(t, h.svc.EnsureWatches(ctx))
	require.Len(t, h.gc.Channels(), 1)
	require.Equal(t, 1, h.gc.CallCount("Watch"))

	// Push the expiry inside the renewal window: a replacement is created and the old one is
	// stopped, because a channel cannot be renewed in place.
	soon := time.Now().Add(time.Hour)
	require.NoError(t, h.repo.SetCalendarWatch(ctx, 1, calendar.WatchInfo{
		ChannelID: firstID, ResourceID: "res-x", Token: "tok", ExpiresAt: soon,
	}))
	require.NoError(t, h.svc.EnsureWatches(ctx))
	require.Equal(t, 2, h.gc.CallCount("Watch"))
	require.Equal(t, 1, h.gc.CallCount("StopChannel"))
	renewed := h.gc.Channels()
	require.Len(t, renewed, 1)
	_, oldStillThere := renewed[firstID]
	require.False(t, oldStillThere)
}

func TestService_Webhook_QueuesOnlyValidNotifications(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	require.NoError(t, h.svc.EnsureWatches(ctx))

	cal, err := h.repo.GetCalendar(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, cal.WatchChannelID)
	require.NotNil(t, cal.WatchToken)

	// The handshake Google sends when a channel is created carries no news.
	require.NoError(t, h.svc.HandleWebhook(ctx, *cal.WatchChannelID, "sync", *cal.WatchToken))
	require.Empty(t, h.svc.Changes())

	require.NoError(t, h.svc.HandleWebhook(ctx, *cal.WatchChannelID, "exists", *cal.WatchToken))
	select {
	case id := <-h.svc.Changes():
		require.EqualValues(t, 1, id)
	default:
		t.Fatal("a real notification should have queued a sync")
	}

	// Anyone can POST to a public webhook; only the channel token makes it ours.
	err = h.svc.HandleWebhook(ctx, *cal.WatchChannelID, "exists", "wrong-token")
	require.ErrorIs(t, err, calendar.ErrInvalidState)
	require.Empty(t, h.svc.Changes())

	err = h.svc.HandleWebhook(ctx, "no-such-channel", "exists", "whatever")
	require.ErrorIs(t, err, calendar.ErrNotFound)
	require.Empty(t, h.svc.Changes())
}

// --- disconnecting -------------------------------------------------------------------

func TestService_DeleteAccount_RevokesStopsAndCleansUp(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.NoError(t, h.svc.EnsureWatches(ctx))

	require.NoError(t, h.svc.DeleteAccount(ctx, 1))
	require.True(t, h.gc.Revoked("me@example.com"), "the grant must not outlive the row")
	require.Empty(t, h.gc.Channels())

	accounts, err := h.svc.ListAccounts(ctx)
	require.NoError(t, err)
	require.Empty(t, accounts)
	require.Zero(t, h.repo.eventCount(1))
}

func TestService_SyncStatus_ReportsWhatMatters(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	require.NoError(t, h.svc.EnsureWatches(ctx))

	status, err := h.svc.SyncStatus(ctx)
	require.NoError(t, err)
	require.True(t, status.Configured)
	require.True(t, status.WebhooksActive)
	require.Len(t, status.Calendars, 1)
	require.EqualValues(t, 1, status.Calendars[0].Events)
	require.True(t, status.Calendars[0].Sync.HasSyncToken)
	require.True(t, status.Calendars[0].Sync.WatchActive)
	require.NotNil(t, status.Calendars[0].Sync.WatchExpiresAt)
	require.NotEmpty(t, status.RecentRuns)
}

func TestService_TokenRefresh_IsCachedUntilItNearlyExpires(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)
	_, err = h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	acc, err := h.repo.GetAccount(ctx, 1)
	require.NoError(t, err)
	require.NotEmpty(t, acc.AccessTokenSealed)
	require.NotContains(t, string(acc.AccessTokenSealed), "access-me@example.com",
		"tokens are encrypted at rest, not merely stored")
	require.NotContains(t, string(acc.RefreshTokenSealed), "refresh-me@example.com")
}

func TestService_ResyncAccount_RebuildsFromScratch(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	res, err := h.svc.ResyncAccount(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, res.Upserted)
	require.Equal(t, "full", h.repo.lastRun().Kind)
	require.Equal(t, 1, h.repo.eventCount(1))
}

func TestService_Sync_ConcurrentPassesDoNotOverlap(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := h.svc.SyncCalendar(ctx, 1, "webhook")
			done <- err
		}()
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, <-done)
	}
	// Whatever the interleaving, the calendar ends up with exactly one usable cursor and no
	// error: a second caller finding the lock taken is a no-op, not a failure.
	cal, err := h.repo.GetCalendar(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, cal.SyncToken)
	require.Nil(t, cal.LastSyncError)
}

func TestService_Sync_UnknownCalendarIsNotFound(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.SyncCalendar(context.Background(), 99, "manual")
	require.True(t, errors.Is(err, calendar.ErrNotFound))
}

func TestService_ImportCalendars_MarksVanishedOnesDeleted(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"), writableEntry("gone@x", "Shared once"))

	cals, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, cals, 2)

	// The second calendar stops being shared: it disappears from the list Google returns.
	h2 := h.gc
	_ = h2
	h.gc.AddAccount("me@example.com", writableEntry(primaryCal, "Personal"))
	_, err = h.svc.SyncAll(ctx, "poll")
	require.NoError(t, err)

	cals, err = h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, cals, 2, "the row stays so its events do not vanish unexplained")
	for _, c := range cals {
		if c.GoogleCalendarID == "gone@x" {
			require.True(t, c.Deleted)
		}
	}
}

func TestService_StreamPublishesOnChange(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	messages, unsubscribe := h.svc.Subscribe()
	defer unsubscribe()

	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Standup", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 9, 30)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	select {
	case msg := <-messages:
		require.Equal(t, "calendar.changed", msg.Type)
		require.EqualValues(t, 1, msg.CalendarID)
	case <-time.After(time.Second):
		t.Fatal("a sync that changed something should have told the stream")
	}
}

func TestService_StreamDoesNotBlockOnASlowClient(t *testing.T) {
	h := newHarness(t)
	_, unsubscribe := h.svc.Subscribe()
	defer unsubscribe()
	// Far more messages than the subscriber buffer holds: publishing must not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.svc.Stream().Publish(calendar.StreamMessage{Type: "calendar.changed"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publishing blocked on a subscriber that is not reading")
	}
}

func TestService_ConnectedAccountEmailIsTrimmedIntoTheRedirect(t *testing.T) {
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))
	out, err := h.svc.AuthURL(context.Background())
	require.NoError(t, err)
	require.True(t, strings.Contains(out.URL, "prompt=consent"),
		"without prompt=consent a re-connect returns no refresh token")
	require.True(t, strings.Contains(out.URL, "access_type=offline"))
}

// --- colours -------------------------------------------------------------------------

func TestService_Colors_AreAssignedNotTakenFromGoogle(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	// Google hands back the same pale cyan for every primary calendar and the same green for
	// every holiday one, which is the whole reason gv assigns its own.
	googleCyan := google.CalendarListEntry{
		ID: primaryCal, Summary: "Personal", TimeZone: "Europe/Madrid",
		AccessRole: "owner", Primary: true, BackgroundColor: "#9fe1e7",
	}
	otherCyan := google.CalendarListEntry{
		ID: "other@x", Summary: "Other", TimeZone: "Europe/Madrid",
		AccessRole: "owner", BackgroundColor: "#9fe1e7",
	}
	h.connect(t, "me@example.com", googleCyan, otherCyan)

	cals, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, cals, 2)
	require.NotEqual(t, cals[0].Color, cals[1].Color,
		"two calendars that share a colour in google must not share one here")
	for _, c := range cals {
		require.NotEqual(t, "#9fe1e7", c.Color, "google's pastel is not what we paint with")
		require.Equal(t, "#9fe1e7", c.BackgroundColor, "but it is still reported")
	}
}

func TestService_Colors_SurviveANewCalendarAppearing(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	before, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, before, 1)

	// A calendar shared with the account later must not repaint the ones already on screen.
	h.gc.AddAccount("me@example.com",
		writableEntry(primaryCal, "Personal"), writableEntry("new@x", "Newly shared"))
	_, err = h.svc.SyncAll(ctx, "poll")
	require.NoError(t, err)

	after, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, after, 2)
	require.Equal(t, before[0].Color, after[0].Color)
	require.NotEqual(t, after[0].Color, after[1].Color)
}

func TestService_Colors_OverrideWins(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com", writableEntry(primaryCal, "Personal"))

	override := "#ff00ff"
	_, err := h.svc.UpdateCalendar(ctx, 1, calendar.UpdateCalendarRequest{ColorOverride: &override})
	require.NoError(t, err)

	cals, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	require.Equal(t, override, cals[0].Color, "an explicit choice is not overruled by the palette")
}

func TestService_Colors_EventsMatchTheirCalendar(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.connect(t, "me@example.com",
		writableEntry(primaryCal, "Personal"), writableEntry("work@x", "Work"))
	h.gc.PutEvent("me@example.com", primaryCal,
		timedEvent("a", "Personal thing", madrid(t, 2026, 8, 20, 9, 0), madrid(t, 2026, 8, 20, 10, 0)))
	h.gc.PutEvent("me@example.com", "work@x",
		timedEvent("b", "Work thing", madrid(t, 2026, 8, 20, 11, 0), madrid(t, 2026, 8, 20, 12, 0)))
	_, err := h.svc.SyncAll(ctx, "manual")
	require.NoError(t, err)

	cals, err := h.svc.ListCalendars(ctx)
	require.NoError(t, err)
	byID := map[int32]string{}
	for _, c := range cals {
		byID[c.ID] = c.Color
	}

	events, err := h.svc.ListEvents(ctx, calendar.EventsQuery{
		From: madrid(t, 2026, 8, 20, 0, 0), To: madrid(t, 2026, 8, 21, 0, 0),
	})
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, e := range events {
		require.Equal(t, byID[e.CalendarID], e.Color,
			"an event's colour is its calendar's, or the two views disagree on screen")
	}
	require.NotEqual(t, events[0].Color, events[1].Color)
}
