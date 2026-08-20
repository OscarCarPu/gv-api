package calendar

import "time"

// --- Stored records ------------------------------------------------------------------
//
// These mirror the rows. They carry the sync bookkeeping the service needs (tokens, etags,
// channels) and never leave the package; what the HTTP layer returns is further down.

type AccountRecord struct {
	ID                   int32
	Email                string
	Label                string
	Color                string
	RefreshTokenSealed   []byte
	AccessTokenSealed    []byte
	AccessTokenExpiresAt *time.Time
	Scopes               string
	Status               string
	LastSyncAt           *time.Time
	LastSyncError        *string
	CreatedAt            time.Time
}

type CalendarRecord struct {
	ID                 int32
	AccountID          int32
	GoogleCalendarID   string
	Summary            string
	Description        string
	TimeZone           string
	BackgroundColor    string
	ForegroundColor    string
	ColorOverride      string
	AccessRole         string
	IsPrimary          bool
	SyncEnabled        bool
	Visible            bool
	SyncToken          *string
	SyncTokenUpdatedAt *time.Time
	LastFullSyncAt     *time.Time
	LastSyncAt         *time.Time
	LastSyncError      *string
	WatchChannelID     *string
	WatchResourceID    *string
	WatchToken         *string
	WatchExpiresAt     *time.Time
	DeletedAt          *time.Time
}

// Writable mirrors Google's access roles: reader and freeBusyReader cannot be written to, and
// the service refuses those writes rather than letting Google answer 403 after the fact.
func (c CalendarRecord) Writable() bool {
	return c.AccessRole == "owner" || c.AccessRole == "writer"
}

type EventRecord struct {
	ID               int32
	CalendarID       int32
	GoogleEventID    string
	ICalUID          string
	Etag             string
	Sequence         int32
	Status           string
	EventType        string
	Summary          string
	Description      string
	Location         string
	AllDay           bool
	StartsAt         time.Time
	EndsAt           time.Time
	StartTZ          string
	EndTZ            string
	Recurrence       []string
	RecurringEventID *string
	MasterID         *int32
	OriginalStartsAt *time.Time
	OrganizerEmail   string
	CreatorEmail     string
	Attendees        []byte
	Reminders        []byte
	Transparency     string
	Visibility       string
	ColorID          string
	HTMLLink         string
	HangoutLink      string
	CreatedByGV      bool
	GoogleUpdatedAt  *time.Time
}

// IsMaster reports a recurring series' defining row.
func (e EventRecord) IsMaster() bool { return len(e.Recurrence) > 0 }

// IsException reports a row that overrides or cancels one occurrence of a series.
func (e EventRecord) IsException() bool {
	return e.RecurringEventID != nil && *e.RecurringEventID != ""
}

type CalendarView struct {
	CalendarRecord
	AccountEmail  string
	AccountLabel  string
	AccountColor  string
	AccountStatus string
}

type SyncRun struct {
	ID         int32
	CalendarID *int32
	Trigger    string
	Kind       string
	StartedAt  time.Time
	FinishedAt *time.Time
	Pages      int32
	Upserted   int32
	Deleted    int32
	Error      *string
}

// --- Repository parameters -----------------------------------------------------------

type UpsertAccountParams struct {
	Email                string
	RefreshTokenSealed   []byte
	AccessTokenSealed    []byte
	AccessTokenExpiresAt *time.Time
	Scopes               string
}

type UpsertCalendarParams struct {
	AccountID        int32
	GoogleCalendarID string
	Summary          string
	Description      string
	TimeZone         string
	BackgroundColor  string
	ForegroundColor  string
	AccessRole       string
	IsPrimary        bool
	// SyncEnabled only applies on insert; an existing row keeps the user's choice.
	SyncEnabled bool
}

type CalendarPrefs struct {
	SyncEnabled   *bool
	Visible       *bool
	ColorOverride *string
}

type WatchInfo struct {
	ChannelID  string
	ResourceID string
	Token      string
	ExpiresAt  time.Time
}

type UpsertEventParams struct {
	CalendarID       int32
	GoogleEventID    string
	ICalUID          string
	Etag             string
	Sequence         int32
	Status           string
	EventType        string
	Summary          string
	Description      string
	Location         string
	AllDay           bool
	StartsAt         time.Time
	EndsAt           time.Time
	StartTZ          string
	EndTZ            string
	Recurrence       []string
	RecurringEventID *string
	OriginalStartsAt *time.Time
	OrganizerEmail   string
	CreatorEmail     string
	Attendees        []byte
	Reminders        []byte
	Transparency     string
	Visibility       string
	ColorID          string
	HTMLLink         string
	HangoutLink      string
	CreatedByGV      bool
	GoogleUpdatedAt  *time.Time
}

