package calendar

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gv-api/internal/calendar/google"
)

// syncPageSize is Google's maximum. Fewer pages means fewer round-trips and, on a full sync
// of a busy calendar, a materially shorter run.
const syncPageSize = 2500

/*
SyncAll brings every syncable calendar up to date.

It also re-reads each account's calendar list, because Google sends no notification when a
calendar is created, shared or unshared — the only way to notice is to look.
*/
func (s *Service) SyncAll(ctx context.Context, trigger string) (SyncResult, error) {
	if err := s.requireConfigured(); err != nil {
		return SyncResult{}, err
	}

	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{}
	for _, acc := range accounts {
		// A parked account is reported rather than skipped in silence: this is what a manual
		// "sync now" answers with, and "0 calendars, no errors" would hide the one thing the
		// user has to act on.
		if acc.Status != "connected" {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", acc.Email, ErrNeedsReauth))
			continue
		}
		token, err := s.accessTokenFor(ctx, acc)
		if err != nil {
			// A parked account is expected and already recorded; do not fail the whole pass.
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", acc.Email, err))
			continue
		}
		if err := s.importCalendars(ctx, acc, token); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: calendar list: %v", acc.Email, err))
		}
	}

	calendars, err := s.repo.ListSyncableCalendars(ctx)
	if err != nil {
		return result, err
	}
	for _, cal := range calendars {
		res, err := s.SyncCalendar(ctx, cal.ID, trigger)
		result.Calendars += res.Calendars
		result.Upserted += res.Upserted
		result.Deleted += res.Deleted
		result.Errors = append(result.Errors, res.Errors...)
		if err != nil && !errors.Is(err, ErrNeedsReauth) {
			result.Errors = append(result.Errors, fmt.Sprintf("calendar %d: %v", cal.ID, err))
		}
	}

	// Cancelled one-offs are dropped as they arrive, so this only ever sweeps leftovers from
	// a write path that stored one. It is cheap and it keeps the table honest.
	if _, err := s.repo.PurgeCancelledEvents(ctx, s.now().Add(-s.cfg.CancelledRetention)); err != nil {
		slog.ErrorContext(ctx, "calendar: purging cancelled events", "error", err)
	}
	return result, nil
}

// SyncCalendar syncs one calendar, taking the per-calendar lock so a push notification and
// the poll tick cannot spend the same sync token twice.
func (s *Service) SyncCalendar(ctx context.Context, calendarID int32, trigger string) (SyncResult, error) {
	if err := s.requireConfigured(); err != nil {
		return SyncResult{}, err
	}
	cal, err := s.repo.GetCalendar(ctx, calendarID)
	if err != nil {
		return SyncResult{}, err
	}
	if !cal.SyncEnabled || cal.DeletedAt != nil {
		return SyncResult{}, nil
	}

	if !s.lockCalendar(calendarID) {
		// Someone is already on it. Skipping is right: the run in flight will pick up
		// whatever this trigger was about.
		slog.DebugContext(ctx, "calendar: sync already running", "calendar", calendarID)
		return SyncResult{}, nil
	}
	defer s.unlockCalendar(calendarID)

	acc, err := s.repo.GetAccount(ctx, cal.AccountID)
	if err != nil {
		return SyncResult{}, err
	}
	token, err := s.accessTokenFor(ctx, acc)
	if err != nil {
		return SyncResult{}, err
	}

	res, err := s.syncOnce(ctx, cal, token, trigger)
	if err != nil && google.IsGone(err) {
		// The token is spent: the local copy of this calendar is not trustworthy any more, so
		// it is thrown away and rebuilt. This is the documented recovery, and holiday
		// calendars in particular go through it regularly.
		slog.InfoContext(ctx, "calendar: sync token expired, doing a full resync",
			"calendar", cal.ID, "summary", cal.Summary)
		if _, delErr := s.repo.DeleteEventsForCalendar(ctx, cal.ID); delErr != nil {
			return res, delErr
		}
		if clrErr := s.repo.ClearCalendarSyncToken(ctx, cal.ID); clrErr != nil {
			return res, clrErr
		}
		cal.SyncToken = nil
		res, err = s.syncOnce(ctx, cal, token, trigger)
	}
	if err != nil {
		msg := err.Error()
		if setErr := s.repo.SetCalendarSyncError(ctx, cal.ID, msg); setErr != nil {
			slog.ErrorContext(ctx, "calendar: recording sync error", "calendar", cal.ID, "error", setErr)
		}
		if touchErr := s.repo.TouchAccountSync(ctx, acc.ID, &msg); touchErr != nil {
			slog.ErrorContext(ctx, "calendar: touching account", "account", acc.ID, "error", touchErr)
		}
		return res, err
	}

	if err := s.repo.TouchAccountSync(ctx, acc.ID, nil); err != nil {
		slog.ErrorContext(ctx, "calendar: touching account", "account", acc.ID, "error", err)
	}
	if res.Upserted > 0 || res.Deleted > 0 {
		s.stream.Publish(StreamMessage{Type: "calendar.changed", CalendarID: cal.ID, AccountEmail: acc.Email})
	}
	return res, nil
}

