package calendar_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/calendar"
	"gv-api/internal/testutil"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Integration tests cover the SQL: the upserts, the range predicates and the cascades. The
// in-memory repository the service tests use reproduces this behaviour, and these are what
// keep the two honest.

func newCalendarRepo(t *testing.T) (*calendar.PostgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "calendar_sync_runs", "calendar_events", "calendars", "google_accounts")
	return calendar.NewRepository(pool), pool
}

func seedAccountAndCalendar(t *testing.T, repo *calendar.PostgresRepository) (calendar.AccountRecord, calendar.CalendarRecord) {
	t.Helper()
	ctx := context.Background()
	acc, err := repo.UpsertAccount(ctx, calendar.UpsertAccountParams{
		Email:              "me@example.com",
		RefreshTokenSealed: []byte{1, 2, 3},
		Scopes:             "calendar",
	})
	require.NoError(t, err)
	cal, err := repo.UpsertCalendar(ctx, calendar.UpsertCalendarParams{
		AccountID:        acc.ID,
		GoogleCalendarID: "me@example.com",
		Summary:          "Personal",
		TimeZone:         "Europe/Madrid",
		AccessRole:       "owner",
		IsPrimary:        true,
		SyncEnabled:      true,
	})
	require.NoError(t, err)
	return acc, cal
}

func utc(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, time.UTC)
}

func insertEvent(t *testing.T, repo *calendar.PostgresRepository, p calendar.UpsertEventParams) calendar.EventRecord {
	t.Helper()
	if p.Status == "" {
		p.Status = "confirmed"
	}
	if p.EventType == "" {
		p.EventType = "default"
	}
	if p.Attendees == nil {
		p.Attendees = []byte("[]")
	}
	if p.Reminders == nil {
		p.Reminders = []byte("{}")
	}
	rec, err := repo.UpsertEvent(context.Background(), p)
	require.NoError(t, err)
	return rec
}

func TestIntegration_Calendar_AccountRoundTripAndReconnect(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)

	acc, err := repo.UpsertAccount(ctx, calendar.UpsertAccountParams{
		Email: "me@example.com", RefreshTokenSealed: []byte("sealed-1"), Scopes: "calendar",
	})
	require.NoError(t, err)
	require.Equal(t, "connected", acc.Status)

	// Parking the account and then reconnecting it must clear the error, not stack on top of it.
	msg := "invalid_grant"
	require.NoError(t, repo.UpdateAccountStatus(ctx, acc.ID, "needs_reauth", &msg))
	parked, err := repo.GetAccount(ctx, acc.ID)
	require.NoError(t, err)
	require.Equal(t, "needs_reauth", parked.Status)

	again, err := repo.UpsertAccount(ctx, calendar.UpsertAccountParams{
		Email: "me@example.com", RefreshTokenSealed: []byte("sealed-2"), Scopes: "calendar",
	})
	require.NoError(t, err)
	require.Equal(t, acc.ID, again.ID, "reconnecting is an update, not a second account")
	require.Equal(t, "connected", again.Status)
	require.Nil(t, again.LastSyncError)
	require.Equal(t, []byte("sealed-2"), again.RefreshTokenSealed)

	_, err = repo.GetAccount(ctx, 99999)
	require.ErrorIs(t, err, calendar.ErrNotFound)
}

func TestIntegration_Calendar_UpsertKeepsLocalPreferences(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	acc, cal := seedAccountAndCalendar(t, repo)

	off, hidden, colour := false, false, "#ff0000"
	_, err := repo.UpdateCalendarPrefs(ctx, cal.ID, calendar.CalendarPrefs{
		SyncEnabled: &off, Visible: &hidden, ColorOverride: &colour,
	})
	require.NoError(t, err)

	// A later calendarList pass refreshes Google's metadata and must not undo the user's
	// choices.
	updated, err := repo.UpsertCalendar(ctx, calendar.UpsertCalendarParams{
		AccountID:        acc.ID,
		GoogleCalendarID: "me@example.com",
		Summary:          "Personal (renamed)",
		TimeZone:         "Europe/Madrid",
		AccessRole:       "writer",
		SyncEnabled:      true,
	})
	require.NoError(t, err)
	require.Equal(t, cal.ID, updated.ID)
	require.Equal(t, "Personal (renamed)", updated.Summary)
	require.Equal(t, "writer", updated.AccessRole)
	require.False(t, updated.SyncEnabled, "sync on/off is the user's, not google's")
	require.False(t, updated.Visible)
	require.Equal(t, "#ff0000", updated.ColorOverride)
}

