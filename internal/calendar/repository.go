package calendar

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/database/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the calendar domain's data access. It maps rows to records and back and
// makes no decisions: what a spent sync token means, when a token is refreshed and which
// fields a write sends all live in the service.
type Repository interface {
	UpsertAccount(ctx context.Context, p UpsertAccountParams) (AccountRecord, error)
	GetAccount(ctx context.Context, id int32) (AccountRecord, error)
	ListAccounts(ctx context.Context) ([]AccountRecord, error)
	UpdateAccountAccessToken(ctx context.Context, id int32, sealed []byte, expiresAt time.Time) error
	UpdateAccountStatus(ctx context.Context, id int32, status string, syncError *string) error
	UpdateAccountMeta(ctx context.Context, id int32, label, color *string) (AccountRecord, error)
	TouchAccountSync(ctx context.Context, id int32, syncError *string) error
	DeleteAccount(ctx context.Context, id int32) error

	UpsertCalendar(ctx context.Context, p UpsertCalendarParams) (CalendarRecord, error)
	GetCalendar(ctx context.Context, id int32) (CalendarRecord, error)
	GetCalendarByChannel(ctx context.Context, channelID string) (CalendarRecord, error)
	ListCalendars(ctx context.Context) ([]CalendarView, error)
	ListCalendarsByAccount(ctx context.Context, accountID int32) ([]CalendarRecord, error)
	ListSyncableCalendars(ctx context.Context) ([]CalendarRecord, error)
	ListCalendarsNeedingWatch(ctx context.Context, renewBefore time.Time) ([]CalendarRecord, error)
	ListWatchedCalendars(ctx context.Context) ([]CalendarRecord, error)
	UpdateCalendarPrefs(ctx context.Context, id int32, p CalendarPrefs) (CalendarRecord, error)
	SetCalendarSyncToken(ctx context.Context, id int32, token string, wasFull bool) error
	ClearCalendarSyncToken(ctx context.Context, id int32) error
	ClearAccountSyncTokens(ctx context.Context, accountID int32) error
	SetCalendarSyncError(ctx context.Context, id int32, syncError string) error
	MarkCalendarsDeleted(ctx context.Context, accountID int32, seenIDs []string) error
	SetCalendarWatch(ctx context.Context, id int32, w WatchInfo) error
	ClearCalendarWatch(ctx context.Context, id int32) error
	CountEvents(ctx context.Context, calendarID int32) (int64, error)

	UpsertEvent(ctx context.Context, p UpsertEventParams) (EventRecord, error)
	GetEvent(ctx context.Context, id int32) (EventRecord, error)
	GetEventByGoogleID(ctx context.Context, calendarID int32, googleEventID string) (EventRecord, error)
	DeleteEvent(ctx context.Context, id int32) error
	DeleteEventByGoogleID(ctx context.Context, calendarID int32, googleEventID string) error
	DeleteEventsForCalendar(ctx context.Context, calendarID int32) (int64, error)
	LinkEventMasters(ctx context.Context, calendarID int32) error
	ListEventsInRange(ctx context.Context, calendarIDs []int32, from, to time.Time) ([]EventRecord, error)
	ListRecurringMasters(ctx context.Context, calendarIDs []int32, to time.Time) ([]EventRecord, error)
	ListEventExceptions(ctx context.Context, masterIDs []int32) ([]EventRecord, error)
	ListOrphanOverridesInRange(ctx context.Context, calendarIDs []int32, from, to time.Time) ([]EventRecord, error)
	PurgeCancelledEvents(ctx context.Context, olderThan time.Time) (int64, error)

	CreateSyncRun(ctx context.Context, calendarID int32, trigger, kind string) (int32, error)
	FinishSyncRun(ctx context.Context, id int32, pages, upserted, deleted int32, runError *string) error
	ListRecentSyncRuns(ctx context.Context, limit int32) ([]SyncRun, error)
}

type PostgresRepository struct {
	q *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: gvdb.New(pool)}
}