/*
syncOnce runs one full or incremental pass over a calendar.

The parameters are fixed rather than derived from the caller: singleEvents=false because a
series is stored as its master plus overrides and expanded on read, showDeleted=true because
deletions are only ever learned from the incremental stream, and no time bounds at all
because Google refuses a syncToken next to timeMin/timeMax — using them once would break
every incremental sync that followed.
*/
func (s *Service) syncOnce(ctx context.Context, cal CalendarRecord, accessToken, trigger string) (SyncResult, error) {
	kind := "incremental"
	if cal.SyncToken == nil {
		kind = "full"
	}
	runID, err := s.repo.CreateSyncRun(ctx, cal.ID, trigger, kind)
	if err != nil {
		return SyncResult{}, err
	}

	var (
		pages     int32
		upserted  int32
		deleted   int32
		pageToken string
		syncToken string
	)
	finish := func(runErr error) {
		var msg *string
		if runErr != nil {
			m := runErr.Error()
			msg = &m
		}
		if err := s.repo.FinishSyncRun(ctx, runID, pages, upserted, deleted, msg); err != nil {
			slog.ErrorContext(ctx, "calendar: finishing sync run", "run", runID, "error", err)
		}
	}

	for {
		params := google.ListEventsParams{
			PageToken:    pageToken,
			MaxResults:   syncPageSize,
			ShowDeleted:  true,
			SingleEvents: false,
		}
		if cal.SyncToken != nil {
			params.SyncToken = *cal.SyncToken
		}
		page, err := s.gc.ListEvents(ctx, accessToken, cal.GoogleCalendarID, params)
		if err != nil {
			finish(err)
			return SyncResult{Calendars: 1, Upserted: int(upserted), Deleted: int(deleted)}, err
		}
		pages++

		for _, ev := range page.Items {
			changed, wasDeleted, err := s.applyGoogleEvent(ctx, cal, ev)
			if err != nil {
				finish(err)
				return SyncResult{Calendars: 1, Upserted: int(upserted), Deleted: int(deleted)}, err
			}
			if wasDeleted {
				deleted++
			} else if changed {
				upserted++
			}
		}

		if page.NextPageToken != "" {
			pageToken = page.NextPageToken
			continue
		}
		syncToken = page.NextSyncToken
		break
	}

	// Overrides can arrive before the master they belong to, so the link is resolved once the
	// whole pass has landed rather than per row.
	if err := s.repo.LinkEventMasters(ctx, cal.ID); err != nil {
		finish(err)
		return SyncResult{Calendars: 1}, err
	}

	if syncToken != "" {
		if err := s.repo.SetCalendarSyncToken(ctx, cal.ID, syncToken, kind == "full"); err != nil {
			finish(err)
			return SyncResult{Calendars: 1}, err
		}
	} else {
		// No token means the next pass has to be a full sync; better a wasted listing than a
		// silently frozen calendar.
		slog.WarnContext(ctx, "calendar: google returned no sync token", "calendar", cal.ID)
		if err := s.repo.ClearCalendarSyncToken(ctx, cal.ID); err != nil {
			slog.ErrorContext(ctx, "calendar: clearing sync token", "calendar", cal.ID, "error", err)
		}
	}

	finish(nil)
	return SyncResult{Calendars: 1, Upserted: int(upserted), Deleted: int(deleted)}, nil
}

/*
applyGoogleEvent stores one event from a sync page.

Cancellations are the interesting case. A cancelled *override* is a hole in a live series and
has to be kept for as long as the series exists, or the occurrence it removed comes back on
the next expansion. A cancelled one-off is just gone, so the local row goes with it.
*/
func (s *Service) applyGoogleEvent(ctx context.Context, cal CalendarRecord, ev google.Event) (changed, wasDeleted bool, err error) {
	if ev.ID == "" {
		return false, false, nil
	}

	isOverride := ev.RecurringEventID != ""
	if ev.Cancelled() && !isOverride {
		existing, getErr := s.repo.GetEventByGoogleID(ctx, cal.ID, ev.ID)
		if getErr != nil {
			if errors.Is(getErr, ErrNotFound) {
				// Cancelled before we ever saw it: nothing to remove.
				return false, false, nil
			}
			return false, false, getErr
		}
		if err := s.repo.DeleteEventByGoogleID(ctx, cal.ID, existing.GoogleEventID); err != nil {
			return false, false, err
		}
		return false, true, nil
	}

	var existing *EventRecord
	if found, getErr := s.repo.GetEventByGoogleID(ctx, cal.ID, ev.ID); getErr == nil {
		existing = &found
	} else if !errors.Is(getErr, ErrNotFound) {
		return false, false, getErr
	}

	params, ok := s.eventParams(cal, ev, existing)
	if !ok {
		return false, false, nil
	}
	if _, err := s.repo.UpsertEvent(ctx, params); err != nil {
		return false, false, err
	}
	return true, false, nil
}

