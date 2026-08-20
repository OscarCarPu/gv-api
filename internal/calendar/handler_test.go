package calendar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gv-api/internal/calendar"
	"gv-api/internal/calendar/mocks"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Handler tests cover HTTP concerns only: parsing, validation of the request shape, and the
// mapping from the domain's errors to status codes. The behaviour behind them is tested
// against the service.

func newReq(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

func withParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func errMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body["error"]
}

func TestHandler_ListEvents_RequiresARange(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	rec := httptest.NewRecorder()
	calendar.NewHandler(svc).ListEvents(rec, newReq(http.MethodGet, "/calendar/events", ""))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, errMessage(t, rec), "from is required")
}

func TestHandler_ListEvents_ParsesTheQuery(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().ListEvents(mock.Anything, mock.MatchedBy(func(q calendar.EventsQuery) bool {
		return q.From.Equal(time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)) &&
			q.To.Equal(time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)) &&
			len(q.CalendarIDs) == 2 && q.CalendarIDs[0] == 3 && q.CalendarIDs[1] == 5 &&
			len(q.AccountIDs) == 1 && q.AccountIDs[0] == 1 && q.VisibleOnly
	})).Return([]calendar.Event{{InstanceID: "1"}}, nil)

	rec := httptest.NewRecorder()
	// A plain date is accepted so a client asking for a week does not have to invent a zone.
	calendar.NewHandler(svc).ListEvents(rec, newReq(http.MethodGet,
		"/calendar/events?from=2026-08-20&to=2026-08-27&calendar_ids=3,5&account_ids=1&visible_only=true", ""))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestHandler_ListEvents_RejectsBadIDLists(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	rec := httptest.NewRecorder()
	calendar.NewHandler(svc).ListEvents(rec, newReq(http.MethodGet,
		"/calendar/events?from=2026-08-20&to=2026-08-27&calendar_ids=3,abc", ""))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, errMessage(t, rec), "calendar_ids")
}

func TestHandler_ErrorMapping(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"missing", calendar.ErrNotFound, http.StatusNotFound, "not found"},
		{"no credentials", calendar.ErrNotConfigured, http.StatusServiceUnavailable, "not configured"},
		{"dead grant", calendar.ErrNeedsReauth, http.StatusConflict, "reconnected"},
		{"etag conflict", calendar.ErrConflict, http.StatusConflict, "refetch and retry"},
		{"read-only", calendar.ErrReadOnly, http.StatusForbidden, "read-only"},
		{"bad range", calendar.ErrInvalidRange, http.StatusBadRequest, "invalid time range"},
		{"bad scope", calendar.ErrInvalidScope, http.StatusBadRequest, "invalid scope"},
		{"google said no", calendar.ErrUpstream, http.StatusBadGateway, "google rejected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := mocks.NewMockServiceInterface(t)
			svc.EXPECT().GetEvent(mock.Anything, "7").Return(calendar.Event{}, tc.err)
			rec := httptest.NewRecorder()
			calendar.NewHandler(svc).GetEvent(rec, withParam(newReq(http.MethodGet, "/", ""), "ref", "7"))
			assert.Equal(t, tc.status, rec.Code)
			assert.Contains(t, errMessage(t, rec), tc.message)
		})
	}
}

func TestHandler_GetEvent_PassesAnInstanceReferenceThrough(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	ref := "12@2026-08-20T07:00:00Z"
	svc.EXPECT().GetEvent(mock.Anything, ref).Return(calendar.Event{InstanceID: ref}, nil)

	rec := httptest.NewRecorder()
	calendar.NewHandler(svc).GetEvent(rec, withParam(newReq(http.MethodGet, "/", ""), "ref", ref))
	assert.Equal(t, http.StatusOK, rec.Code)

	var out calendar.Event
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, ref, out.InstanceID)
}