// --- conversions ---------------------------------------------------------------------

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: !t.IsZero()}
}

func tsPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func accountToRecord(a gvdb.GoogleAccount) AccountRecord {
	return AccountRecord{
		ID:                   a.ID,
		Email:                a.Email,
		Label:                a.Label,
		Color:                a.Color,
		RefreshTokenSealed:   a.RefreshToken,
		AccessTokenSealed:    a.AccessToken,
		AccessTokenExpiresAt: pgconv.TimePtr(a.AccessTokenExpiresAt),
		Scopes:               a.Scopes,
		Status:               a.Status,
		LastSyncAt:           pgconv.TimePtr(a.LastSyncAt),
		LastSyncError:        a.LastSyncError,
		CreatedAt:            a.CreatedAt.Time,
	}
}

func calendarToRecord(c gvdb.Calendar) CalendarRecord {
	return CalendarRecord{
		ID:                 c.ID,
		AccountID:          c.AccountID,
		GoogleCalendarID:   c.GoogleCalendarID,
		Summary:            c.Summary,
		Description:        c.Description,
		TimeZone:           c.TimeZone,
		BackgroundColor:    c.BackgroundColor,
		ForegroundColor:    c.ForegroundColor,
		ColorOverride:      c.ColorOverride,
		AccessRole:         c.AccessRole,
		IsPrimary:          c.IsPrimary,
		SyncEnabled:        c.SyncEnabled,
		Visible:            c.Visible,
		SyncToken:          c.SyncToken,
		SyncTokenUpdatedAt: pgconv.TimePtr(c.SyncTokenUpdatedAt),
		LastFullSyncAt:     pgconv.TimePtr(c.LastFullSyncAt),
		LastSyncAt:         pgconv.TimePtr(c.LastSyncAt),
		LastSyncError:      c.LastSyncError,
		WatchChannelID:     c.WatchChannelID,
		WatchResourceID:    c.WatchResourceID,
		WatchToken:         c.WatchToken,
		WatchExpiresAt:     pgconv.TimePtr(c.WatchExpiresAt),
		DeletedAt:          pgconv.TimePtr(c.DeletedAt),
	}
}

func eventToRecord(e gvdb.CalendarEvent) EventRecord {
	return EventRecord{
		ID:               e.ID,
		CalendarID:       e.CalendarID,
		GoogleEventID:    e.GoogleEventID,
		ICalUID:          e.IcalUid,
		Etag:             e.Etag,
		Sequence:         e.Sequence,
		Status:           e.Status,
		EventType:        e.EventType,
		Summary:          e.Summary,
		Description:      e.Description,
		Location:         e.Location,
		AllDay:           e.AllDay,
		StartsAt:         e.StartsAt.Time,
		EndsAt:           e.EndsAt.Time,
		StartTZ:          e.StartTz,
		EndTZ:            e.EndTz,
		Recurrence:       e.Recurrence,
		RecurringEventID: e.RecurringEventID,
		MasterID:         e.MasterID,
		OriginalStartsAt: pgconv.TimePtr(e.OriginalStartsAt),
		OrganizerEmail:   e.OrganizerEmail,
		CreatorEmail:     e.CreatorEmail,
		Attendees:        e.Attendees,
		Reminders:        e.Reminders,
		Transparency:     e.Transparency,
		Visibility:       e.Visibility,
		ColorID:          e.ColorID,
		HTMLLink:         e.HtmlLink,
		HangoutLink:      e.HangoutLink,
		CreatedByGV:      e.CreatedByGv,
		GoogleUpdatedAt:  pgconv.TimePtr(e.GoogleUpdatedAt),
	}
}

func eventsToRecords(rows []gvdb.CalendarEvent) []EventRecord {
	out := make([]EventRecord, len(rows))
	for i, row := range rows {
		out[i] = eventToRecord(row)
	}
	return out
}

