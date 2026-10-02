package calendar

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gv-api/internal/calendar/google"
)

// Events created from here are tagged with this.
const (
	gvMarkerKey   = "gv"
	gvMarkerValue = "1"
)

const (
	ScopeAll       = "all"
	ScopeInstance  = "instance"
	ScopeFollowing = "following"
)

/*
CreateEvent creates the event in Google and then mirrors it.

Order matters and it is not an implementation detail: writing to Google first means a row in
calendar_events always corresponds to something that exists over there. The alternative —
store locally, push later — needs a queue, a retry policy and a story for what the user sees
in between, and it can leave the two copies disagreeing for as long as the queue is stuck.
If Google says no, the request fails and nothing is stored.
*/
func (s *Service) CreateEvent(ctx context.Context, req CreateEventRequest) (Event, error) {
	if err := s.requireConfigured(); err != nil {
		return Event{}, err
	}
	cal, token, err := s.writableCalendar(ctx, req.CalendarID)
	if err != nil {
		return Event{}, err
	}

	tz := firstNonEmpty(req.TimeZone, cal.TimeZone, s.loc.String())
	start, end, err := parseWriteRange(req.StartsAt, req.EndsAt, req.AllDay, tz)
	if err != nil {
		return Event{}, err
	}

	body := map[string]any{
		"summary": strings.TrimSpace(req.Summary),
		"start":   googleDateTime(start, req.AllDay, tz),
		"end":     googleDateTime(end, req.AllDay, tz),
		"extendedProperties": map[string]any{
			"private": map[string]string{gvMarkerKey: gvMarkerValue},
		},
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Location != "" {
		body["location"] = req.Location
	}
	if len(req.Recurrence) > 0 {
		body["recurrence"] = req.Recurrence
	}
	if len(req.Attendees) > 0 {
		body["attendees"] = attendeeBody(req.Attendees)
	}
	if req.Reminders != nil {
		body["reminders"] = remindersBody(*req.Reminders)
	}
	for key, value := range map[string]string{
		"transparency": req.Transparency,
		"visibility":   req.Visibility,
		"colorId":      req.ColorID,
	} {
		if value != "" {
			body[key] = value
		}
	}

	created, err := s.gc.InsertEvent(ctx, token, cal.GoogleCalendarID, body, sendUpdates(req.SendUpdates))
	if err != nil {
		return Event{}, s.upstreamError(ctx, cal, err)
	}

	rec, err := s.mirror(ctx, cal, *created)
	if err != nil {
		return Event{}, err
	}
	s.afterWrite(ctx, cal)
	return s.GetEvent(ctx, strconv.FormatInt(int64(rec.ID), 10))
}

/*
UpdateEvent patches an event or one occurrence of a series.

The scope is what makes this more than a passthrough:

  - all: the series master (or the plain event) is patched.
  - instance: only that occurrence. If Google has no override for it yet, one is materialised
    — asking Google for the instance rather than guessing its id, because the id format is
    documented loosely and getting it wrong writes to the wrong event.
  - following: the original series is ended just before the occurrence and a new series is
    created from it. That is the only way Google models a "this and following" change, and it
    is why the split has to compute the leftover COUNT when the rule uses one.
*/
func (s *Service) UpdateEvent(ctx context.Context, ref string, req UpdateEventRequest) (Event, error) {
	if err := s.requireConfigured(); err != nil {
		return Event{}, err
	}
	rec, cal, token, originalStart, scope, err := s.resolveWriteTarget(ctx, ref, req.Scope)
	if err != nil {
		return Event{}, err
	}

	tz := firstNonEmpty(derefOr(req.TimeZone, ""), rec.StartTZ, cal.TimeZone, s.loc.String())
	allDay := derefOr(req.AllDay, rec.AllDay)

	patch, err := s.buildPatch(req, rec, allDay, tz, originalStart, scope)
	if err != nil {
		return Event{}, err
	}

	switch scope {
	case ScopeAll:
		updated, err := s.gc.PatchEvent(ctx, token, cal.GoogleCalendarID, rec.GoogleEventID, rec.Etag,
			patch, sendUpdates(req.SendUpdates))
		if err != nil {
			return Event{}, s.upstreamError(ctx, cal, err)
		}
		if _, err := s.mirror(ctx, cal, *updated); err != nil {
			return Event{}, err
		}

	case ScopeInstance:
		target, etag, err := s.resolveInstance(ctx, token, cal, rec, *originalStart)
		if err != nil {
			return Event{}, err
		}
		updated, err := s.gc.PatchEvent(ctx, token, cal.GoogleCalendarID, target, etag,
			patch, sendUpdates(req.SendUpdates))
		if err != nil {
			return Event{}, s.upstreamError(ctx, cal, err)
		}
		if _, err := s.mirror(ctx, cal, *updated); err != nil {
			return Event{}, err
		}

	case ScopeFollowing:
		if err := s.splitSeries(ctx, token, cal, rec, *originalStart, patch, req); err != nil {
			return Event{}, err
		}
	}

	s.afterWrite(ctx, cal)

	if req.StartsAt != nil || req.EndsAt != nil {
		s.syncPlanAfterUpdate(ctx, ref, scope)
	}

	// The ref stays valid for instance and whole-series edits; after a split, re-resolve from the
	// occurrence.
	if scope == ScopeFollowing {
		return s.eventAt(ctx, cal.ID, *originalStart)
	}
	return s.GetEvent(ctx, ref)
}

// syncPlanAfterUpdate keeps a linked plan_block in step with the event's new time. After a
// scope=following split, ref may point into a new series under a different id.
func (s *Service) syncPlanAfterUpdate(ctx context.Context, ref, scope string) {
	ev, err := s.GetEvent(ctx, ref)
	if err != nil {
		if scope == ScopeFollowing && errors.Is(err, ErrNotFound) {
			if derr := s.planSync.DetachEvent(ctx, ref); derr != nil {
				slog.ErrorContext(ctx, "calendar: detaching plan block after series split", "ref", ref, "error", derr)
			}
			return
		}
		slog.ErrorContext(ctx, "calendar: resolving event to sync linked plan block", "ref", ref, "error", err)
		return
	}
	if err := s.planSync.SyncEventTime(ctx, ref, ev.StartsAt, ev.EndsAt); err != nil {
		slog.ErrorContext(ctx, "calendar: syncing linked plan block", "ref", ref, "error", err)
	}
}

/*
DeleteEvent removes an event, one occurrence, or an occurrence and everything after it.

Deleting a single occurrence does not remove a row from Google: it leaves a cancelled
override behind, which is the hole in the series. That row is mirrored and kept, because
dropping it would make the occurrence reappear on the next expansion.
*/
func (s *Service) DeleteEvent(ctx context.Context, ref, scopeParam, sendUpdatesParam string) error {
	if err := s.requireConfigured(); err != nil {
		return err
	}
	rec, cal, token, originalStart, scope, err := s.resolveWriteTarget(ctx, ref, scopeParam)
	if err != nil {
		return err
	}

	switch scope {
	case ScopeAll:
		if err := s.gc.DeleteEvent(ctx, token, cal.GoogleCalendarID, rec.GoogleEventID, rec.Etag,
			sendUpdates(sendUpdatesParam)); err != nil {
			return s.upstreamError(ctx, cal, err)
		}
		if err := s.repo.DeleteEvent(ctx, rec.ID); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}

	case ScopeInstance:
		target, etag, err := s.resolveInstance(ctx, token, cal, rec, *originalStart)
		if err != nil {
			return err
		}
		if err := s.gc.DeleteEvent(ctx, token, cal.GoogleCalendarID, target, etag,
			sendUpdates(sendUpdatesParam)); err != nil {
			return s.upstreamError(ctx, cal, err)
		}

	case ScopeFollowing:
		lines, err := endSeriesBefore(rec.Recurrence, *originalStart, rec, s.loc)
		if err != nil {
			return err
		}
		updated, err := s.gc.PatchEvent(ctx, token, cal.GoogleCalendarID, rec.GoogleEventID, rec.Etag,
			map[string]any{"recurrence": lines}, sendUpdates(sendUpdatesParam))
		if err != nil {
			return s.upstreamError(ctx, cal, err)
		}
		if _, err := s.mirror(ctx, cal, *updated); err != nil {
			return err
		}
	}

	s.afterWrite(ctx, cal)
	if err := s.planSync.DetachEvent(ctx, ref); err != nil {
		slog.ErrorContext(ctx, "calendar: detaching plan block after event delete", "ref", ref, "error", err)
	}
	return nil
}

