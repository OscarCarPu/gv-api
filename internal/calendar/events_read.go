package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dateLayout is the wire format for an all-day event's dates, matching Google's.
const dateLayout = "2006-01-02"

// maxRangeSpan caps a single query. A calendar UI asks for a day, a week or a month; anything
// past a couple of years is a client bug or a scrape, and expanding infinite series over it
// is unbounded work.
const maxRangeSpan = 2 * 366 * 24 * time.Hour

/*
ListEvents returns everything happening in [From, To), with recurring series already expanded
into the occurrences that fall inside it.

The expansion is done here, not in the database and not in the client: the rules for it
(zones, DST, overrides that moved in or out of the window) are the same for every consumer,
and gv-web and gv-android must not each get their own half-right version.
*/
func (s *Service) ListEvents(ctx context.Context, q EventsQuery) ([]Event, error) {
	if q.To.Before(q.From) || q.To.Equal(q.From) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalidRange)
	}
	if q.To.Sub(q.From) > maxRangeSpan {
		return nil, fmt.Errorf("%w: range is longer than two years", ErrInvalidRange)
	}

	views, err := s.repo.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	// Events carry their calendar's colour, so the same assignment the calendar list uses has to
	// run here too, or the two would disagree.
	assignColors(views)
	byID := make(map[int32]CalendarView, len(views))
	ids := make([]int32, 0, len(views))
	for _, v := range views {
		if !calendarSelected(v, q) {
			continue
		}
		byID[v.ID] = v
		ids = append(ids, v.ID)
	}
	if len(ids) == 0 {
		return []Event{}, nil
	}

	singles, err := s.repo.ListEventsInRange(ctx, ids, q.From, q.To)
	if err != nil {
		return nil, err
	}
	masters, err := s.repo.ListRecurringMasters(ctx, ids, q.To)
	if err != nil {
		return nil, err
	}
	masterIDs := make([]int32, 0, len(masters))
	for _, m := range masters {
		masterIDs = append(masterIDs, m.ID)
	}
	overrides, err := s.repo.ListEventExceptions(ctx, masterIDs)
	if err != nil {
		return nil, err
	}
	byMaster := make(map[int32][]EventRecord, len(masterIDs))
	for _, o := range overrides {
		if o.MasterID == nil {
			continue
		}
		byMaster[*o.MasterID] = append(byMaster[*o.MasterID], o)
	}
	orphans, err := s.repo.ListOrphanOverridesInRange(ctx, ids, q.From, q.To)
	if err != nil {
		return nil, err
	}

	out := make([]Event, 0, len(singles)+len(masters)+len(orphans))
	for _, rec := range singles {
		out = append(out, s.toEventDTO(rec, byID[rec.CalendarID], nil, rec))
	}
	for _, rec := range orphans {
		out = append(out, s.toEventDTO(rec, byID[rec.CalendarID], nil, rec))
	}
	for _, master := range masters {
		cal := byID[master.CalendarID]
		occurrences, err := expandSeries(master, byMaster[master.ID], cal.TimeZone, q.From, q.To)
		if err != nil {
			// One unparseable rule must not take the whole month's view down with it. The
			// series is skipped, loudly.
			slog.ErrorContext(ctx, "calendar: expanding series",
				"event", master.ID, "recurrence", master.Recurrence, "error", err)
			continue
		}
		for i := range occurrences {
			occ := occurrences[i]
			source := master
			if occ.Exception != nil {
				source = *occ.Exception
			}
			out = append(out, s.toEventDTO(master, cal, &occ, source))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartsAt.Equal(out[j].StartsAt) {
			return out[i].InstanceID < out[j].InstanceID
		}
		return out[i].StartsAt.Before(out[j].StartsAt)
	})
	return out, nil
}

func calendarSelected(v CalendarView, q EventsQuery) bool {
	if q.VisibleOnly && !v.Visible {
		return false
	}
	if len(q.CalendarIDs) > 0 && !containsInt32(q.CalendarIDs, v.ID) {
		return false
	}
	if len(q.AccountIDs) > 0 && !containsInt32(q.AccountIDs, v.AccountID) {
		return false
	}
	return true
}

func containsInt32(haystack []int32, needle int32) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