func syncRunToRecord(r gvdb.CalendarSyncRun) SyncRun {
	return SyncRun{
		ID:         r.ID,
		CalendarID: r.CalendarID,
		Trigger:    r.Trigger,
		Kind:       r.Kind,
		StartedAt:  r.StartedAt.Time,
		FinishedAt: pgconv.TimePtr(r.FinishedAt),
		Pages:      r.Pages,
		Upserted:   r.Upserted,
		Deleted:    r.Deleted,
		Error:      r.Error,
	}
}

// --- accounts ------------------------------------------------------------------------

func (r *PostgresRepository) UpsertAccount(ctx context.Context, p UpsertAccountParams) (AccountRecord, error) {
	row, err := r.q.UpsertGoogleAccount(ctx, gvdb.UpsertGoogleAccountParams{
		Email:                p.Email,
		RefreshToken:         p.RefreshTokenSealed,
		AccessToken:          p.AccessTokenSealed,
		AccessTokenExpiresAt: tsPtr(p.AccessTokenExpiresAt),
		Scopes:               p.Scopes,
	})
	if err != nil {
		return AccountRecord{}, err
	}
	return accountToRecord(row), nil
}

func (r *PostgresRepository) GetAccount(ctx context.Context, id int32) (AccountRecord, error) {
	row, err := r.q.GetGoogleAccount(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountRecord{}, ErrNotFound
		}
		return AccountRecord{}, err
	}
	return accountToRecord(row), nil
}

func (r *PostgresRepository) ListAccounts(ctx context.Context) ([]AccountRecord, error) {
	rows, err := r.q.ListGoogleAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountRecord, len(rows))
	for i, row := range rows {
		out[i] = accountToRecord(row)
	}
	return out, nil
}

func (r *PostgresRepository) UpdateAccountAccessToken(ctx context.Context, id int32, sealed []byte, expiresAt time.Time) error {
	return r.q.UpdateGoogleAccountAccessToken(ctx, gvdb.UpdateGoogleAccountAccessTokenParams{
		ID:                   id,
		AccessToken:          sealed,
		AccessTokenExpiresAt: ts(expiresAt),
	})
}

func (r *PostgresRepository) UpdateAccountStatus(ctx context.Context, id int32, status string, syncError *string) error {
	return r.q.UpdateGoogleAccountStatus(ctx, gvdb.UpdateGoogleAccountStatusParams{
		ID: id, Status: status, LastSyncError: syncError,
	})
}

func (r *PostgresRepository) UpdateAccountMeta(ctx context.Context, id int32, label, color *string) (AccountRecord, error) {
	row, err := r.q.UpdateGoogleAccountMeta(ctx, gvdb.UpdateGoogleAccountMetaParams{
		ID: id, Label: label, Color: color,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountRecord{}, ErrNotFound
		}
		return AccountRecord{}, err
	}
	return accountToRecord(row), nil
}

func (r *PostgresRepository) TouchAccountSync(ctx context.Context, id int32, syncError *string) error {
	return r.q.TouchGoogleAccountSync(ctx, gvdb.TouchGoogleAccountSyncParams{ID: id, LastSyncError: syncError})
}

func (r *PostgresRepository) DeleteAccount(ctx context.Context, id int32) error {
	rows, err := r.q.DeleteGoogleAccount(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// --- calendars -----------------------------------------------------------------------

func (r *PostgresRepository) UpsertCalendar(ctx context.Context, p UpsertCalendarParams) (CalendarRecord, error) {
	row, err := r.q.UpsertCalendar(ctx, gvdb.UpsertCalendarParams{
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
	})
	if err != nil {
		return CalendarRecord{}, err
	}
	return calendarToRecord(row), nil
}

func (r *PostgresRepository) GetCalendar(ctx context.Context, id int32) (CalendarRecord, error) {
	row, err := r.q.GetCalendar(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarRecord{}, ErrNotFound
		}
		return CalendarRecord{}, err
	}
	return calendarToRecord(row), nil
}

func (r *PostgresRepository) GetCalendarByChannel(ctx context.Context, channelID string) (CalendarRecord, error) {
	row, err := r.q.GetCalendarByChannelID(ctx, &channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarRecord{}, ErrNotFound
		}
		return CalendarRecord{}, err
	}
	return calendarToRecord(row), nil
}