/*
eventParams maps a Google event onto a row.

A cancelled override arrives stripped down — often just id, status and originalStartTime — so
the times come from whatever is already stored, and failing that from the original start. If
there is nothing to anchor it to at all it is dropped: a row with no time is not something any
view can render, and it would break the NOT NULL that keeps range queries simple.
*/
func (s *Service) eventParams(cal CalendarRecord, ev google.Event, existing *EventRecord) (UpsertEventParams, bool) {
	fallbackTZ := firstNonEmpty(cal.TimeZone, s.loc.String())

	start, startTZ, allDay, haveStart := parseEventTime(ev.Start, fallbackTZ)
	end, endTZ, _, haveEnd := parseEventTime(ev.End, fallbackTZ)

	var originalStart *time.Time
	if ev.OriginalStartTime != nil {
		if t, _, _, ok := parseEventTime(ev.OriginalStartTime, fallbackTZ); ok {
			originalStart = &t
		}
	}

	switch {
	case haveStart:
	case existing != nil:
		start, allDay, startTZ = existing.StartsAt, existing.AllDay, existing.StartTZ
		if !haveEnd {
			end, endTZ, haveEnd = existing.EndsAt, existing.EndTZ, true
		}
	case originalStart != nil:
		start, startTZ = *originalStart, fallbackTZ
	default:
		return UpsertEventParams{}, false
	}

	if !haveEnd {
		if allDay {
			loc := resolveLocation(startTZ, fallbackTZ)
			local := start.In(loc)
			end = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
		} else {
			end = start
		}
		endTZ = startTZ
	}

	attendees := make([]Attendee, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		attendees = append(attendees, Attendee{
			Email:          a.Email,
			DisplayName:    a.DisplayName,
			Optional:       a.Optional,
			ResponseStatus: a.ResponseStatus,
			Self:           a.Self,
			Organizer:      a.Organizer,
		})
	}
	reminders := Reminders{UseDefault: true}
	if ev.Reminders != nil {
		reminders.UseDefault = ev.Reminders.UseDefault
		for _, o := range ev.Reminders.Overrides {
			reminders.Overrides = append(reminders.Overrides, ReminderOverride{Method: o.Method, Minutes: o.Minutes})
		}
	}

	createdByGV := false
	if ev.ExtendedProperties != nil && ev.ExtendedProperties.Private != nil {
		createdByGV = ev.ExtendedProperties.Private[gvMarkerKey] == gvMarkerValue
	}
	if existing != nil {
		createdByGV = createdByGV || existing.CreatedByGV
	}

	p := UpsertEventParams{
		CalendarID:     cal.ID,
		GoogleEventID:  ev.ID,
		ICalUID:        ev.ICalUID,
		Etag:           ev.Etag,
		Sequence:       int32(ev.Sequence),
		Status:         firstNonEmpty(ev.Status, "confirmed"),
		EventType:      firstNonEmpty(ev.EventType, "default"),
		Summary:        ev.Summary,
		Description:    ev.Description,
		Location:       ev.Location,
		AllDay:         allDay,
		StartsAt:       start,
		EndsAt:         end,
		StartTZ:        startTZ,
		EndTZ:          firstNonEmpty(endTZ, startTZ),
		Recurrence:     ev.Recurrence,
		OrganizerEmail: personEmail(ev.Organizer),
		CreatorEmail:   personEmail(ev.Creator),
		Attendees:      jsonOrEmpty(attendees, "[]"),
		Reminders:      jsonOrEmpty(reminders, "{}"),
		Transparency:   ev.Transparency,
		Visibility:     ev.Visibility,
		ColorID:        ev.ColorID,
		HTMLLink:       ev.HTMLLink,
		HangoutLink:    ev.HangoutLink,
		CreatedByGV:    createdByGV,
	}
	if ev.RecurringEventID != "" {
		id := ev.RecurringEventID
		p.RecurringEventID = &id
		p.OriginalStartsAt = originalStart
	}
	if ev.Updated != "" {
		if t, err := time.Parse(time.RFC3339, ev.Updated); err == nil {
			p.GoogleUpdatedAt = &t
		}
	}
	return p, true
}