/*
toEventDTO builds one row of the API's answer.

owner is the row a client edits through (the series master for an occurrence, the event
itself otherwise) and source is where the displayed fields come from (an override when there
is one). Keeping them apart is what lets "edit this occurrence" work whether or not an
override exists yet: the client always addresses the series and the original slot.
*/
func (s *Service) toEventDTO(owner EventRecord, cal CalendarView, occ *Occurrence, source EventRecord) Event {
	e := Event{
		InstanceID:     strconv.FormatInt(int64(owner.ID), 10),
		EventID:        owner.ID,
		CalendarID:     cal.ID,
		AccountID:      cal.AccountID,
		AccountEmail:   cal.AccountEmail,
		CalendarName:   cal.Summary,
		Color:          cal.DisplayColor(),
		GoogleEventID:  owner.GoogleEventID,
		Summary:        source.Summary,
		Description:    source.Description,
		Location:       source.Location,
		Status:         source.Status,
		EventType:      source.EventType,
		AllDay:         source.AllDay,
		StartsAt:       source.StartsAt,
		EndsAt:         source.EndsAt,
		TimeZone:       firstNonEmpty(source.StartTZ, cal.TimeZone),
		Recurring:      owner.IsMaster(),
		Recurrence:     owner.Recurrence,
		Editable:       cal.Writable() && cal.AccountStatus == "connected" && isEditableType(source.EventType),
		OrganizerEmail: source.OrganizerEmail,
		Transparency:   source.Transparency,
		Visibility:     source.Visibility,
		HTMLLink:       source.HTMLLink,
		HangoutLink:    source.HangoutLink,
		CreatedByGV:    source.CreatedByGV,
		UpdatedAt:      source.GoogleUpdatedAt,
	}
	if occ != nil {
		e.StartsAt = occ.Start
		e.EndsAt = occ.End
		original := occ.OriginalStart
		e.OriginalStartsAt = &original
		e.IsException = occ.Exception != nil
		e.InstanceID = instanceRef(owner.ID, occ.OriginalStart)
	} else if source.IsException() {
		e.IsException = true
		e.OriginalStartsAt = source.OriginalStartsAt
	}
	if e.AllDay {
		// Computed after any occurrence override, so a moved all-day instance reports the day it
		// actually lands on. The zone is the event's own, which is what the instants were built
		// from, so this round-trips to the dates Google sent.
		loc := resolveLocation(source.StartTZ, cal.TimeZone)
		e.StartDate = e.StartsAt.In(loc).Format(dateLayout)
		e.EndDate = e.EndsAt.In(loc).Format(dateLayout)
	}
	e.Attendees = decodeAttendees(source.Attendees)
	e.Reminders = decodeReminders(source.Reminders)
	return e
}

// isEditableType keeps the derived event kinds read-only. Google generates them from other
// data (contacts, Gmail, working location) and rejects edits; refusing here means the client
// gets a clear answer instead of a 403 from three layers away.
func isEditableType(eventType string) bool {
	switch eventType {
	case "", "default", "outOfOffice", "focusTime":
		return true
	default:
		return false
	}
}

func decodeAttendees(raw []byte) []Attendee {
	if len(raw) == 0 {
		return nil
	}
	var out []Attendee
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeReminders(raw []byte) *Reminders {
	if len(raw) == 0 {
		return nil
	}
	var out Reminders
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}

// instanceRef names one occurrence of a series: the event row plus the slot the rule
// produced. The original start is used rather than the current one because an override can
// move an occurrence, and the client must still be able to say which one it means.
func instanceRef(eventID int32, originalStart time.Time) string {
	return fmt.Sprintf("%d@%s", eventID, originalStart.UTC().Format(time.RFC3339))
}

// parseEventRef reads what instanceRef wrote, and a plain id on its own.
func parseEventRef(ref string) (int32, *time.Time, error) {
	idPart, startPart, hasStart := strings.Cut(ref, "@")
	id, err := strconv.ParseInt(idPart, 10, 32)
	if err != nil || id <= 0 {
		return 0, nil, fmt.Errorf("invalid event id")
	}
	if !hasStart {
		return int32(id), nil, nil
	}
	t, err := time.Parse(time.RFC3339, startPart)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid instance start %q: want RFC3339", startPart)
	}
	utc := t.UTC()
	return int32(id), &utc, nil
}

// GetEvent resolves a single event or occurrence.
func (s *Service) GetEvent(ctx context.Context, ref string) (Event, error) {
	id, originalStart, err := parseEventRef(ref)
	if err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	rec, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	cal, err := s.calendarView(ctx, rec.CalendarID)
	if err != nil {
		return Event{}, err
	}
	if originalStart == nil {
		return s.toEventDTO(rec, cal, nil, rec), nil
	}

	overrides, err := s.repo.ListEventExceptions(ctx, []int32{rec.ID})
	if err != nil {
		return Event{}, err
	}
	// An override is looked up by its slot rather than expanded into a window: an occurrence
	// that was moved is no longer anywhere near the slot it belongs to, and asking "give me
	// that occurrence" must still answer with it.
	for i := range overrides {
		ex := overrides[i]
		if ex.OriginalStartsAt == nil || !ex.OriginalStartsAt.Equal(*originalStart) {
			continue
		}
		if ex.Status == "cancelled" {
			return Event{}, ErrNotFound
		}
		occ := Occurrence{OriginalStart: *originalStart, Start: ex.StartsAt, End: ex.EndsAt, Exception: &ex}
		return s.toEventDTO(rec, cal, &occ, ex), nil
	}

	// No override: check the rule really produces that slot. A window of a second either side
	// is enough to pick out exactly one occurrence.
	occurrences, err := expandSeries(rec, nil, cal.TimeZone,
		originalStart.Add(-time.Second), originalStart.Add(time.Second))
	if err != nil {
		return Event{}, err
	}
	for i := range occurrences {
		if occurrences[i].OriginalStart.Equal(*originalStart) {
			return s.toEventDTO(rec, cal, &occurrences[i], rec), nil
		}
	}
	return Event{}, ErrNotFound
}

func (s *Service) calendarView(ctx context.Context, calendarID int32) (CalendarView, error) {
	views, err := s.repo.ListCalendars(ctx)
	if err != nil {
		return CalendarView{}, err
	}
	assignColors(views)
	for _, v := range views {
		if v.ID == calendarID {
			return v, nil
		}
	}
	return CalendarView{}, ErrNotFound
}
