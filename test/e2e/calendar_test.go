package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Calendar DTOs as a client sees them.
type CalendarEvent struct {
	InstanceID       string     `json:"instance_id"`
	EventID          int32      `json:"event_id"`
	CalendarID       int32      `json:"calendar_id"`
	AccountEmail     string     `json:"account_email"`
	CalendarName     string     `json:"calendar_name"`
	Color            string     `json:"color"`
	Summary          string     `json:"summary"`
	AllDay           bool       `json:"all_day"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           time.Time  `json:"ends_at"`
	TimeZone         string     `json:"time_zone"`
	Recurring        bool       `json:"recurring"`
	IsException      bool       `json:"is_exception"`
	OriginalStartsAt *time.Time `json:"original_starts_at"`
	Editable         bool       `json:"editable"`
}

type CalendarSyncStatus struct {
	Configured     bool   `json:"configured"`
	WebhooksActive bool   `json:"webhooks_active"`
	PollInterval   string `json:"poll_interval"`
	Accounts       []struct {
		Email  string `json:"email"`
		Status string `json:"status"`
	} `json:"accounts"`
	Calendars []struct {
		CalendarID int32 `json:"calendar_id"`
		Events     int64 `json:"events"`
	} `json:"calendars"`
}

func truncateCalendarTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, getDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		"TRUNCATE calendar_sync_runs, calendar_events, calendars, google_accounts CASCADE"); err != nil {
		t.Fatalf("failed to truncate calendar tables: %v", err)
	}
}

/*
seedCalendar puts an account, a calendar and a recurring series straight into the database.

The e2e stack has no Google credentials, which is exactly the deployment state before the
first account is connected. Seeding the mirror directly exercises what the API does with it —
the range query, the expansion, the overrides — through the real router and the real database.
*/
func seedCalendar(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, getDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(ctx)

	var accountID, calendarID, masterID int32
	if err := conn.QueryRow(ctx, `
		INSERT INTO google_accounts (email, refresh_token, scopes)
		VALUES ('e2e@example.com', '\x00'::bytea, 'calendar')
		RETURNING id`).Scan(&accountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO calendars (account_id, google_calendar_id, summary, time_zone, background_color,
		                       access_role, is_primary)
		VALUES ($1, 'e2e@example.com', 'Personal', 'Europe/Madrid', '#3366cc', 'owner', TRUE)
		RETURNING id`, accountID).Scan(&calendarID); err != nil {
		t.Fatalf("seed calendar: %v", err)
	}

	// A one-off, an all-day event, and a daily series with one occurrence moved and one
	// cancelled: enough to prove the expansion runs for real.
	if _, err := conn.Exec(ctx, `
		INSERT INTO calendar_events (calendar_id, google_event_id, etag, summary, all_day,
		                             starts_at, ends_at, start_tz, end_tz)
		VALUES ($1, 'oneoff', '"1"', 'Dentist', FALSE,
		        '2026-08-20T15:00:00Z', '2026-08-20T16:00:00Z', 'Europe/Madrid', 'Europe/Madrid'),
		       ($1, 'allday', '"1"', 'Trip', TRUE,
		        '2026-08-19T22:00:00Z', '2026-08-22T22:00:00Z', 'Europe/Madrid', 'Europe/Madrid')`,
		calendarID); err != nil {
		t.Fatalf("seed events: %v", err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO calendar_events (calendar_id, google_event_id, etag, summary, starts_at, ends_at,
		                             start_tz, end_tz, recurrence)
		VALUES ($1, 'series', '"1"', 'Standup', '2026-08-17T07:00:00Z', '2026-08-17T07:30:00Z',
		        'Europe/Madrid', 'Europe/Madrid', ARRAY['RRULE:FREQ=DAILY;COUNT=5'])
		RETURNING id`, calendarID).Scan(&masterID); err != nil {
		t.Fatalf("seed series: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO calendar_events (calendar_id, google_event_id, etag, summary, starts_at, ends_at,
		                             start_tz, end_tz, recurring_event_id, master_id, original_starts_at,
		                             status)
		VALUES ($1, 'series_moved', '"1"', 'Standup (late)', '2026-08-19T09:00:00Z', '2026-08-19T09:30:00Z',
		        'Europe/Madrid', 'Europe/Madrid', 'series', $2, '2026-08-19T07:00:00Z', 'confirmed'),
		       ($1, 'series_gone', '"1"', 'Standup', '2026-08-20T07:00:00Z', '2026-08-20T07:30:00Z',
		        'Europe/Madrid', 'Europe/Madrid', 'series', $2, '2026-08-20T07:00:00Z', 'cancelled')`,
		calendarID, masterID); err != nil {
		t.Fatalf("seed overrides: %v", err)
	}
}

func (c *APIClient) getJSON(t *testing.T, path string, out any) *http.Response {
	t.Helper()
	resp := c.do(t, http.MethodGet, path, nil)
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp
}