/*
MoveEvent changes which calendar an event lives on.

Google can only move an event between calendars of the same account. Across accounts there is
no move at all, so the event is recreated on the destination and removed from the source —
which changes its id, drops any attendee responses, and is reported back as Recreated so a
client does not keep using the old reference.
*/
func (s *Service) MoveEvent(ctx context.Context, ref string, req MoveEventRequest) (MoveResult, error) {
	if err := s.requireConfigured(); err != nil {
		return MoveResult{}, err
	}
	id, originalStart, err := parseEventRef(ref)
	if err != nil {
		return MoveResult{}, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if originalStart != nil {
		return MoveResult{}, fmt.Errorf("%w: a single occurrence cannot be moved to another calendar", ErrInvalidScope)
	}
	rec, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return MoveResult{}, err
	}
	source, sourceToken, err := s.writableCalendar(ctx, rec.CalendarID)
	if err != nil {
		return MoveResult{}, err
	}
	dest, destToken, err := s.writableCalendar(ctx, req.CalendarID)
	if err != nil {
		return MoveResult{}, err
	}
	if dest.ID == source.ID {
		ev, err := s.GetEvent(ctx, ref)
		return MoveResult{Event: ev}, err
	}

	if dest.AccountID == source.AccountID {
		moved, err := s.gc.MoveEvent(ctx, sourceToken, source.GoogleCalendarID, rec.GoogleEventID,
			dest.GoogleCalendarID, sendUpdates(req.SendUpdates))
		if err != nil {
			return MoveResult{}, s.upstreamError(ctx, source, err)
		}
		if err := s.repo.DeleteEvent(ctx, rec.ID); err != nil && !errors.Is(err, ErrNotFound) {
			return MoveResult{}, err
		}
		newRec, err := s.mirror(ctx, dest, *moved)
		if err != nil {
			return MoveResult{}, err
		}
		s.afterWrite(ctx, source)
		s.afterWrite(ctx, dest)
		// The row was recreated, so its id and event_ref changed.
		if err := s.planSync.DetachEvent(ctx, ref); err != nil {
			slog.ErrorContext(ctx, "calendar: detaching plan block after move", "ref", ref, "error", err)
		}
		ev, err := s.GetEvent(ctx, strconv.FormatInt(int64(newRec.ID), 10))
		return MoveResult{Event: ev}, err
	}

	body := bodyFromRecord(rec)
	created, err := s.gc.InsertEvent(ctx, destToken, dest.GoogleCalendarID, body, sendUpdates(req.SendUpdates))
	if err != nil {
		return MoveResult{}, s.upstreamError(ctx, dest, err)
	}
	if err := s.gc.DeleteEvent(ctx, sourceToken, source.GoogleCalendarID, rec.GoogleEventID, rec.Etag,
		sendUpdates(req.SendUpdates)); err != nil {
		// The copy exists; never delete it to roll back, it may be the only remaining version.
		slog.ErrorContext(ctx, "calendar: cross-account move left the original in place",
			"event", rec.ID, "error", err)
		return MoveResult{}, s.upstreamError(ctx, source, err)
	}
	if err := s.repo.DeleteEvent(ctx, rec.ID); err != nil && !errors.Is(err, ErrNotFound) {
		return MoveResult{}, err
	}
	newRec, err := s.mirror(ctx, dest, *created)
	if err != nil {
		return MoveResult{}, err
	}
	s.afterWrite(ctx, source)
	s.afterWrite(ctx, dest)
	if err := s.planSync.DetachEvent(ctx, ref); err != nil {
		slog.ErrorContext(ctx, "calendar: detaching plan block after cross-account move", "ref", ref, "error", err)
	}
	ev, err := s.GetEvent(ctx, strconv.FormatInt(int64(newRec.ID), 10))
	return MoveResult{Event: ev, Recreated: true}, err
}