func (r *PostgresRepository) ListCalendars(ctx context.Context) ([]CalendarView, error) {
	rows, err := r.q.ListCalendars(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CalendarView, len(rows))
	for i, row := range rows {
		out[i] = CalendarView{
			CalendarRecord: CalendarRecord{
				ID:                 row.ID,
				AccountID:          row.AccountID,
				GoogleCalendarID:   row.GoogleCalendarID,
				Summary:            row.Summary,
				Description:        row.Description,
				TimeZone:           row.TimeZone,
				BackgroundColor:    row.BackgroundColor,
				ForegroundColor:    row.ForegroundColor,
				ColorOverride:      row.ColorOverride,
				AccessRole:         row.AccessRole,
				IsPrimary:          row.IsPrimary,
				SyncEnabled:        row.SyncEnabled,
				Visible:            row.Visible,
				SyncToken:          row.SyncToken,
				SyncTokenUpdatedAt: pgconv.TimePtr(row.SyncTokenUpdatedAt),
				LastFullSyncAt:     pgconv.TimePtr(row.LastFullSyncAt),
				LastSyncAt:         pgconv.TimePtr(row.LastSyncAt),
				LastSyncError:      row.LastSyncError,
				WatchChannelID:     row.WatchChannelID,
				WatchResourceID:    row.WatchResourceID,
				WatchToken:         row.WatchToken,
				WatchExpiresAt:     pgconv.TimePtr(row.WatchExpiresAt),
				DeletedAt:          pgconv.TimePtr(row.DeletedAt),
			},
			AccountEmail:  row.AccountEmail,
			AccountLabel:  row.AccountLabel,
			AccountColor:  row.AccountColor,
			AccountStatus: row.AccountStatus,
		}
	}
	return out, nil
}

func (r *PostgresRepository) ListCalendarsByAccount(ctx context.Context, accountID int32) ([]CalendarRecord, error) {
	rows, err := r.q.ListCalendarsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return calendarsToRecords(rows), nil
}

func (r *PostgresRepository) ListSyncableCalendars(ctx context.Context) ([]CalendarRecord, error) {
	rows, err := r.q.ListSyncableCalendars(ctx)
	if err != nil {
		return nil, err
	}
	return calendarsToRecords(rows), nil
}

func (r *PostgresRepository) ListCalendarsNeedingWatch(ctx context.Context, renewBefore time.Time) ([]CalendarRecord, error) {
	rows, err := r.q.ListCalendarsNeedingWatch(ctx, ts(renewBefore))
	if err != nil {
		return nil, err
	}
	return calendarsToRecords(rows), nil
}

func (r *PostgresRepository) ListWatchedCalendars(ctx context.Context) ([]CalendarRecord, error) {
	rows, err := r.q.ListWatchedCalendars(ctx)
	if err != nil {
		return nil, err
	}
	return calendarsToRecords(rows), nil
}

func calendarsToRecords(rows []gvdb.Calendar) []CalendarRecord {
	out := make([]CalendarRecord, len(rows))
	for i, row := range rows {
		out[i] = calendarToRecord(row)
	}
	return out
}

func (r *PostgresRepository) UpdateCalendarPrefs(ctx context.Context, id int32, p CalendarPrefs) (CalendarRecord, error) {
	row, err := r.q.UpdateCalendarPrefs(ctx, gvdb.UpdateCalendarPrefsParams{
		ID: id, SyncEnabled: p.SyncEnabled, Visible: p.Visible, ColorOverride: p.ColorOverride,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarRecord{}, ErrNotFound
		}
		return CalendarRecord{}, err
	}
	return calendarToRecord(row), nil
}

func (r *PostgresRepository) SetCalendarSyncToken(ctx context.Context, id int32, token string, wasFull bool) error {
	return r.q.SetCalendarSyncToken(ctx, gvdb.SetCalendarSyncTokenParams{
		ID: id, SyncToken: strPtr(token), WasFull: wasFull,
	})
}

