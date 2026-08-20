package calendar_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"gv-api/internal/calendar"
)

/*
fakeRepo is an in-memory Repository.

The sync path makes dozens of repository calls per pass, and expressing those as mock
expectations would test the call sequence rather than the behaviour. What matters here is the
outcome — which rows exist after a 410, whether a cancelled override survived — so the tests
run against a store they can inspect. The SQL itself is covered by the integration tests.
*/
type fakeRepo struct {
	mu sync.Mutex

	accounts  map[int32]calendar.AccountRecord
	calendars map[int32]calendar.CalendarRecord
	events    map[int32]calendar.EventRecord
	runs      []calendar.SyncRun

	nextAccount  int32
	nextCalendar int32
	nextEvent    int32
	nextRun      int32

	// FailUpsertEvent, when set, makes the next event upsert fail. Used to check that a
	// half-applied sync does not leave a sync token behind.
	FailUpsertEvent error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		accounts:  map[int32]calendar.AccountRecord{},
		calendars: map[int32]calendar.CalendarRecord{},
		events:    map[int32]calendar.EventRecord{},
	}
}

func (r *fakeRepo) UpsertAccount(_ context.Context, p calendar.UpsertAccountParams) (calendar.AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, acc := range r.accounts {
		if acc.Email == p.Email {
			acc.RefreshTokenSealed = p.RefreshTokenSealed
			acc.AccessTokenSealed = p.AccessTokenSealed
			acc.AccessTokenExpiresAt = p.AccessTokenExpiresAt
			acc.Scopes = p.Scopes
			acc.Status = "connected"
			acc.LastSyncError = nil
			r.accounts[id] = acc
			return acc, nil
		}
	}
	r.nextAccount++
	acc := calendar.AccountRecord{
		ID:                   r.nextAccount,
		Email:                p.Email,
		RefreshTokenSealed:   p.RefreshTokenSealed,
		AccessTokenSealed:    p.AccessTokenSealed,
		AccessTokenExpiresAt: p.AccessTokenExpiresAt,
		Scopes:               p.Scopes,
		Status:               "connected",
		CreatedAt:            time.Now(),
	}
	r.accounts[acc.ID] = acc
	return acc, nil
}

func (r *fakeRepo) GetAccount(_ context.Context, id int32) (calendar.AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[id]
	if !ok {
		return calendar.AccountRecord{}, calendar.ErrNotFound
	}
	return acc, nil
}