func TestIntegration_Calendar_MarkDeletedAndRevive(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	acc, cal := seedAccountAndCalendar(t, repo)
	second, err := repo.UpsertCalendar(ctx, calendar.UpsertCalendarParams{
		AccountID: acc.ID, GoogleCalendarID: "shared@x", Summary: "Shared",
		TimeZone: "Europe/Madrid", AccessRole: "reader", SyncEnabled: true,
	})
	require.NoError(t, err)

	require.NoError(t, repo.MarkCalendarsDeleted(ctx, acc.ID, []string{"me@example.com"}))
	gone, err := repo.GetCalendar(ctx, second.ID)
	require.NoError(t, err)
	require.NotNil(t, gone.DeletedAt)
	kept, err := repo.GetCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.Nil(t, kept.DeletedAt)

	// It comes back when the calendar is shared again.
	revived, err := repo.UpsertCalendar(ctx, calendar.UpsertCalendarParams{
		AccountID: acc.ID, GoogleCalendarID: "shared@x", Summary: "Shared",
		TimeZone: "Europe/Madrid", AccessRole: "reader", SyncEnabled: true,
	})
	require.NoError(t, err)
	require.Nil(t, revived.DeletedAt)

	// An empty "seen" list marks everything deleted rather than failing on an empty array.
	require.NoError(t, repo.MarkCalendarsDeleted(ctx, acc.ID, nil))
	all, err := repo.ListCalendarsByAccount(ctx, acc.ID)
	require.NoError(t, err)
	for _, c := range all {
		require.NotNil(t, c.DeletedAt)
	}
}

func TestIntegration_Calendar_SyncableAndWatchQueries(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	acc, cal := seedAccountAndCalendar(t, repo)

	syncable, err := repo.ListSyncableCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, syncable, 1)

	// A parked account takes its calendars out of the rotation.
	require.NoError(t, repo.UpdateAccountStatus(ctx, acc.ID, "needs_reauth", nil))
	syncable, err = repo.ListSyncableCalendars(ctx)
	require.NoError(t, err)
	require.Empty(t, syncable)
	require.NoError(t, repo.UpdateAccountStatus(ctx, acc.ID, "connected", nil))

	// No channel yet: it needs one.
	due, err := repo.ListCalendarsNeedingWatch(ctx, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, due, 1)

	require.NoError(t, repo.SetCalendarWatch(ctx, cal.ID, calendar.WatchInfo{
		ChannelID: "chan-1", ResourceID: "res-1", Token: "tok", ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}))
	due, err = repo.ListCalendarsNeedingWatch(ctx, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, due, "a healthy channel is left alone")

	byChannel, err := repo.GetCalendarByChannel(ctx, "chan-1")
	require.NoError(t, err)
	require.Equal(t, cal.ID, byChannel.ID)
	require.NotNil(t, byChannel.WatchToken)
	_, err = repo.GetCalendarByChannel(ctx, "nope")
	require.ErrorIs(t, err, calendar.ErrNotFound)

	// Inside the renewal window it comes back up for replacement.
	require.NoError(t, repo.SetCalendarWatch(ctx, cal.ID, calendar.WatchInfo{
		ChannelID: "chan-1", ResourceID: "res-1", Token: "tok", ExpiresAt: time.Now().Add(time.Hour),
	}))
	due, err = repo.ListCalendarsNeedingWatch(ctx, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, due, 1)

	require.NoError(t, repo.ClearCalendarWatch(ctx, cal.ID))
	watched, err := repo.ListWatchedCalendars(ctx)
	require.NoError(t, err)
	require.Empty(t, watched)
}