// writableCalendar loads a calendar and access token, refusing early what Google would refuse.
func (s *Service) writableCalendar(ctx context.Context, calendarID int32) (CalendarRecord, string, error) {
	cal, err := s.repo.GetCalendar(ctx, calendarID)
	if err != nil {
		return CalendarRecord{}, "", err
	}
	if cal.DeletedAt != nil {
		return CalendarRecord{}, "", fmt.Errorf("%w: calendar no longer exists in google", ErrReadOnly)
	}
	if !cal.Writable() {
		return CalendarRecord{}, "", fmt.Errorf("%w: access role is %s", ErrReadOnly, cal.AccessRole)
	}
	acc, err := s.repo.GetAccount(ctx, cal.AccountID)
	if err != nil {
		return CalendarRecord{}, "", err
	}
	token, err := s.accessTokenFor(ctx, acc)
	if err != nil {
		return CalendarRecord{}, "", err
	}
	return cal, token, nil
}

// resolveWriteTarget resolves a reference and scope into the row, occurrence and effective scope.
func (s *Service) resolveWriteTarget(ctx context.Context, ref, requested string) (
	EventRecord, CalendarRecord, string, *time.Time, string, error,
) {
	id, originalStart, err := parseEventRef(ref)
	if err != nil {
		return EventRecord{}, CalendarRecord{}, "", nil, "", fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	rec, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return EventRecord{}, CalendarRecord{}, "", nil, "", err
	}
	if !isEditableType(rec.EventType) {
		return EventRecord{}, CalendarRecord{}, "", nil, "",
			fmt.Errorf("%w: %s events are generated by google and cannot be edited", ErrReadOnly, rec.EventType)
	}
	cal, token, err := s.writableCalendar(ctx, rec.CalendarID)
	if err != nil {
		return EventRecord{}, CalendarRecord{}, "", nil, "", err
	}

	scope, err := resolveScope(requested, originalStart != nil, rec.IsMaster())
	if err != nil {
		return EventRecord{}, CalendarRecord{}, "", nil, "", err
	}
	return rec, cal, token, originalStart, scope, nil
}