func TestHandler_CreateEvent_Validation(t *testing.T) {
	t.Run("bad json", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).CreateEvent(rec, newReq(http.MethodPost, "/", "{"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("no calendar", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).CreateEvent(rec, newReq(http.MethodPost, "/", `{"summary":"x"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, errMessage(t, rec), "calendar_id")
	})

	t.Run("no summary", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).CreateEvent(rec, newReq(http.MethodPost, "/",
			`{"calendar_id":1,"summary":"   "}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, errMessage(t, rec), "summary")
	})

	t.Run("created", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().CreateEvent(mock.Anything, mock.Anything).Return(calendar.Event{InstanceID: "9"}, nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).CreateEvent(rec, newReq(http.MethodPost, "/",
			`{"calendar_id":1,"summary":"Dentist","starts_at":"2026-08-20T17:00:00Z"}`))
		assert.Equal(t, http.StatusCreated, rec.Code)
	})
}

func TestHandler_DeleteEvent_ForwardsScopeAndSendUpdates(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().DeleteEvent(mock.Anything, "12@2026-08-20T07:00:00Z", "instance", "all").Return(nil)

	rec := httptest.NewRecorder()
	req := withParam(newReq(http.MethodDelete, "/?scope=instance&send_updates=all", ""),
		"ref", "12@2026-08-20T07:00:00Z")
	calendar.NewHandler(svc).DeleteEvent(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestHandler_MoveEvent_RequiresADestination(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	rec := httptest.NewRecorder()
	calendar.NewHandler(svc).MoveEvent(rec, withParam(newReq(http.MethodPost, "/", `{}`), "ref", "7"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, errMessage(t, rec), "calendar_id")
}

func TestHandler_Sync_AllOrOne(t *testing.T) {
	t.Run("everything", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().SyncAll(mock.Anything, "manual").Return(calendar.SyncResult{Calendars: 2}, nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).Sync(rec, newReq(http.MethodPost, "/calendar/sync", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("one calendar", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().SyncCalendar(mock.Anything, int32(4), "manual").Return(calendar.SyncResult{Calendars: 1}, nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).Sync(rec, newReq(http.MethodPost, "/calendar/sync?calendar_id=4", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("bad id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).Sync(rec, newReq(http.MethodPost, "/calendar/sync?calendar_id=0", ""))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// --- the two public endpoints --------------------------------------------------------

func TestHandler_OAuthCallback_RedirectsOnSuccessAndOnFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().HandleCallback(mock.Anything, "the-code", "the-state").
			Return("https://app.example/calendar?connected=me@example.com", nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).OAuthCallback(rec,
			newReq(http.MethodGet, "/calendar/google/callback?code=the-code&state=the-state", ""))
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, "https://app.example/calendar?connected=me@example.com", rec.Header().Get("Location"))
	})

	t.Run("declined at google", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		// The person is looking at a browser tab; a JSON error there is a dead end.
		calendar.NewHandler(svc).OAuthCallback(rec,
			newReq(http.MethodGet, "/calendar/google/callback?error=access_denied", ""))
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "error=access_denied")
	})

	t.Run("state rejected", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().HandleCallback(mock.Anything, "c", "bad").Return("", calendar.ErrInvalidState)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).OAuthCallback(rec,
			newReq(http.MethodGet, "/calendar/google/callback?code=c&state=bad", ""))
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "error=")
	})
}

func TestHandler_Webhook(t *testing.T) {
	newWebhookReq := func(channel, state, token string) *http.Request {
		req := newReq(http.MethodPost, "/calendar/google/webhook", "")
		req.Header.Set("X-Goog-Channel-ID", channel)
		req.Header.Set("X-Goog-Resource-State", state)
		req.Header.Set("X-Goog-Channel-Token", token)
		return req
	}

	t.Run("accepted", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().HandleWebhook(mock.Anything, "chan-1", "exists", "secret").Return(nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).Webhook(rec, newWebhookReq("chan-1", "exists", "secret"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("unknown channel still answers 200", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().HandleWebhook(mock.Anything, "old", "exists", "secret").Return(calendar.ErrNotFound)
		rec := httptest.NewRecorder()
		// Anything but a 200 makes google retry and eventually drop the channel; a
		// notification for a channel we replaced is not worth that.
		calendar.NewHandler(svc).Webhook(rec, newWebhookReq("old", "exists", "secret"))
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("bad token", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().HandleWebhook(mock.Anything, "chan-1", "exists", "guess").
			Return(calendar.ErrInvalidState)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).Webhook(rec, newWebhookReq("chan-1", "exists", "guess"))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// --- SSE -----------------------------------------------------------------------------

func TestHandler_Stream_SendsEventsAndClosesWithTheClient(t *testing.T) {
	messages := make(chan calendar.StreamMessage, 4)
	released := make(chan struct{})

	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().Subscribe().Return(messages, func() { close(released) })

	ctx, cancel := context.WithCancel(context.Background())
	req := newReq(http.MethodGet, "/calendar/stream", "").WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		calendar.NewHandler(svc).Stream(rec, req)
		close(done)
	}()

	messages <- calendar.StreamMessage{Type: "calendar.changed", CalendarID: 3}
	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "calendar.changed")
	}, time.Second, 5*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the stream should end when the client disconnects")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("the subscription must be released, or every reconnect leaks one")
	}

	body := rec.Body.String()
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	assert.Contains(t, body, ": connected")
	assert.Contains(t, body, "event: calendar.changed")
	assert.Contains(t, body, `"calendar_id":3`)
}

func TestHandler_Accounts(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().ListAccounts(mock.Anything).
			Return([]calendar.Account{{ID: 1, Email: "me@example.com", Status: "connected"}}, nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).ListAccounts(rec, newReq(http.MethodGet, "/", ""))
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "me@example.com")
	})

	t.Run("auth url when unconfigured", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().AuthURL(mock.Anything).Return(calendar.AuthURLResponse{}, calendar.ErrNotConfigured)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).AuthURL(rec, newReq(http.MethodPost, "/", ""))
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("disconnect", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().DeleteAccount(mock.Anything, int32(2)).Return(nil)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).DeleteAccount(rec, withParam(newReq(http.MethodDelete, "/", ""), "id", "2"))
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("bad id", func(t *testing.T) {
		svc := mocks.NewMockServiceInterface(t)
		rec := httptest.NewRecorder()
		calendar.NewHandler(svc).DeleteAccount(rec, withParam(newReq(http.MethodDelete, "/", ""), "id", "x"))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandler_UpdateCalendar(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().UpdateCalendar(mock.Anything, int32(3), mock.MatchedBy(func(req calendar.UpdateCalendarRequest) bool {
		return req.SyncEnabled != nil && !*req.SyncEnabled && req.Visible == nil
	})).Return(calendar.Calendar{ID: 3}, nil)

	rec := httptest.NewRecorder()
	req := withParam(newReq(http.MethodPatch, "/", `{"sync_enabled":false}`), "id", "3")
	calendar.NewHandler(svc).UpdateCalendar(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_RoutesAreMounted(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	h := calendar.NewHandler(svc)
	private := chi.NewRouter()
	h.RegisterRoutes(private)
	public := chi.NewRouter()
	h.RegisterPublicRoutes(public)

	// A route that answers 405 or 404 here would be a silently missing endpoint.
	for _, tc := range []struct {
		router chi.Router
		method string
		target string
	}{
		{private, http.MethodGet, "/calendar/accounts"},
		{private, http.MethodPost, "/calendar/accounts/auth-url"},
		{private, http.MethodPatch, "/calendar/accounts/1"},
		{private, http.MethodDelete, "/calendar/accounts/1"},
		{private, http.MethodPost, "/calendar/accounts/1/resync"},
		{private, http.MethodGet, "/calendar/calendars"},
		{private, http.MethodPatch, "/calendar/calendars/1"},
		{private, http.MethodGet, "/calendar/events"},
		{private, http.MethodPost, "/calendar/events"},
		{private, http.MethodGet, "/calendar/events/1"},
		{private, http.MethodPatch, "/calendar/events/1"},
		{private, http.MethodDelete, "/calendar/events/1"},
		{private, http.MethodPost, "/calendar/events/1/move"},
		{private, http.MethodPost, "/calendar/sync"},
		{private, http.MethodGet, "/calendar/sync/status"},
		{private, http.MethodGet, "/calendar/stream"},
		{public, http.MethodGet, "/calendar/google/callback"},
		{public, http.MethodPost, "/calendar/google/webhook"},
	} {
		match := tc.router.Match(chi.NewRouteContext(), tc.method, tc.target)
		assert.True(t, match, "%s %s is not routed", tc.method, tc.target)
	}
}