func (r *fakeRepo) ListAccounts(_ context.Context) ([]calendar.AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]calendar.AccountRecord, 0, len(r.accounts))
	for _, acc := range r.accounts {
		out = append(out, acc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) UpdateAccountAccessToken(_ context.Context, id int32, sealed []byte, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[id]
	if !ok {
		return calendar.ErrNotFound
	}
	acc.AccessTokenSealed = sealed
	exp := expiresAt
	acc.AccessTokenExpiresAt = &exp
	r.accounts[id] = acc
	return nil
}

func (r *fakeRepo) UpdateAccountStatus(_ context.Context, id int32, status string, syncError *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[id]
	if !ok {
		return calendar.ErrNotFound
	}
	acc.Status = status
	acc.LastSyncError = syncError
	r.accounts[id] = acc
	return nil
}

func (r *fakeRepo) UpdateAccountMeta(_ context.Context, id int32, label, color *string) (calendar.AccountRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[id]
	if !ok {
		return calendar.AccountRecord{}, calendar.ErrNotFound
	}
	if label != nil {
		acc.Label = *label
	}
	if color != nil {
		acc.Color = *color
	}
	r.accounts[id] = acc
	return acc, nil
}

func (r *fakeRepo) TouchAccountSync(_ context.Context, id int32, syncError *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	acc, ok := r.accounts[id]
	if !ok {
		return calendar.ErrNotFound
	}
	now := time.Now()
	acc.LastSyncAt = &now
	acc.LastSyncError = syncError
	r.accounts[id] = acc
	return nil
}

func (r *fakeRepo) DeleteAccount(_ context.Context, id int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[id]; !ok {
		return calendar.ErrNotFound
	}
	delete(r.accounts, id)
	for calID, cal := range r.calendars {
		if cal.AccountID != id {
			continue
		}
		delete(r.calendars, calID)
		for evID, ev := range r.events {
			if ev.CalendarID == calID {
				delete(r.events, evID)
			}
		}
	}
	return nil
}

func (r *fakeRepo) UpsertCalendar(_ context.Context, p calendar.UpsertCalendarParams) (calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, cal := range r.calendars {
		if cal.AccountID == p.AccountID && cal.GoogleCalendarID == p.GoogleCalendarID {
			cal.Summary = p.Summary
			cal.Description = p.Description
			cal.TimeZone = p.TimeZone
			cal.BackgroundColor = p.BackgroundColor
			cal.ForegroundColor = p.ForegroundColor
			cal.AccessRole = p.AccessRole
			cal.IsPrimary = p.IsPrimary
			cal.DeletedAt = nil
			r.calendars[id] = cal
			return cal, nil
		}
	}
	r.nextCalendar++
	cal := calendar.CalendarRecord{
		ID:               r.nextCalendar,
		AccountID:        p.AccountID,
		GoogleCalendarID: p.GoogleCalendarID,
		Summary:          p.Summary,
		Description:      p.Description,
		TimeZone:         p.TimeZone,
		BackgroundColor:  p.BackgroundColor,
		ForegroundColor:  p.ForegroundColor,
		AccessRole:       p.AccessRole,
		IsPrimary:        p.IsPrimary,
		SyncEnabled:      p.SyncEnabled,
		Visible:          true,
	}
	r.calendars[cal.ID] = cal
	return cal, nil
}

func (r *fakeRepo) GetCalendar(_ context.Context, id int32) (calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.CalendarRecord{}, calendar.ErrNotFound
	}
	return cal, nil
}

func (r *fakeRepo) GetCalendarByChannel(_ context.Context, channelID string) (calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cal := range r.calendars {
		if cal.WatchChannelID != nil && *cal.WatchChannelID == channelID {
			return cal, nil
		}
	}
	return calendar.CalendarRecord{}, calendar.ErrNotFound
}