/*
resolveScope decides what a write touches.

The default is the least surprising one available: an occurrence reference edits that
occurrence, a plain reference edits the event. Asking for an occurrence-level scope without
naming an occurrence is rejected rather than guessed at, because guessing wrong there edits
somebody's whole series.
*/
func resolveScope(requested string, hasInstance, isSeries bool) (string, error) {
	switch requested {
	case "":
		if hasInstance && isSeries {
			return ScopeInstance, nil
		}
		return ScopeAll, nil
	case ScopeAll:
		return ScopeAll, nil
	case ScopeInstance, ScopeFollowing:
		if !hasInstance {
			return "", fmt.Errorf("%w: scope %q needs an instance reference like 12@2026-08-20T07:00:00Z",
				ErrInvalidScope, requested)
		}
		if !isSeries {
			return "", fmt.Errorf("%w: %q only applies to recurring events", ErrInvalidScope, requested)
		}
		return requested, nil
	default:
		return "", fmt.Errorf("%w: %q, want all, instance or following", ErrInvalidScope, requested)
	}
}

func (s *Service) buildPatch(req UpdateEventRequest, rec EventRecord, allDay bool, tz string,
	originalStart *time.Time, scope string,
) (map[string]any, error) {
	patch := map[string]any{}
	if req.Summary != nil {
		patch["summary"] = strings.TrimSpace(*req.Summary)
	}
	if req.Description != nil {
		patch["description"] = *req.Description
	}
	if req.Location != nil {
		patch["location"] = *req.Location
	}
	if req.Transparency != nil {
		patch["transparency"] = *req.Transparency
	}
	if req.Visibility != nil {
		patch["visibility"] = *req.Visibility
	}
	if req.ColorID != nil {
		patch["colorId"] = *req.ColorID
	}
	if req.Attendees != nil {
		patch["attendees"] = attendeeBody(*req.Attendees)
	}
	if req.Reminders != nil {
		patch["reminders"] = remindersBody(*req.Reminders)
	}
	if req.Recurrence != nil {
		if scope != ScopeAll {
			return nil, fmt.Errorf("%w: the recurrence rule can only be changed with scope=all", ErrInvalidScope)
		}
		patch["recurrence"] = *req.Recurrence
	}

	// Times are only sent when asked for, based on the edited occurrence rather than the series
	// start; moving one edge keeps the other.
	if req.StartsAt != nil || req.EndsAt != nil || req.AllDay != nil {
		baseStart, baseEnd := rec.StartsAt, rec.EndsAt
		if originalStart != nil {
			duration := rec.EndsAt.Sub(rec.StartsAt)
			baseStart = *originalStart
			baseEnd = originalStart.Add(duration)
		}
		start, end, err := resolveWriteTimes(req, baseStart, baseEnd, allDay, tz)
		if err != nil {
			return nil, err
		}
		patch["start"] = googleDateTime(start, allDay, tz)
		patch["end"] = googleDateTime(end, allDay, tz)
	}
	if len(patch) == 0 {
		return nil, fmt.Errorf("%w: nothing to update", ErrInvalidRange)
	}
	return patch, nil
}