func (r *PostgresRepository) ClearCalendarSyncToken(ctx context.Context, id int32) error {
	return r.q.ClearCalendarSyncToken(ctx, id)
}

func (r *PostgresRepository) ClearAccountSyncTokens(ctx context.Context, accountID int32) error {
	return r.q.ClearAccountSyncTokens(ctx, accountID)
}

func (r *PostgresRepository) SetCalendarSyncError(ctx context.Context, id int32, syncError string) error {
	return r.q.SetCalendarSyncError(ctx, gvdb.SetCalendarSyncErrorParams{ID: id, LastSyncError: strPtr(syncError)})
}

func (r *PostgresRepository) MarkCalendarsDeleted(ctx context.Context, accountID int32, seenIDs []string) error {
	if seenIDs == nil {
		seenIDs = []string{}
	}
	return r.q.MarkCalendarsDeleted(ctx, gvdb.MarkCalendarsDeletedParams{AccountID: accountID, SeenIds: seenIDs})
}

func (r *PostgresRepository) SetCalendarWatch(ctx context.Context, id int32, w WatchInfo) error {
	return r.q.SetCalendarWatch(ctx, gvdb.SetCalendarWatchParams{
		ID:              id,
		WatchChannelID:  strPtr(w.ChannelID),
		WatchResourceID: strPtr(w.ResourceID),
		WatchToken:      strPtr(w.Token),
		WatchExpiresAt:  ts(w.ExpiresAt),
	})
}

func (r *PostgresRepository) ClearCalendarWatch(ctx context.Context, id int32) error {
	return r.q.ClearCalendarWatch(ctx, id)
}

func (r *PostgresRepository) CountEvents(ctx context.Context, calendarID int32) (int64, error) {
	return r.q.CountCalendarEvents(ctx, calendarID)
}

// --- events --------------------------------------------------------------------------

func (r *PostgresRepository) UpsertEvent(ctx context.Context, p UpsertEventParams) (EventRecord, error) {
	row, err := r.q.UpsertCalendarEvent(ctx, gvdb.UpsertCalendarEventParams{
		CalendarID:       p.CalendarID,
		GoogleEventID:    p.GoogleEventID,
		IcalUid:          p.ICalUID,
		Etag:             p.Etag,
		Sequence:         p.Sequence,
		Status:           p.Status,
		EventType:        p.EventType,
		Summary:          p.Summary,
		Description:      p.Description,
		Location:         p.Location,
		AllDay:           p.AllDay,
		StartsAt:         ts(p.StartsAt),
		EndsAt:           ts(p.EndsAt),
		StartTz:          p.StartTZ,
		EndTz:            p.EndTZ,
		Recurrence:       p.Recurrence,
		RecurringEventID: p.RecurringEventID,
		OriginalStartsAt: tsPtr(p.OriginalStartsAt),
		OrganizerEmail:   p.OrganizerEmail,
		CreatorEmail:     p.CreatorEmail,
		Attendees:        p.Attendees,
		Reminders:        p.Reminders,
		Transparency:     p.Transparency,
		Visibility:       p.Visibility,
		ColorID:          p.ColorID,
		HtmlLink:         p.HTMLLink,
		HangoutLink:      p.HangoutLink,
		CreatedByGv:      p.CreatedByGV,
		GoogleUpdatedAt:  tsPtr(p.GoogleUpdatedAt),
	})
	if err != nil {
		return EventRecord{}, err
	}
	return eventToRecord(row), nil
}

func (r *PostgresRepository) GetEvent(ctx context.Context, id int32) (EventRecord, error) {
	row, err := r.q.GetCalendarEvent(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventRecord{}, ErrNotFound
		}
		return EventRecord{}, err
	}
	return eventToRecord(row), nil
}