func (r *fakeRepo) ListCalendars(_ context.Context) ([]calendar.CalendarView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]calendar.CalendarView, 0, len(r.calendars))
	for _, cal := range r.calendars {
		acc := r.accounts[cal.AccountID]
		out = append(out, calendar.CalendarView{
			CalendarRecord: cal,
			AccountEmail:   acc.Email,
			AccountLabel:   acc.Label,
			AccountColor:   acc.Color,
			AccountStatus:  acc.Status,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) ListCalendarsByAccount(_ context.Context, accountID int32) ([]calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []calendar.CalendarRecord{}
	for _, cal := range r.calendars {
		if cal.AccountID == accountID {
			out = append(out, cal)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) ListSyncableCalendars(ctx context.Context) ([]calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []calendar.CalendarRecord{}
	for _, cal := range r.calendars {
		acc := r.accounts[cal.AccountID]
		if cal.SyncEnabled && cal.DeletedAt == nil && acc.Status == "connected" {
			out = append(out, cal)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) ListCalendarsNeedingWatch(_ context.Context, renewBefore time.Time) ([]calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []calendar.CalendarRecord{}
	for _, cal := range r.calendars {
		acc := r.accounts[cal.AccountID]
		if !cal.SyncEnabled || cal.DeletedAt != nil || acc.Status != "connected" {
			continue
		}
		if cal.WatchChannelID == nil || cal.WatchExpiresAt == nil || cal.WatchExpiresAt.Before(renewBefore) {
			out = append(out, cal)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) ListWatchedCalendars(_ context.Context) ([]calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []calendar.CalendarRecord{}
	for _, cal := range r.calendars {
		if cal.WatchChannelID != nil {
			out = append(out, cal)
		}
	}
	return out, nil
}

func (r *fakeRepo) UpdateCalendarPrefs(_ context.Context, id int32, p calendar.CalendarPrefs) (calendar.CalendarRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.CalendarRecord{}, calendar.ErrNotFound
	}
	if p.SyncEnabled != nil {
		cal.SyncEnabled = *p.SyncEnabled
	}
	if p.Visible != nil {
		cal.Visible = *p.Visible
	}
	if p.ColorOverride != nil {
		cal.ColorOverride = *p.ColorOverride
	}
	r.calendars[id] = cal
	return cal, nil
}

func (r *fakeRepo) SetCalendarSyncToken(_ context.Context, id int32, token string, wasFull bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.ErrNotFound
	}
	t := token
	now := time.Now()
	cal.SyncToken = &t
	cal.SyncTokenUpdatedAt = &now
	cal.LastSyncAt = &now
	cal.LastSyncError = nil
	if wasFull {
		cal.LastFullSyncAt = &now
	}
	r.calendars[id] = cal
	return nil
}

func (r *fakeRepo) ClearCalendarSyncToken(_ context.Context, id int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.ErrNotFound
	}
	cal.SyncToken = nil
	cal.SyncTokenUpdatedAt = nil
	r.calendars[id] = cal
	return nil
}

func (r *fakeRepo) ClearAccountSyncTokens(ctx context.Context, accountID int32) error {
	cals, _ := r.ListCalendarsByAccount(ctx, accountID)
	for _, cal := range cals {
		if err := r.ClearCalendarSyncToken(ctx, cal.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeRepo) SetCalendarSyncError(_ context.Context, id int32, syncError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.ErrNotFound
	}
	msg := syncError
	now := time.Now()
	cal.LastSyncError = &msg
	cal.LastSyncAt = &now
	r.calendars[id] = cal
	return nil
}

func (r *fakeRepo) MarkCalendarsDeleted(_ context.Context, accountID int32, seenIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	for _, id := range seenIDs {
		seen[id] = true
	}
	now := time.Now()
	for id, cal := range r.calendars {
		if cal.AccountID == accountID && cal.DeletedAt == nil && !seen[cal.GoogleCalendarID] {
			cal.DeletedAt = &now
			r.calendars[id] = cal
		}
	}
	return nil
}

func (r *fakeRepo) SetCalendarWatch(_ context.Context, id int32, w calendar.WatchInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.ErrNotFound
	}
	channelID, resourceID, token, expires := w.ChannelID, w.ResourceID, w.Token, w.ExpiresAt
	cal.WatchChannelID = &channelID
	cal.WatchResourceID = &resourceID
	cal.WatchToken = &token
	cal.WatchExpiresAt = &expires
	r.calendars[id] = cal
	return nil
}

func (r *fakeRepo) ClearCalendarWatch(_ context.Context, id int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cal, ok := r.calendars[id]
	if !ok {
		return calendar.ErrNotFound
	}
	cal.WatchChannelID, cal.WatchResourceID, cal.WatchToken, cal.WatchExpiresAt = nil, nil, nil, nil
	r.calendars[id] = cal
	return nil
}

func (r *fakeRepo) CountEvents(_ context.Context, calendarID int32) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, ev := range r.events {
		if ev.CalendarID == calendarID {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) UpsertEvent(_ context.Context, p calendar.UpsertEventParams) (calendar.EventRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FailUpsertEvent != nil {
		err := r.FailUpsertEvent
		r.FailUpsertEvent = nil
		return calendar.EventRecord{}, err
	}
	rec := calendar.EventRecord{
		CalendarID:       p.CalendarID,
		GoogleEventID:    p.GoogleEventID,
		ICalUID:          p.ICalUID,
		Etag:             p.Etag,
		Sequence:         p.Sequence,
		Status:           p.Status,
		EventType:        p.EventType,
		Summary:          p.Summary,
		Description:      p.Description,
		Location:         p.Location,
		AllDay:           p.AllDay,
		StartsAt:         p.StartsAt,
		EndsAt:           p.EndsAt,
		StartTZ:          p.StartTZ,
		EndTZ:            p.EndTZ,
		Recurrence:       p.Recurrence,
		RecurringEventID: p.RecurringEventID,
		OriginalStartsAt: p.OriginalStartsAt,
		OrganizerEmail:   p.OrganizerEmail,
		CreatorEmail:     p.CreatorEmail,
		Attendees:        p.Attendees,
		Reminders:        p.Reminders,
		Transparency:     p.Transparency,
		Visibility:       p.Visibility,
		ColorID:          p.ColorID,
		HTMLLink:         p.HTMLLink,
		HangoutLink:      p.HangoutLink,
		CreatedByGV:      p.CreatedByGV,
		GoogleUpdatedAt:  p.GoogleUpdatedAt,
	}
	for id, existing := range r.events {
		if existing.CalendarID == p.CalendarID && existing.GoogleEventID == p.GoogleEventID {
			rec.ID = id
			rec.MasterID = existing.MasterID
			rec.CreatedByGV = existing.CreatedByGV || p.CreatedByGV
			r.events[id] = rec
			return rec, nil
		}
	}
	r.nextEvent++
	rec.ID = r.nextEvent
	r.events[rec.ID] = rec
	return rec, nil
}

func (r *fakeRepo) GetEvent(_ context.Context, id int32) (calendar.EventRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ev, ok := r.events[id]
	if !ok {
		return calendar.EventRecord{}, calendar.ErrNotFound
	}
	return ev, nil
}

func (r *fakeRepo) GetEventByGoogleID(_ context.Context, calendarID int32, googleEventID string) (calendar.EventRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ev := range r.events {
		if ev.CalendarID == calendarID && ev.GoogleEventID == googleEventID {
			return ev, nil
		}
	}
	return calendar.EventRecord{}, calendar.ErrNotFound
}

func (r *fakeRepo) DeleteEvent(_ context.Context, id int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.events[id]; !ok {
		return calendar.ErrNotFound
	}
	delete(r.events, id)
	// Overrides go with their master, like the ON DELETE CASCADE does.
	for evID, ev := range r.events {
		if ev.MasterID != nil && *ev.MasterID == id {
			delete(r.events, evID)
		}
	}
	return nil
}

func (r *fakeRepo) DeleteEventByGoogleID(_ context.Context, calendarID int32, googleEventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, ev := range r.events {
		if ev.CalendarID == calendarID && ev.GoogleEventID == googleEventID {
			delete(r.events, id)
		}
	}
	return nil
}

func (r *fakeRepo) DeleteEventsForCalendar(_ context.Context, calendarID int32) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for id, ev := range r.events {
		if ev.CalendarID == calendarID {
			delete(r.events, id)
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) LinkEventMasters(_ context.Context, calendarID int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	byGoogleID := map[string]int32{}
	for id, ev := range r.events {
		if ev.CalendarID == calendarID {
			byGoogleID[ev.GoogleEventID] = id
		}
	}
	for id, ev := range r.events {
		if ev.CalendarID != calendarID || ev.RecurringEventID == nil || ev.MasterID != nil {
			continue
		}
		if masterID, ok := byGoogleID[*ev.RecurringEventID]; ok {
			m := masterID
			ev.MasterID = &m
			r.events[id] = ev
		}
	}
	return nil
}

func (r *fakeRepo) ListEventsInRange(_ context.Context, calendarIDs []int32, from, to time.Time) ([]calendar.EventRecord, error) {
	return r.filterEvents(calendarIDs, func(ev calendar.EventRecord) bool {
		if len(ev.Recurrence) > 0 || ev.RecurringEventID != nil || ev.Status == "cancelled" {
			return false
		}
		return ev.StartsAt.Before(to) &&
			(ev.EndsAt.After(from) || (ev.EndsAt.Equal(ev.StartsAt) && !ev.StartsAt.Before(from)))
	}), nil
}

func (r *fakeRepo) ListRecurringMasters(_ context.Context, calendarIDs []int32, to time.Time) ([]calendar.EventRecord, error) {
	return r.filterEvents(calendarIDs, func(ev calendar.EventRecord) bool {
		return len(ev.Recurrence) > 0 && ev.Status != "cancelled" && ev.StartsAt.Before(to)
	}), nil
}

func (r *fakeRepo) ListEventExceptions(_ context.Context, masterIDs []int32) ([]calendar.EventRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wanted := map[int32]bool{}
	for _, id := range masterIDs {
		wanted[id] = true
	}
	out := []calendar.EventRecord{}
	for _, ev := range r.events {
		if ev.MasterID != nil && wanted[*ev.MasterID] {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRepo) ListOrphanOverridesInRange(_ context.Context, calendarIDs []int32, from, to time.Time) ([]calendar.EventRecord, error) {
	return r.filterEvents(calendarIDs, func(ev calendar.EventRecord) bool {
		if ev.RecurringEventID == nil || ev.MasterID != nil || ev.Status == "cancelled" {
			return false
		}
		return ev.StartsAt.Before(to) &&
			(ev.EndsAt.After(from) || (ev.EndsAt.Equal(ev.StartsAt) && !ev.StartsAt.Before(from)))
	}), nil
}

func (r *fakeRepo) filterEvents(calendarIDs []int32, keep func(calendar.EventRecord) bool) []calendar.EventRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	wanted := map[int32]bool{}
	for _, id := range calendarIDs {
		wanted[id] = true
	}
	out := []calendar.EventRecord{}
	for _, ev := range r.events {
		if wanted[ev.CalendarID] && keep(ev) {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out
}

func (r *fakeRepo) PurgeCancelledEvents(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (r *fakeRepo) CreateSyncRun(_ context.Context, calendarID int32, trigger, kind string) (int32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextRun++
	id := calendarID
	r.runs = append(r.runs, calendar.SyncRun{
		ID: r.nextRun, CalendarID: &id, Trigger: trigger, Kind: kind, StartedAt: time.Now(),
	})
	return r.nextRun, nil
}

func (r *fakeRepo) FinishSyncRun(_ context.Context, id int32, pages, upserted, deleted int32, runError *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.runs {
		if r.runs[i].ID == id {
			now := time.Now()
			r.runs[i].FinishedAt = &now
			r.runs[i].Pages = pages
			r.runs[i].Upserted = upserted
			r.runs[i].Deleted = deleted
			r.runs[i].Error = runError
		}
	}
	return nil
}

func (r *fakeRepo) ListRecentSyncRuns(_ context.Context, limit int32) ([]calendar.SyncRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]calendar.SyncRun, 0, len(r.runs))
	for i := len(r.runs) - 1; i >= 0 && int32(len(out)) < limit; i-- {
		out = append(out, r.runs[i])
	}
	return out, nil
}

// eventCount is a test helper: how many rows are stored for a calendar.
func (r *fakeRepo) eventCount(calendarID int32) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, ev := range r.events {
		if ev.CalendarID == calendarID {
			n++
		}
	}
	return n
}

// lastRun is the most recent sync run, for asserting what a pass recorded.
func (r *fakeRepo) lastRun() calendar.SyncRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runs) == 0 {
		return calendar.SyncRun{}
	}
	return r.runs[len(r.runs)-1]
}

var _ calendar.Repository = (*fakeRepo)(nil)