func resolveWriteTimes(req UpdateEventRequest, baseStart, baseEnd time.Time, allDay bool, tz string) (time.Time, time.Time, error) {
	loc := resolveLocation(tz)
	start, end := baseStart, baseEnd
	if req.StartsAt != nil {
		parsed, err := parseWriteTime(*req.StartsAt, allDay, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		// Moving the start alone keeps the length.
		if req.EndsAt == nil {
			end = end.Add(parsed.Sub(start))
		}
		start = parsed
	}
	if req.EndsAt != nil {
		parsed, err := parseWriteTime(*req.EndsAt, allDay, loc)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = parsed
	}
	// All-day ends are exclusive: an end on the start date is a zero-day event no query returns.
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: end must be after start", ErrInvalidRange)
	}
	return start, end, nil
}

/*
resolveInstance finds the Google event id to write for one occurrence of a series.

If an override already exists locally its id and etag are used, which costs no round-trip and
keeps the concurrency check honest. Otherwise Google is asked for the instance: it returns the
id (and etag) of the occurrence, materialising the override on the first write to it.
*/
func (s *Service) resolveInstance(ctx context.Context, token string, cal CalendarRecord,
	master EventRecord, originalStart time.Time,
) (string, string, error) {
	overrides, err := s.repo.ListEventExceptions(ctx, []int32{master.ID})
	if err != nil {
		return "", "", err
	}
	for _, o := range overrides {
		if o.OriginalStartsAt != nil && o.OriginalStartsAt.Equal(originalStart) {
			return o.GoogleEventID, o.Etag, nil
		}
	}

	instances, err := s.gc.ListInstances(ctx, token, cal.GoogleCalendarID, master.GoogleEventID,
		originalStart.UTC().Format(time.RFC3339))
	if err != nil {
		return "", "", s.upstreamError(ctx, cal, err)
	}
	for _, inst := range instances {
		if inst.ID != "" {
			return inst.ID, inst.Etag, nil
		}
	}
	return "", "", fmt.Errorf("%w: google has no occurrence of this series at %s",
		ErrNotFound, originalStart.Format(time.RFC3339))
}