func TestE2E_Calendar_RequiresAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	client := NewAPIClient(t)

	for _, path := range []string{"/calendar/accounts", "/calendar/calendars",
		"/calendar/events?from=2026-08-20&to=2026-08-21", "/calendar/sync/status", "/calendar/stream"} {
		resp := client.do(t, http.MethodGet, path, nil)
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("GET %s without a token: got %d, want 401", path, resp.StatusCode)
			}
		}()
	}
}

func TestE2E_Calendar_PublicEndpointsNeedNoToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	client := NewAPIClient(t)

	// Google POSTs the webhook with no credentials at all; an unknown channel still gets a
	// 200, because anything else makes google retry and eventually drop the channel.
	req, err := http.NewRequest(http.MethodPost, getBaseURL(t)+"/calendar/google/webhook", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Goog-Channel-ID", "unknown-channel")
	req.Header.Set("X-Goog-Resource-State", "exists")
	req.Header.Set("X-Goog-Channel-Token", "whatever")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("webhook with an unknown channel: got %d, want 200", resp.StatusCode)
	}

	// The consent callback redirects a browser rather than answering with JSON.
	noRedirects := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err = noRedirects.Get(getBaseURL(t) + "/calendar/google/callback?error=access_denied")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("oauth callback: got %d, want 302", resp.StatusCode)
	}
	_ = client
}

func TestE2E_Calendar_WithoutCredentialsItSaysSo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	client := authenticate(t)

	// Anything that needs Google answers 503 rather than failing obscurely.
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/calendar/accounts/auth-url", ""},
		{http.MethodPost, "/calendar/sync", ""},
		{http.MethodPost, "/calendar/events", `{"calendar_id":1,"summary":"x","starts_at":"2026-08-20T09:00:00Z"}`},
	} {
		var body []byte
		if tc.body != "" {
			body = []byte(tc.body)
		}
		resp := client.do(t, tc.method, tc.path, body)
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("%s %s: got %d, want 503", tc.method, tc.path, resp.StatusCode)
			}
		}()
	}

	// Reads still work, and the status endpoint is what tells you why nothing is syncing.
	var status CalendarSyncStatus
	resp := client.getJSON(t, "/calendar/sync/status", &status)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync status: got %d, want 200", resp.StatusCode)
	}
	if status.Configured {
		t.Error("configured should be false with no google credentials")
	}
	if status.PollInterval == "" {
		t.Error("the poll interval is part of the answer: it is the staleness bound")
	}
}

func TestE2E_Calendar_ListEventsExpandsTheMirror(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	seedCalendar(t)
	client := authenticate(t)

	var events []CalendarEvent
	resp := client.getJSON(t,
		"/calendar/events?from=2026-08-17T00:00:00Z&to=2026-08-24T00:00:00Z", &events)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list events: got %d, want 200", resp.StatusCode)
	}

	byName := map[string]int{}
	for _, e := range events {
		byName[e.Summary]++
		if e.AccountEmail != "e2e@example.com" {
			t.Errorf("event %q is missing its source account", e.Summary)
		}
		if e.CalendarName != "Personal" || e.Color != "#3366cc" {
			t.Errorf("event %q is missing its calendar's name or colour", e.Summary)
		}
		if !e.Editable {
			t.Errorf("event %q should be editable: the calendar's role is owner", e.Summary)
		}
	}

	// 5 daily occurrences: one moved, one cancelled, so 3 plain plus 1 moved.
	if byName["Standup"] != 3 {
		t.Errorf("got %d plain occurrences of the series, want 3", byName["Standup"])
	}
	if byName["Standup (late)"] != 1 {
		t.Errorf("the moved occurrence should appear once, got %d", byName["Standup (late)"])
	}
	if byName["Dentist"] != 1 || byName["Trip"] != 1 {
		t.Errorf("the one-off and the all-day event should each appear once: %v", byName)
	}

	var previous time.Time
	for _, e := range events {
		if !previous.IsZero() && e.StartsAt.Before(previous) {
			t.Fatalf("events must come back in time order, got %s after %s", e.StartsAt, previous)
		}
		previous = e.StartsAt

		switch e.Summary {
		case "Standup (late)":
			if !e.IsException {
				t.Error("a moved occurrence is an exception")
			}
			if e.OriginalStartsAt == nil || !e.OriginalStartsAt.Equal(time.Date(2026, 8, 19, 7, 0, 0, 0, time.UTC)) {
				t.Errorf("the moved occurrence must keep its original slot, got %v", e.OriginalStartsAt)
			}
			if e.InstanceID == "" || e.InstanceID == "0" {
				t.Error("an occurrence needs an addressable instance id")
			}
		case "Standup":
			if !e.Recurring {
				t.Error("occurrences of a series are marked recurring")
			}
			if e.StartsAt.Equal(time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)) {
				t.Error("the cancelled occurrence must not come back")
			}
		case "Trip":
			if !e.AllDay {
				t.Error("the all-day flag is part of the contract")
			}
		}
	}

	// A single day, with the day boundaries taken from the query's own zone.
	events = nil
	resp2 := client.getJSON(t, "/calendar/events?from=2026-08-20T00:00:00Z&to=2026-08-21T00:00:00Z", &events)
	defer resp2.Body.Close()
	if len(events) == 0 {
		t.Fatal("the 20th has a dentist appointment and a trip running through it")
	}
	for _, e := range events {
		if e.Summary == "Standup" && e.StartsAt.Equal(time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)) {
			t.Error("the cancelled occurrence is still showing on its own day")
		}
	}
}