func (r *PostgresRepository) GetEventByGoogleID(ctx context.Context, calendarID int32, googleEventID string) (EventRecord, error) {
	row, err := r.q.GetCalendarEventByGoogleID(ctx, gvdb.GetCalendarEventByGoogleIDParams{
		CalendarID: calendarID, GoogleEventID: googleEventID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EventRecord{}, ErrNotFound
		}
		return EventRecord{}, err
	}
	return eventToRecord(row), nil
}

func (r *PostgresRepository) DeleteEvent(ctx context.Context, id int32) error {
	rows, err := r.q.DeleteCalendarEvent(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) DeleteEventByGoogleID(ctx context.Context, calendarID int32, googleEventID string) error {
	_, err := r.q.DeleteCalendarEventByGoogleID(ctx, gvdb.DeleteCalendarEventByGoogleIDParams{
		CalendarID: calendarID, GoogleEventID: googleEventID,
	})
	return err
}

func (r *PostgresRepository) DeleteEventsForCalendar(ctx context.Context, calendarID int32) (int64, error) {
	return r.q.DeleteCalendarEventsForCalendar(ctx, calendarID)
}

func (r *PostgresRepository) LinkEventMasters(ctx context.Context, calendarID int32) error {
	return r.q.LinkCalendarEventMasters(ctx, calendarID)
}

func (r *PostgresRepository) ListEventsInRange(ctx context.Context, calendarIDs []int32, from, to time.Time) ([]EventRecord, error) {
	rows, err := r.q.ListEventsInRange(ctx, gvdb.ListEventsInRangeParams{
		CalendarIds: calendarIDs, RangeStart: ts(from), RangeEnd: ts(to),
	})
	if err != nil {
		return nil, err
	}
	return eventsToRecords(rows), nil
}

func (r *PostgresRepository) ListRecurringMasters(ctx context.Context, calendarIDs []int32, to time.Time) ([]EventRecord, error) {
	rows, err := r.q.ListRecurringMasters(ctx, gvdb.ListRecurringMastersParams{
		CalendarIds: calendarIDs, RangeEnd: ts(to),
	})
	if err != nil {
		return nil, err
	}
	return eventsToRecords(rows), nil
}

func (r *PostgresRepository) ListEventExceptions(ctx context.Context, masterIDs []int32) ([]EventRecord, error) {
	if len(masterIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListEventExceptions(ctx, masterIDs)
	if err != nil {
		return nil, err
	}
	return eventsToRecords(rows), nil
}

func (r *PostgresRepository) ListOrphanOverridesInRange(ctx context.Context, calendarIDs []int32, from, to time.Time) ([]EventRecord, error) {
	rows, err := r.q.ListOrphanOverridesInRange(ctx, gvdb.ListOrphanOverridesInRangeParams{
		CalendarIds: calendarIDs, RangeStart: ts(from), RangeEnd: ts(to),
	})
	if err != nil {
		return nil, err
	}
	return eventsToRecords(rows), nil
}

func (r *PostgresRepository) PurgeCancelledEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	return r.q.PurgeCancelledEvents(ctx, ts(olderThan))
}

// --- sync runs -----------------------------------------------------------------------

func (r *PostgresRepository) CreateSyncRun(ctx context.Context, calendarID int32, trigger, kind string) (int32, error) {
	row, err := r.q.CreateSyncRun(ctx, gvdb.CreateSyncRunParams{
		CalendarID: &calendarID, Trigger: trigger, Kind: kind,
	})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (r *PostgresRepository) FinishSyncRun(ctx context.Context, id int32, pages, upserted, deleted int32, runError *string) error {
	return r.q.FinishSyncRun(ctx, gvdb.FinishSyncRunParams{
		ID: id, Pages: pages, Upserted: upserted, Deleted: deleted, Error: runError,
	})
}

func (r *PostgresRepository) ListRecentSyncRuns(ctx context.Context, limit int32) ([]SyncRun, error) {
	rows, err := r.q.ListRecentSyncRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]SyncRun, len(rows))
	for i, row := range rows {
		out[i] = syncRunToRecord(row)
	}
	return out, nil
}