/*
splitSeries implements "this and following": the original series is ended just before the
occurrence, and a new series carrying the change starts at it.

The leftover COUNT has to be worked out when the rule counts occurrences rather than ending on
a date, otherwise the new series would run for the full original count and the series would
get longer every time someone edits it.
*/
func (s *Service) splitSeries(ctx context.Context, token string, cal CalendarRecord, master EventRecord,
	originalStart time.Time, patch map[string]any, req UpdateEventRequest,
) error {
	overrides, err := s.repo.ListEventExceptions(ctx, []int32{master.ID})
	if err != nil {
		return err
	}
	before, err := expandSeries(master, overrides, cal.TimeZone, master.StartsAt.Add(-time.Second), originalStart)
	if err != nil {
		return err
	}
	remaining, err := remainingRecurrence(master.Recurrence, len(before))
	if err != nil {
		return err
	}
	truncated, err := endSeriesBefore(master.Recurrence, originalStart, master, s.loc)
	if err != nil {
		return err
	}

	// Build the tail from the master, then apply the requested change on top.
	tail := bodyFromRecord(master)
	delete(tail, "recurringEventId")
	delete(tail, "originalStartTime")
	tail["recurrence"] = remaining
	duration := master.EndsAt.Sub(master.StartsAt)
	tz := firstNonEmpty(master.StartTZ, cal.TimeZone, s.loc.String())
	tail["start"] = googleDateTime(originalStart, master.AllDay, tz)
	tail["end"] = googleDateTime(originalStart.Add(duration), master.AllDay, tz)
	for k, v := range patch {
		tail[k] = v
	}

	// End the original first: if creating the tail fails, a series that stops early is easier to fix
	// than two overlapping ones.
	updated, err := s.gc.PatchEvent(ctx, token, cal.GoogleCalendarID, master.GoogleEventID, master.Etag,
		map[string]any{"recurrence": truncated}, sendUpdates(req.SendUpdates))
	if err != nil {
		return s.upstreamError(ctx, cal, err)
	}
	if _, err := s.mirror(ctx, cal, *updated); err != nil {
		return err
	}

	created, err := s.gc.InsertEvent(ctx, token, cal.GoogleCalendarID, tail, sendUpdates(req.SendUpdates))
	if err != nil {
		return s.upstreamError(ctx, cal, err)
	}
	if _, err := s.mirror(ctx, cal, *created); err != nil {
		return err
	}
	return nil
}

// endSeriesBefore stops the series before the given occurrence. UNTIL is inclusive, so it is set
// one second earlier; COUNT is dropped.
func endSeriesBefore(lines []string, before time.Time, _ EventRecord, _ *time.Location) ([]string, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: not a recurring event", ErrInvalidScope)
	}
	until := before.UTC().Add(-time.Second).Format("20060102T150405Z")
	out := make([]string, 0, len(lines))
	found := false
	for _, line := range lines {
		if !strings.HasPrefix(strings.ToUpper(line), "RRULE") {
			out = append(out, line)
			continue
		}
		found = true
		out = append(out, rewriteRRule(line, map[string]string{"UNTIL": until}, []string{"COUNT"}))
	}
	if !found {
		return nil, fmt.Errorf("%w: event has no RRULE to split", ErrInvalidScope)
	}
	return out, nil
}

func remainingRecurrence(lines []string, consumed int) ([]string, error) {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		upper := strings.ToUpper(line)
		if !strings.HasPrefix(upper, "RRULE") {
			// The original's EXDATEs/RDATEs belong to the old series.
			continue
		}
		if idx := strings.Index(upper, "COUNT="); idx >= 0 {
			value := countValue(line[idx+len("COUNT="):])
			total, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("cannot read COUNT in %q", line)
			}
			left := total - consumed
			if left < 1 {
				left = 1
			}
			out = append(out, rewriteRRule(line, map[string]string{"COUNT": strconv.Itoa(left)}, nil))
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: event has no RRULE to split", ErrInvalidScope)
	}
	return out, nil
}