// --- HTTP responses ------------------------------------------------------------------

type Account struct {
	ID            int32      `json:"id"`
	Email         string     `json:"email"`
	Label         string     `json:"label"`
	Color         string     `json:"color"`
	Status        string     `json:"status"`
	Calendars     int        `json:"calendars"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	LastSyncError *string    `json:"last_sync_error"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CalendarSyncState struct {
	HasSyncToken   bool       `json:"has_sync_token"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
	LastFullSyncAt *time.Time `json:"last_full_sync_at"`
	LastSyncError  *string    `json:"last_sync_error"`
	// WatchActive is the difference between "changes arrive in seconds" and "changes arrive
	// on the next poll", so it is part of the contract rather than a debug detail.
	WatchActive    bool       `json:"watch_active"`
	WatchExpiresAt *time.Time `json:"watch_expires_at"`
}

type Calendar struct {
	ID               int32             `json:"id"`
	AccountID        int32             `json:"account_id"`
	AccountEmail     string            `json:"account_email"`
	AccountStatus    string            `json:"account_status"`
	GoogleCalendarID string            `json:"google_calendar_id"`
	Summary          string            `json:"summary"`
	Description      string            `json:"description"`
	TimeZone         string            `json:"time_zone"`
	Color            string            `json:"color"`
	ForegroundColor  string            `json:"foreground_color"`
	AccessRole       string            `json:"access_role"`
	Writable         bool              `json:"writable"`
	IsPrimary        bool              `json:"is_primary"`
	SyncEnabled      bool              `json:"sync_enabled"`
	Visible          bool              `json:"visible"`
	Deleted          bool              `json:"deleted"`
	Sync             CalendarSyncState `json:"sync"`
}

type Attendee struct {
	Email          string `json:"email"`
	DisplayName    string `json:"display_name,omitempty"`
	Optional       bool   `json:"optional,omitempty"`
	ResponseStatus string `json:"response_status,omitempty"`
	Self           bool   `json:"self,omitempty"`
	Organizer      bool   `json:"organizer,omitempty"`
}

type ReminderOverride struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

type Reminders struct {
	UseDefault bool               `json:"use_default"`
	Overrides  []ReminderOverride `json:"overrides,omitempty"`
}

/*
Event is one thing on the calendar at one time — a one-off event or a single occurrence of a
series, already expanded.

InstanceID is what a client edits or deletes: "12" for a plain event, "12@2026-08-20T07:00:00Z"
for an occurrence of series 12 starting at that instant. The suffix is the *original* start,
not the current one, because that is the only stable name an occurrence has: Google identifies
a moved instance by the slot it came from.
*/
type Event struct {
	InstanceID   string `json:"instance_id"`
	EventID      int32  `json:"event_id"`
	CalendarID   int32  `json:"calendar_id"`
	AccountID    int32  `json:"account_id"`
	AccountEmail string `json:"account_email"`
	CalendarName string `json:"calendar_name"`
	Color        string `json:"color"`

	GoogleEventID string `json:"google_event_id"`
	Summary       string `json:"summary"`
	Description   string `json:"description"`
	Location      string `json:"location"`
	Status        string `json:"status"`
	EventType     string `json:"event_type"`

	AllDay   bool      `json:"all_day"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	TimeZone string    `json:"time_zone"`

	Recurring        bool       `json:"recurring"`
	Recurrence       []string   `json:"recurrence,omitempty"`
	IsException      bool       `json:"is_exception"`
	OriginalStartsAt *time.Time `json:"original_starts_at,omitempty"`

	Editable       bool       `json:"editable"`
	OrganizerEmail string     `json:"organizer_email,omitempty"`
	Attendees      []Attendee `json:"attendees,omitempty"`
	Reminders      *Reminders `json:"reminders,omitempty"`
	Transparency   string     `json:"transparency,omitempty"`
	Visibility     string     `json:"visibility,omitempty"`
	HTMLLink       string     `json:"html_link,omitempty"`
	HangoutLink    string     `json:"hangout_link,omitempty"`
	CreatedByGV    bool       `json:"created_by_gv"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

type SyncStatus struct {
	Configured     bool             `json:"configured"`
	WebhooksActive bool             `json:"webhooks_active"`
	PollInterval   string           `json:"poll_interval"`
	Accounts       []Account        `json:"accounts"`
	Calendars      []CalendarSync   `json:"calendars"`
	RecentRuns     []SyncRunSummary `json:"recent_runs"`
}

type CalendarSync struct {
	CalendarID   int32             `json:"calendar_id"`
	AccountEmail string            `json:"account_email"`
	Summary      string            `json:"summary"`
	SyncEnabled  bool              `json:"sync_enabled"`
	Events       int64             `json:"events"`
	Sync         CalendarSyncState `json:"sync"`
}

type SyncRunSummary struct {
	ID         int32      `json:"id"`
	CalendarID *int32     `json:"calendar_id"`
	Trigger    string     `json:"trigger"`
	Kind       string     `json:"kind"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Pages      int32      `json:"pages"`
	Upserted   int32      `json:"upserted"`
	Deleted    int32      `json:"deleted"`
	Error      *string    `json:"error"`
}

type AuthURLResponse struct {
	URL string `json:"url"`
}

type SyncResult struct {
	Calendars int      `json:"calendars"`
	Upserted  int      `json:"upserted"`
	Deleted   int      `json:"deleted"`
	Errors    []string `json:"errors"`
}

// --- HTTP requests -------------------------------------------------------------------

type AttendeeInput struct {
	Email    string `json:"email"`
	Optional bool   `json:"optional,omitempty"`
}

type RemindersInput struct {
	UseDefault bool               `json:"use_default"`
	Overrides  []ReminderOverride `json:"overrides,omitempty"`
}

/*
CreateEventRequest creates an event in Google and mirrors the answer.

StartsAt/EndsAt are RFC3339 instants, or YYYY-MM-DD when AllDay is set — the same split
Google draws between start.dateTime and start.date. EndsAt for an all-day event is
exclusive, so a single-day event ends on the following day.
*/
type CreateEventRequest struct {
	CalendarID   int32           `json:"calendar_id"`
	Summary      string          `json:"summary"`
	Description  string          `json:"description"`
	Location     string          `json:"location"`
	AllDay       bool            `json:"all_day"`
	StartsAt     string          `json:"starts_at"`
	EndsAt       string          `json:"ends_at"`
	TimeZone     string          `json:"time_zone"`
	Recurrence   []string        `json:"recurrence"`
	Attendees    []AttendeeInput `json:"attendees"`
	Reminders    *RemindersInput `json:"reminders"`
	Transparency string          `json:"transparency"`
	Visibility   string          `json:"visibility"`
	ColorID      string          `json:"color_id"`
	// SendUpdates is passed through to Google: all, externalOnly or none (default none, so
	// editing your own calendar does not mail people by accident).
	SendUpdates string `json:"send_updates"`
}

/*
UpdateEventRequest patches an event. Every field is a pointer: absent means "leave alone",
which is what makes this a patch rather than a replace.

Scope decides what a change to a recurring series touches:
  - "instance" (default when the reference names an occurrence): only that occurrence.
  - "following": that occurrence and everything after it, by ending the original series just
    before it and creating a new one.
  - "all": the whole series.
*/
type UpdateEventRequest struct {
	Summary      *string          `json:"summary"`
	Description  *string          `json:"description"`
	Location     *string          `json:"location"`
	AllDay       *bool            `json:"all_day"`
	StartsAt     *string          `json:"starts_at"`
	EndsAt       *string          `json:"ends_at"`
	TimeZone     *string          `json:"time_zone"`
	Recurrence   *[]string        `json:"recurrence"`
	Attendees    *[]AttendeeInput `json:"attendees"`
	Reminders    *RemindersInput  `json:"reminders"`
	Transparency *string          `json:"transparency"`
	Visibility   *string          `json:"visibility"`
	ColorID      *string          `json:"color_id"`
	Scope        string           `json:"scope"`
	SendUpdates  string           `json:"send_updates"`
}

type MoveEventRequest struct {
	CalendarID  int32  `json:"calendar_id"`
	SendUpdates string `json:"send_updates"`
}

// MoveResult reports what a move actually did. Google can only move an event between
// calendars of the same account; across accounts the event is recreated, so its id changes
// and the client must not keep using the old one.
type MoveResult struct {
	Event     Event `json:"event"`
	Recreated bool  `json:"recreated"`
}

type UpdateAccountRequest struct {
	Label *string `json:"label"`
	Color *string `json:"color"`
}

type UpdateCalendarRequest struct {
	SyncEnabled   *bool   `json:"sync_enabled"`
	Visible       *bool   `json:"visible"`
	ColorOverride *string `json:"color_override"`
}

type EventsQuery struct {
	From        time.Time
	To          time.Time
	CalendarIDs []int32
	AccountIDs  []int32
	// VisibleOnly restricts to calendars the user has not hidden; a client that manages its
	// own filtering asks for everything.
	VisibleOnly bool
}