func TestIntegration_Calendar_EventUpsertIsIdempotentPerGoogleID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	first := insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "a", Etag: `"1"`, Summary: "Standup",
		StartsAt: utc(2026, 8, 20, 7, 0), EndsAt: utc(2026, 8, 20, 7, 30),
		StartTZ: "Europe/Madrid", EndTZ: "Europe/Madrid", CreatedByGV: true,
	})
	second := insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "a", Etag: `"2"`, Summary: "Standup (renamed)",
		StartsAt: utc(2026, 8, 20, 8, 0), EndsAt: utc(2026, 8, 20, 8, 30),
		StartTZ: "Europe/Madrid", EndTZ: "Europe/Madrid",
	})
	require.Equal(t, first.ID, second.ID, "the same google event is one row")
	require.Equal(t, `"2"`, second.Etag)
	require.Equal(t, "Standup (renamed)", second.Summary)
	require.True(t, second.CreatedByGV, "the ours-marker is sticky; google does not send it back")

	count, err := repo.CountEvents(ctx, cal.ID)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
}

func TestIntegration_Calendar_RangeQueryEdges(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	base := calendar.UpsertEventParams{CalendarID: cal.ID, StartTZ: "UTC", EndTZ: "UTC"}
	mk := func(id string, start, end time.Time) {
		p := base
		p.GoogleEventID = id
		p.Summary = id
		p.StartsAt, p.EndsAt = start, end
		insertEvent(t, repo, p)
	}
	// A window of the 20th, 09:00 to 10:00.
	mk("inside", utc(2026, 8, 20, 9, 15), utc(2026, 8, 20, 9, 45))
	mk("straddles-start", utc(2026, 8, 20, 8, 0), utc(2026, 8, 20, 9, 30))
	mk("straddles-end", utc(2026, 8, 20, 9, 45), utc(2026, 8, 20, 11, 0))
	mk("covers", utc(2026, 8, 20, 7, 0), utc(2026, 8, 20, 12, 0))
	mk("ends-exactly-at-start", utc(2026, 8, 20, 8, 0), utc(2026, 8, 20, 9, 0))
	mk("starts-exactly-at-end", utc(2026, 8, 20, 10, 0), utc(2026, 8, 20, 11, 0))
	mk("zero-length-inside", utc(2026, 8, 20, 9, 30), utc(2026, 8, 20, 9, 30))
	mk("before", utc(2026, 8, 19, 9, 0), utc(2026, 8, 19, 10, 0))

	found, err := repo.ListEventsInRange(ctx, []int32{cal.ID}, utc(2026, 8, 20, 9, 0), utc(2026, 8, 20, 10, 0))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, e := range found {
		names[e.Summary] = true
	}
	require.True(t, names["inside"])
	require.True(t, names["straddles-start"])
	require.True(t, names["straddles-end"])
	require.True(t, names["covers"])
	require.True(t, names["zero-length-inside"], "an instant inside the window is inside it")
	require.False(t, names["ends-exactly-at-start"], "an event ending as the window opens is outside")
	require.False(t, names["starts-exactly-at-end"], "the window's end is exclusive")
	require.False(t, names["before"])
}