func countValue(s string) string {
	if idx := strings.IndexAny(s, ";"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// rewriteRRule sets and removes parameters of a single RRULE line.
func rewriteRRule(line string, set map[string]string, remove []string) string {
	prefix := ""
	body := line
	if idx := strings.Index(line, ":"); idx >= 0 {
		prefix, body = line[:idx+1], line[idx+1:]
	}
	parts := strings.Split(body, ";")
	kept := make([]string, 0, len(parts)+len(set))
	seen := map[string]bool{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		key := strings.ToUpper(part)
		if idx := strings.Index(part, "="); idx >= 0 {
			key = strings.ToUpper(part[:idx])
		}
		if contains(remove, key) {
			continue
		}
		if value, ok := set[key]; ok {
			kept = append(kept, key+"="+value)
			seen[key] = true
			continue
		}
		kept = append(kept, part)
	}
	for key, value := range set {
		if !seen[key] && !contains(remove, key) {
			kept = append(kept, key+"="+value)
		}
	}
	return prefix + strings.Join(kept, ";")
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// mirror stores Google's response, which is authoritative (etag, normalised fields).
func (s *Service) mirror(ctx context.Context, cal CalendarRecord, ev google.Event) (EventRecord, error) {
	if ev.Cancelled() && ev.RecurringEventID == "" {
		if err := s.repo.DeleteEventByGoogleID(ctx, cal.ID, ev.ID); err != nil {
			return EventRecord{}, err
		}
		return EventRecord{}, nil
	}
	var existing *EventRecord
	if found, err := s.repo.GetEventByGoogleID(ctx, cal.ID, ev.ID); err == nil {
		existing = &found
	} else if !errors.Is(err, ErrNotFound) {
		return EventRecord{}, err
	}
	params, ok := s.eventParams(cal, ev, existing)
	if !ok {
		return EventRecord{}, fmt.Errorf("google returned an event with no usable start")
	}
	rec, err := s.repo.UpsertEvent(ctx, params)
	if err != nil {
		return EventRecord{}, err
	}
	if err := s.repo.LinkEventMasters(ctx, cal.ID); err != nil {
		return EventRecord{}, err
	}
	return rec, nil
}

// afterWrite notifies listeners and queues a sync to reconcile the write's side effects.
func (s *Service) afterWrite(ctx context.Context, cal CalendarRecord) {
	s.stream.Publish(StreamMessage{Type: "calendar.changed", CalendarID: cal.ID})
	s.notifyChange(cal.ID)
}

// eventAt finds the event covering an instant on a calendar, used after a split.
func (s *Service) eventAt(ctx context.Context, calendarID int32, at time.Time) (Event, error) {
	events, err := s.ListEvents(ctx, EventsQuery{
		From:        at.Add(-time.Second),
		To:          at.Add(time.Second),
		CalendarIDs: []int32{calendarID},
	})
	if err != nil {
		return Event{}, err
	}
	for _, e := range events {
		if e.StartsAt.Equal(at) {
			return e, nil
		}
	}
	if len(events) > 0 {
		return events[0], nil
	}
	return Event{}, ErrNotFound
}

/*
upstreamError translates Google's refusals into this domain's errors.

412 becomes a conflict rather than a failure: the event changed elsewhere since the etag we
held, so the honest answer is "refetch and try again", not "something broke". A calendar that
answers 401 after a successful token refresh means the grant died mid-request, so the account
is parked.
*/
func (s *Service) upstreamError(ctx context.Context, cal CalendarRecord, err error) error {
	switch {
	case google.IsPreconditionFailed(err):
		s.notifyChange(cal.ID)
		return fmt.Errorf("%w: %v", ErrConflict, err)
	case google.IsForbidden(err):
		return fmt.Errorf("%w: %v", ErrReadOnly, err)
	case google.IsNotFound(err):
		s.notifyChange(cal.ID)
		return fmt.Errorf("%w: google does not have this event any more", ErrNotFound)
	case google.IsUnauthorized(err), google.IsInvalidGrant(err):
		msg := "google rejected the credentials mid-request; reconnect this account"
		if statusErr := s.repo.UpdateAccountStatus(ctx, cal.AccountID, "needs_reauth", &msg); statusErr != nil {
			slog.ErrorContext(ctx, "calendar: parking account", "account", cal.AccountID, "error", statusErr)
		}
		return ErrNeedsReauth
	default:
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
}

// notifyChange queues an incremental sync without blocking; a dropped one costs a poll interval.
func (s *Service) notifyChange(calendarID int32) {
	select {
	case s.changes <- calendarID:
	default:
		slog.Warn("calendar: change queue is full, falling back to the poll", "calendar", calendarID)
	}
}

func googleDateTime(t time.Time, allDay bool, tz string) map[string]any {
	if allDay {
		loc := resolveLocation(tz)
		return map[string]any{"date": t.In(loc).Format(dateLayout)}
	}
	return map[string]any{"dateTime": t.Format(time.RFC3339), "timeZone": tz}
}

// parseWriteRange validates a create request's times. A missing end defaults to one day
// (all-day) or one hour (timed).
func parseWriteRange(startRaw, endRaw string, allDay bool, tz string) (time.Time, time.Time, error) {
	loc := resolveLocation(tz)
	if strings.TrimSpace(startRaw) == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: starts_at is required", ErrInvalidRange)
	}
	start, err := parseWriteTime(startRaw, allDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if strings.TrimSpace(endRaw) == "" {
		if allDay {
			local := start.In(loc)
			return start, time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc), nil
		}
		return start, start.Add(time.Hour), nil
	}
	end, err := parseWriteTime(endRaw, allDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: end must be after start", ErrInvalidRange)
	}
	return start, end, nil
}

// parseWriteTime accepts a date for an all-day event (or keeps an instant's local date) and an
// RFC3339 instant otherwise.
func parseWriteTime(raw string, allDay bool, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if allDay {
		if t, err := time.ParseInLocation(dateLayout, raw, loc); err == nil {
			return t, nil
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			local := t.In(loc)
			return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc), nil
		}
		return time.Time{}, fmt.Errorf("%w: %q is not a date (YYYY-MM-DD)", ErrInvalidRange, raw)
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not an RFC3339 instant", ErrInvalidRange, raw)
	}
	return t, nil
}

func attendeeBody(in []AttendeeInput) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, a := range in {
		email := strings.TrimSpace(a.Email)
		if email == "" {
			continue
		}
		entry := map[string]any{"email": email}
		if a.Optional {
			entry["optional"] = true
		}
		out = append(out, entry)
	}
	return out
}

func remindersBody(in RemindersInput) map[string]any {
	body := map[string]any{"useDefault": in.UseDefault}
	if !in.UseDefault && len(in.Overrides) > 0 {
		overrides := make([]map[string]any, 0, len(in.Overrides))
		for _, o := range in.Overrides {
			overrides = append(overrides, map[string]any{"method": o.Method, "minutes": o.Minutes})
		}
		body["overrides"] = overrides
	}
	return body
}

// bodyFromRecord rebuilds a Google insert body from a stored event, for cross-account moves and
// split tails.
func bodyFromRecord(rec EventRecord) map[string]any {
	tz := firstNonEmpty(rec.StartTZ, "UTC")
	body := map[string]any{
		"summary": rec.Summary,
		"start":   googleDateTime(rec.StartsAt, rec.AllDay, tz),
		"end":     googleDateTime(rec.EndsAt, rec.AllDay, tz),
		"extendedProperties": map[string]any{
			"private": map[string]string{gvMarkerKey: gvMarkerValue},
		},
	}
	if rec.Description != "" {
		body["description"] = rec.Description
	}
	if rec.Location != "" {
		body["location"] = rec.Location
	}
	if len(rec.Recurrence) > 0 {
		body["recurrence"] = rec.Recurrence
	}
	if rec.Transparency != "" {
		body["transparency"] = rec.Transparency
	}
	if rec.Visibility != "" {
		body["visibility"] = rec.Visibility
	}
	if rec.ColorID != "" {
		body["colorId"] = rec.ColorID
	}
	if attendees := decodeAttendees(rec.Attendees); len(attendees) > 0 {
		converted := make([]map[string]any, 0, len(attendees))
		for _, a := range attendees {
			converted = append(converted, map[string]any{"email": a.Email, "optional": a.Optional})
		}
		body["attendees"] = converted
	}
	if r := decodeReminders(rec.Reminders); r != nil {
		body["reminders"] = remindersBody(RemindersInput{UseDefault: r.UseDefault, Overrides: r.Overrides})
	}
	return body
}

// sendUpdates defaults to none, so edits do not mail attendees unless asked.
func sendUpdates(requested string) string {
	switch requested {
	case "all", "externalOnly", "none":
		return requested
	default:
		return "none"
	}
}

func derefOr[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