func personEmail(p *google.Person) string {
	if p == nil {
		return ""
	}
	return p.Email
}

// parseEventTime reads Google's start/end shape. An all-day value has no zone of its own, so
// it is pinned to the calendar's: the same date means a different instant in Madrid and in
// Mexico City, and range queries need one answer.
func parseEventTime(dt *google.EventDateTime, fallbackTZ string) (t time.Time, tz string, allDay bool, ok bool) {
	if dt == nil {
		return time.Time{}, "", false, false
	}
	if dt.Date != "" {
		tz = firstNonEmpty(dt.TimeZone, fallbackTZ)
		loc := resolveLocation(tz)
		parsed, err := time.ParseInLocation(dateLayout, dt.Date, loc)
		if err != nil {
			return time.Time{}, "", false, false
		}
		return parsed, tz, true, true
	}
	if dt.DateTime == "" {
		return time.Time{}, "", false, false
	}
	parsed, err := time.Parse(time.RFC3339, dt.DateTime)
	if err != nil {
		return time.Time{}, "", false, false
	}
	return parsed, firstNonEmpty(dt.TimeZone, fallbackTZ), false, true
}

// ResyncAccount throws away the account's sync cursors and its local events, then rebuilds
// from scratch. The escape hatch for when the mirror is visibly wrong.
func (s *Service) ResyncAccount(ctx context.Context, id int32) (SyncResult, error) {
	if err := s.requireConfigured(); err != nil {
		return SyncResult{}, err
	}
	acc, err := s.repo.GetAccount(ctx, id)
	if err != nil {
		return SyncResult{}, err
	}
	cals, err := s.repo.ListCalendarsByAccount(ctx, acc.ID)
	if err != nil {
		return SyncResult{}, err
	}
	for _, c := range cals {
		if _, err := s.repo.DeleteEventsForCalendar(ctx, c.ID); err != nil {
			return SyncResult{}, err
		}
	}
	if err := s.repo.ClearAccountSyncTokens(ctx, acc.ID); err != nil {
		return SyncResult{}, err
	}

	result := SyncResult{}
	for _, c := range cals {
		if !c.SyncEnabled || c.DeletedAt != nil {
			continue
		}
		res, err := s.SyncCalendar(ctx, c.ID, "manual")
		result.Calendars += res.Calendars
		result.Upserted += res.Upserted
		result.Deleted += res.Deleted
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("calendar %d: %v", c.ID, err))
		}
	}
	return result, nil
}

// SyncStatus is the operational view: it exists so a dead push channel or a stalled sync
// token is visible before someone notices their calendar stopped changing.
func (s *Service) SyncStatus(ctx context.Context) (SyncStatus, error) {
	accounts, err := s.ListAccounts(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	views, err := s.repo.ListCalendars(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	cals := make([]CalendarSync, 0, len(views))
	for _, v := range views {
		count, err := s.repo.CountEvents(ctx, v.ID)
		if err != nil {
			return SyncStatus{}, err
		}
		dto := toCalendarDTO(v)
		cals = append(cals, CalendarSync{
			CalendarID:   v.ID,
			AccountEmail: v.AccountEmail,
			Summary:      v.Summary,
			SyncEnabled:  v.SyncEnabled,
			Events:       count,
			Sync:         dto.Sync,
		})
	}
	runs, err := s.repo.ListRecentSyncRuns(ctx, 25)
	if err != nil {
		return SyncStatus{}, err
	}
	summaries := make([]SyncRunSummary, 0, len(runs))
	for _, r := range runs {
		summaries = append(summaries, SyncRunSummary{
			ID: r.ID, CalendarID: r.CalendarID, Trigger: r.Trigger, Kind: r.Kind,
			StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Pages: r.Pages,
			Upserted: r.Upserted, Deleted: r.Deleted, Error: r.Error,
		})
	}
	return SyncStatus{
		Configured:     s.Configured(),
		WebhooksActive: s.cfg.WebhookEnabled && s.cfg.WebhookURL != "",
		PollInterval:   s.cfg.SyncInterval.String(),
		Accounts:       accounts,
		Calendars:      cals,
		RecentRuns:     summaries,
	}, nil
}

func (s *Service) lockCalendar(id int32) bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.syncing[id] {
		return false
	}
	s.syncing[id] = true
	return true
}

func (s *Service) unlockCalendar(id int32) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	delete(s.syncing, id)
}