func TestIntegration_Calendar_MastersAndOverrides(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	master := insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "series", Summary: "Standup",
		StartsAt: utc(2026, 8, 17, 7, 0), EndsAt: utc(2026, 8, 17, 7, 30),
		StartTZ: "Europe/Madrid", EndTZ: "Europe/Madrid",
		Recurrence: []string{"RRULE:FREQ=DAILY;COUNT=5"},
	})
	slot := utc(2026, 8, 19, 7, 0)
	seriesID := "series"
	override := insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "series_x", Summary: "Standup (late)",
		StartsAt: utc(2026, 8, 19, 9, 0), EndsAt: utc(2026, 8, 19, 9, 30),
		StartTZ: "Europe/Madrid", EndTZ: "Europe/Madrid",
		RecurringEventID: &seriesID, OriginalStartsAt: &slot,
	})

	// A master is not returned by the one-off query, and an override is not either.
	singles, err := repo.ListEventsInRange(ctx, []int32{cal.ID}, utc(2026, 8, 17, 0, 0), utc(2026, 8, 24, 0, 0))
	require.NoError(t, err)
	require.Empty(t, singles)

	masters, err := repo.ListRecurringMasters(ctx, []int32{cal.ID}, utc(2026, 8, 24, 0, 0))
	require.NoError(t, err)
	require.Len(t, masters, 1)
	require.Equal(t, master.ID, masters[0].ID)
	require.Equal(t, []string{"RRULE:FREQ=DAILY;COUNT=5"}, masters[0].Recurrence)

	// Before linking, the override is an orphan and must still be findable.
	orphans, err := repo.ListOrphanOverridesInRange(ctx, []int32{cal.ID},
		utc(2026, 8, 19, 0, 0), utc(2026, 8, 20, 0, 0))
	require.NoError(t, err)
	require.Len(t, orphans, 1)

	require.NoError(t, repo.LinkEventMasters(ctx, cal.ID))
	linked, err := repo.GetEvent(ctx, override.ID)
	require.NoError(t, err)
	require.NotNil(t, linked.MasterID)
	require.Equal(t, master.ID, *linked.MasterID)
	require.NotNil(t, linked.OriginalStartsAt)
	require.True(t, linked.OriginalStartsAt.Equal(slot))

	exceptions, err := repo.ListEventExceptions(ctx, []int32{master.ID})
	require.NoError(t, err)
	require.Len(t, exceptions, 1)

	orphans, err = repo.ListOrphanOverridesInRange(ctx, []int32{cal.ID},
		utc(2026, 8, 19, 0, 0), utc(2026, 8, 20, 0, 0))
	require.NoError(t, err)
	require.Empty(t, orphans, "once linked it is no longer an orphan")

	// Deleting the master takes its overrides with it: they only mean anything as holes in it.
	require.NoError(t, repo.DeleteEvent(ctx, master.ID))
	count, err := repo.CountEvents(ctx, cal.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestIntegration_Calendar_CascadesFromAccountToEvents(t *testing.T) {
	ctx := context.Background()
	repo, pool := newCalendarRepo(t)
	acc, cal := seedAccountAndCalendar(t, repo)
	insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "a", Summary: "One",
		StartsAt: utc(2026, 8, 20, 7, 0), EndsAt: utc(2026, 8, 20, 8, 0),
	})
	runID, err := repo.CreateSyncRun(ctx, cal.ID, "manual", "full")
	require.NoError(t, err)
	require.NoError(t, repo.FinishSyncRun(ctx, runID, 1, 1, 0, nil))

	require.NoError(t, repo.DeleteAccount(ctx, acc.ID))

	var calendars, events, runs int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM calendars").Scan(&calendars))
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM calendar_events").Scan(&events))
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM calendar_sync_runs").Scan(&runs))
	require.Zero(t, calendars)
	require.Zero(t, events)
	require.Zero(t, runs)

	require.ErrorIs(t, repo.DeleteAccount(ctx, acc.ID), calendar.ErrNotFound)
}

func TestIntegration_Calendar_SyncTokenBookkeeping(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	require.NoError(t, repo.SetCalendarSyncError(ctx, cal.ID, "boom"))
	failed, err := repo.GetCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.NotNil(t, failed.LastSyncError)

	require.NoError(t, repo.SetCalendarSyncToken(ctx, cal.ID, "tok-1", true))
	full, err := repo.GetCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.NotNil(t, full.SyncToken)
	require.Equal(t, "tok-1", *full.SyncToken)
	require.NotNil(t, full.LastFullSyncAt)
	require.Nil(t, full.LastSyncError, "a good pass clears the last error")

	fullStamp := *full.LastFullSyncAt
	require.NoError(t, repo.SetCalendarSyncToken(ctx, cal.ID, "tok-2", false))
	incremental, err := repo.GetCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.Equal(t, "tok-2", *incremental.SyncToken)
	require.True(t, incremental.LastFullSyncAt.Equal(fullStamp),
		"an incremental pass does not pretend to be a full one")

	require.NoError(t, repo.ClearAccountSyncTokens(ctx, cal.AccountID))
	cleared, err := repo.GetCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.SyncToken, "no token means the next pass is a full sync")
}