func TestE2E_Calendar_EventLookupAndValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	seedCalendar(t)
	client := authenticate(t)

	var events []CalendarEvent
	resp := client.getJSON(t, "/calendar/events?from=2026-08-17T00:00:00Z&to=2026-08-24T00:00:00Z", &events)
	resp.Body.Close()

	var occurrence CalendarEvent
	for _, e := range events {
		if e.Summary == "Standup" {
			occurrence = e
			break
		}
	}
	if occurrence.InstanceID == "" {
		t.Fatal("expected an occurrence of the series")
	}

	// An instance reference survives a round trip through the URL.
	var single CalendarEvent
	resp = client.getJSON(t, "/calendar/events/"+occurrence.InstanceID, &single)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get occurrence %s: got %d, want 200", occurrence.InstanceID, resp.StatusCode)
	}
	if !single.StartsAt.Equal(occurrence.StartsAt) {
		t.Errorf("got %s, want %s", single.StartsAt, occurrence.StartsAt)
	}

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/calendar/events", http.StatusBadRequest},
		{"/calendar/events?from=2026-08-20", http.StatusBadRequest},
		{"/calendar/events?from=2026-08-21&to=2026-08-20", http.StatusBadRequest},
		{"/calendar/events?from=2020-01-01&to=2030-01-01", http.StatusBadRequest},
		{"/calendar/events?from=yesterday&to=tomorrow", http.StatusBadRequest},
		{"/calendar/events/999999", http.StatusNotFound},
		{"/calendar/events/not-an-id", http.StatusNotFound},
	} {
		resp := client.do(t, http.MethodGet, tc.path, nil)
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("GET %s: got %d, want %d", tc.path, resp.StatusCode, tc.want)
			}
		}()
	}
}

func TestE2E_Calendar_CalendarsAndPreferences(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	seedCalendar(t)
	client := authenticate(t)

	var calendars []struct {
		ID          int32  `json:"id"`
		Summary     string `json:"summary"`
		Color       string `json:"color"`
		Writable    bool   `json:"writable"`
		Visible     bool   `json:"visible"`
		SyncEnabled bool   `json:"sync_enabled"`
	}
	resp := client.getJSON(t, "/calendar/calendars", &calendars)
	resp.Body.Close()
	if len(calendars) != 1 {
		t.Fatalf("got %d calendars, want 1", len(calendars))
	}
	if !calendars[0].Writable || !calendars[0].Visible {
		t.Error("an owned calendar starts writable and visible")
	}

	// A local preference: it changes here and never goes to Google.
	resp = client.do(t, http.MethodPatch,
		"/calendar/calendars/"+itoa(calendars[0].ID), []byte(`{"visible":false,"color_override":"#ff0000"}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch calendar: got %d, want 200", resp.StatusCode)
	}
	var updated struct {
		Visible bool   `json:"visible"`
		Color   string `json:"color"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if updated.Visible {
		t.Error("visible should be false")
	}
	if updated.Color != "#ff0000" {
		t.Errorf("got colour %q, want the override", updated.Color)
	}

	// Hidden calendars drop out of a visible-only query but not out of an unfiltered one.
	var events []CalendarEvent
	resp2 := client.getJSON(t,
		"/calendar/events?from=2026-08-17T00:00:00Z&to=2026-08-24T00:00:00Z&visible_only=true", &events)
	resp2.Body.Close()
	if len(events) != 0 {
		t.Errorf("got %d events from a hidden calendar, want 0", len(events))
	}
}

func TestE2E_Calendar_StreamAnswersAsAnEventStream(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	client := authenticate(t)

	req, err := http.NewRequest(http.MethodGet, getBaseURL(t)+"/calendar/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := (&http.Client{}).Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: got %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("got content type %q, want text/event-stream", got)
	}

	// The greeting arrives immediately, which is how a client knows the stream is live rather
	// than buffered somewhere upstream.
	buf := make([]byte, 16)
	n, err := resp.Body.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("expected an immediate greeting, got n=%d err=%v", n, err)
	}
	cancel()
}

func itoa(v int32) string { return strconv.FormatInt(int64(v), 10) }
