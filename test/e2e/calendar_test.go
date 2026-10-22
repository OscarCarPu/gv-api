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

type CalendarEvent struct {
	ID        int32     `json:"id"`
	Name      string    `json:"name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

func truncateCalendarTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, getDBURL(t))
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "TRUNCATE calendar_events CASCADE"); err != nil {
		t.Fatalf("failed to truncate calendar tables: %v", err)
	}
}

func (c *APIClient) calendarJSON(t *testing.T, method, path, body string, out any) int {
	t.Helper()
	var raw []byte
	if body != "" {
		raw = []byte(body)
	}
	resp := c.do(t, method, path, raw)
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s %s: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

func TestE2E_Calendar_RequiresAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	client := NewAPIClient(t)

	resp := client.do(t, http.MethodGet, "/calendar/events?from=2026-08-20&to=2026-08-21", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET without a token: got %d, want 401", resp.StatusCode)
	}
}

func TestE2E_Calendar_EventLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	client := authenticate(t)

	var created CalendarEvent
	if code := client.calendarJSON(t, http.MethodPost, "/calendar/events",
		`{"name":"Dentist","start_time":"2026-08-20T09:00:00Z","end_time":"2026-08-20T10:00:00Z"}`, &created); code != http.StatusCreated {
		t.Fatalf("create: got %d", code)
	}
	path := "/calendar/events/" + strconv.Itoa(int(created.ID))

	var listed []CalendarEvent
	client.calendarJSON(t, http.MethodGet, "/calendar/events?from=2026-08-20&to=2026-08-21", "", &listed)
	if len(listed) != 1 || listed[0].Name != "Dentist" {
		t.Fatalf("list: got %+v", listed)
	}
	client.calendarJSON(t, http.MethodGet, "/calendar/events?from=2026-08-21&to=2026-08-22", "", &listed)
	if len(listed) != 0 {
		t.Fatalf("list outside the range: got %+v", listed)
	}

	var updated CalendarEvent
	if code := client.calendarJSON(t, http.MethodPatch, path,
		`{"name":"Dentist (moved)","end_time":"2026-08-20T11:00:00Z"}`, &updated); code != http.StatusOK {
		t.Fatalf("update: got %d", code)
	}
	if updated.Name != "Dentist (moved)" || !updated.EndTime.Equal(time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC)) {
		t.Errorf("update: got %+v", updated)
	}

	if code := client.calendarJSON(t, http.MethodDelete, path, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete: got %d", code)
	}
	if code := client.calendarJSON(t, http.MethodGet, path, "", nil); code != http.StatusNotFound {
		t.Errorf("get after delete: got %d, want 404", code)
	}
}

func TestE2E_Calendar_Validation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	truncateCalendarTables(t)
	client := authenticate(t)

	for name, tc := range map[string]struct{ method, path, body string }{
		"no name":          {http.MethodPost, "/calendar/events", `{"start_time":"2026-08-20T09:00:00Z","end_time":"2026-08-20T10:00:00Z"}`},
		"end before start": {http.MethodPost, "/calendar/events", `{"name":"x","start_time":"2026-08-20T10:00:00Z","end_time":"2026-08-20T09:00:00Z"}`},
		"no range":         {http.MethodGet, "/calendar/events", ""},
		"inverted range":   {http.MethodGet, "/calendar/events?from=2026-08-21&to=2026-08-20", ""},
	} {
		if code := client.calendarJSON(t, tc.method, tc.path, tc.body, nil); code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, code)
		}
	}
	if code := client.calendarJSON(t, http.MethodPatch, "/calendar/events/999", `{"name":"x"}`, nil); code != http.StatusNotFound {
		t.Errorf("patch missing: got %d, want 404", code)
	}
}