func TestIntegration_Calendar_PurgeOnlyTouchesStandaloneCancellations(t *testing.T) {
	ctx := context.Background()
	repo, pool := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "old-cancelled", Status: "cancelled",
		StartsAt: utc(2026, 1, 1, 9, 0), EndsAt: utc(2026, 1, 1, 10, 0),
	})
	seriesID := "series"
	slot := utc(2026, 1, 2, 9, 0)
	insertEvent(t, repo, calendar.UpsertEventParams{
		CalendarID: cal.ID, GoogleEventID: "cancelled-instance", Status: "cancelled",
		RecurringEventID: &seriesID, OriginalStartsAt: &slot,
		StartsAt: slot, EndsAt: slot.Add(time.Hour),
	})
	// Age both rows past the retention window. The touch trigger has to be stood down for
	// this: it exists precisely to stop updated_at being written by hand.
	_, err := pool.Exec(ctx, `
		ALTER TABLE calendar_events DISABLE TRIGGER calendar_events_touch_updated_at;
		UPDATE calendar_events SET updated_at = now() - interval '200 days';
		ALTER TABLE calendar_events ENABLE TRIGGER calendar_events_touch_updated_at;`)
	require.NoError(t, err)

	purged, err := repo.PurgeCancelledEvents(ctx, time.Now().Add(-90*24*time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 1, purged)

	_, err = repo.GetEventByGoogleID(ctx, cal.ID, "old-cancelled")
	require.ErrorIs(t, err, calendar.ErrNotFound)
	kept, err := repo.GetEventByGoogleID(ctx, cal.ID, "cancelled-instance")
	require.NoError(t, err)
	require.Equal(t, "cancelled", kept.Status,
		"a cancelled occurrence is a hole in a live series and must outlive the retention window")
}

func TestIntegration_Calendar_DeleteEventsForCalendarAndByGoogleID(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)
	for _, id := range []string{"a", "b", "c"} {
		insertEvent(t, repo, calendar.UpsertEventParams{
			CalendarID: cal.ID, GoogleEventID: id,
			StartsAt: utc(2026, 8, 20, 7, 0), EndsAt: utc(2026, 8, 20, 8, 0),
		})
	}

	require.NoError(t, repo.DeleteEventByGoogleID(ctx, cal.ID, "b"))
	count, err := repo.CountEvents(ctx, cal.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

	// Deleting one that is already gone is not an error: the sync learns of deletions it may
	// have already applied.
	require.NoError(t, repo.DeleteEventByGoogleID(ctx, cal.ID, "b"))

	wiped, err := repo.DeleteEventsForCalendar(ctx, cal.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, wiped)
}

func TestIntegration_Calendar_ListCalendarsJoinsTheAccount(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	acc, _ := seedAccountAndCalendar(t, repo)
	label, colour := "personal", "#123456"
	_, err := repo.UpdateAccountMeta(ctx, acc.ID, &label, &colour)
	require.NoError(t, err)

	views, err := repo.ListCalendars(ctx)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "me@example.com", views[0].AccountEmail)
	require.Equal(t, "personal", views[0].AccountLabel)
	require.Equal(t, "#123456", views[0].AccountColor)
	require.Equal(t, "connected", views[0].AccountStatus)
}

func TestIntegration_Calendar_SyncRunsAreRecorded(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCalendarRepo(t)
	_, cal := seedAccountAndCalendar(t, repo)

	okRun, err := repo.CreateSyncRun(ctx, cal.ID, "poll", "incremental")
	require.NoError(t, err)
	require.NoError(t, repo.FinishSyncRun(ctx, okRun, 2, 7, 1, nil))

	failed, err := repo.CreateSyncRun(ctx, cal.ID, "webhook", "full")
	require.NoError(t, err)
	msg := "410 fullSyncRequired"
	require.NoError(t, repo.FinishSyncRun(ctx, failed, 1, 0, 0, &msg))

	runs, err := repo.ListRecentSyncRuns(ctx, 10)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Equal(t, "webhook", runs[0].Trigger, "newest first")
	require.NotNil(t, runs[0].Error)
	require.EqualValues(t, 7, runs[1].Upserted)
	require.EqualValues(t, 1, runs[1].Deleted)
	require.NotNil(t, runs[1].FinishedAt)
}
