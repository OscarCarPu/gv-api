package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gv-api/internal/httputil"
	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

// ServiceInterface is what the HTTP layer needs. Declared here, by the consumer, so the
// handler can be tested against a mock without a database or a Google.
type ServiceInterface interface {
	Configured() bool
	AuthURL(ctx context.Context) (AuthURLResponse, error)
	HandleCallback(ctx context.Context, code, state string) (string, error)

	ListAccounts(ctx context.Context) ([]Account, error)
	UpdateAccount(ctx context.Context, id int32, req UpdateAccountRequest) (Account, error)
	DeleteAccount(ctx context.Context, id int32) error
	ResyncAccount(ctx context.Context, id int32) (SyncResult, error)

	ListCalendars(ctx context.Context) ([]Calendar, error)
	UpdateCalendar(ctx context.Context, id int32, req UpdateCalendarRequest) (Calendar, error)

	ListEvents(ctx context.Context, q EventsQuery) ([]Event, error)
	GetEvent(ctx context.Context, ref string) (Event, error)
	CreateEvent(ctx context.Context, req CreateEventRequest) (Event, error)
	UpdateEvent(ctx context.Context, ref string, req UpdateEventRequest) (Event, error)
	DeleteEvent(ctx context.Context, ref, scope, sendUpdates string) error
	MoveEvent(ctx context.Context, ref string, req MoveEventRequest) (MoveResult, error)

	SyncAll(ctx context.Context, trigger string) (SyncResult, error)
	SyncCalendar(ctx context.Context, calendarID int32, trigger string) (SyncResult, error)
	SyncStatus(ctx context.Context) (SyncStatus, error)
	HandleWebhook(ctx context.Context, channelID, resourceState, channelToken string) error
	Subscribe() (<-chan StreamMessage, func())
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts the endpoints that require full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/calendar/accounts", h.ListAccounts)
	r.Post("/calendar/accounts/auth-url", h.AuthURL)
	r.Patch("/calendar/accounts/{id}", h.UpdateAccount)
	r.Delete("/calendar/accounts/{id}", h.DeleteAccount)
	r.Post("/calendar/accounts/{id}/resync", h.ResyncAccount)

	r.Get("/calendar/calendars", h.ListCalendars)
	r.Patch("/calendar/calendars/{id}", h.UpdateCalendar)

	r.Get("/calendar/events", h.ListEvents)
	r.Post("/calendar/events", h.CreateEvent)
	r.Get("/calendar/events/{ref}", h.GetEvent)
	r.Patch("/calendar/events/{ref}", h.UpdateEvent)
	r.Delete("/calendar/events/{ref}", h.DeleteEvent)
	r.Post("/calendar/events/{ref}/move", h.MoveEvent)

	r.Post("/calendar/sync", h.Sync)
	r.Get("/calendar/sync/status", h.SyncStatus)
	r.Get("/calendar/stream", h.Stream)
}

/*
RegisterPublicRoutes mounts the two endpoints that cannot be behind the bearer middleware:

  - the OAuth callback, which the browser reaches by Google's redirect to the API host, where
    there is no session cookie to present. It is guarded by the signed state parameter.
  - the push webhook, which Google POSTs to with no credentials at all. It is guarded by the
    per-channel token it echoes back.
*/
func (h *Handler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/calendar/google/callback", h.OAuthCallback)
	r.Post("/calendar/google/webhook", h.Webhook)
}

// --- accounts ------------------------------------------------------------------------

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.service.ListAccounts(r.Context())
	if err != nil {
		h.fail(w, r, err, "Failed to list accounts")
		return
	}
	response.JSON(w, http.StatusOK, accounts)
}

func (h *Handler) AuthURL(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.AuthURL(r.Context())
	if err != nil {
		h.fail(w, r, err, "Failed to build the consent url")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	acc, err := h.service.UpdateAccount(r.Context(), id, req)
	if err != nil {
		h.fail(w, r, err, "Failed to update account")
		return
	}
	response.JSON(w, http.StatusOK, acc)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteAccount(r.Context(), id); err != nil {
		h.fail(w, r, err, "Failed to disconnect account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ResyncAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.service.ResyncAccount(r.Context(), id)
	if err != nil {
		h.fail(w, r, err, "Failed to resync account")
		return
	}
	response.JSON(w, http.StatusOK, res)
}

// --- OAuth ---------------------------------------------------------------------------

/*
OAuthCallback finishes the consent flow and sends the browser back to the web app.

Errors redirect too, with the reason in the query string, rather than rendering JSON: the
person is looking at a browser tab they were sent to by Google, and a bare error object there
is a dead end.
*/
func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if reason := query.Get("error"); reason != "" {
		slog.WarnContext(r.Context(), "calendar: consent was declined", "reason", reason)
		http.Redirect(w, r, "/calendar?error="+reason, http.StatusFound)
		return
	}
	redirect, err := h.service.HandleCallback(r.Context(), query.Get("code"), query.Get("state"))
	if err != nil {
		slog.ErrorContext(r.Context(), "calendar: oauth callback failed", "error", err)
		http.Redirect(w, r, "/calendar?error="+urlSafe(err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func urlSafe(s string) string {
	replacer := strings.NewReplacer(" ", "+", "&", "%26", "?", "%3F", "#", "%23", "\n", " ")
	return replacer.Replace(s)
}

// Webhook accepts Google's push notification. It always answers 200 unless the request is
// unrecognisable: Google retries anything else and eventually drops the channel, and a
// notification we cannot place is not worth losing a channel over.
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	channelID := r.Header.Get("X-Goog-Channel-ID")
	state := r.Header.Get("X-Goog-Resource-State")
	token := r.Header.Get("X-Goog-Channel-Token")

	err := h.service.HandleWebhook(r.Context(), channelID, state, token)
	switch {
	case err == nil, errors.Is(err, ErrNotFound):
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, ErrInvalidState):
		response.Error(w, http.StatusUnauthorized, "invalid channel token")
	default:
		response.InternalError(w, r, err, "Failed to handle notification")
	}
}

// --- calendars -----------------------------------------------------------------------

func (h *Handler) ListCalendars(w http.ResponseWriter, r *http.Request) {
	cals, err := h.service.ListCalendars(r.Context())
	if err != nil {
		h.fail(w, r, err, "Failed to list calendars")
		return
	}
	response.JSON(w, http.StatusOK, cals)
}

func (h *Handler) UpdateCalendar(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "calendar")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateCalendarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	cal, err := h.service.UpdateCalendar(r.Context(), id, req)
	if err != nil {
		h.fail(w, r, err, "Failed to update calendar")
		return
	}
	response.JSON(w, http.StatusOK, cal)
}

// --- events --------------------------------------------------------------------------

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := parseQueryTime(q.Get("from"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "from is required (RFC3339 or YYYY-MM-DD)")
		return
	}
	to, err := parseQueryTime(q.Get("to"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "to is required (RFC3339 or YYYY-MM-DD)")
		return
	}
	calendarIDs, err := parseIDList(q.Get("calendar_ids"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid calendar_ids")
		return
	}
	accountIDs, err := parseIDList(q.Get("account_ids"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid account_ids")
		return
	}

	events, err := h.service.ListEvents(r.Context(), EventsQuery{
		From:        from,
		To:          to,
		CalendarIDs: calendarIDs,
		AccountIDs:  accountIDs,
		VisibleOnly: q.Get("visible_only") == "true",
	})
	if err != nil {
		h.fail(w, r, err, "Failed to list events")
		return
	}
	response.JSON(w, http.StatusOK, events)
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	ev, err := h.service.GetEvent(r.Context(), chi.URLParam(r, "ref"))
	if err != nil {
		h.fail(w, r, err, "Failed to get event")
		return
	}
	response.JSON(w, http.StatusOK, ev)
}

func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.CalendarID <= 0 {
		response.Error(w, http.StatusBadRequest, "calendar_id is required")
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		response.Error(w, http.StatusBadRequest, "summary is required")
		return
	}
	ev, err := h.service.CreateEvent(r.Context(), req)
	if err != nil {
		h.fail(w, r, err, "Failed to create event")
		return
	}
	response.JSON(w, http.StatusCreated, ev)
}

func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	var req UpdateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	ev, err := h.service.UpdateEvent(r.Context(), chi.URLParam(r, "ref"), req)
	if err != nil {
		h.fail(w, r, err, "Failed to update event")
		return
	}
	response.JSON(w, http.StatusOK, ev)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := h.service.DeleteEvent(r.Context(), chi.URLParam(r, "ref"),
		q.Get("scope"), q.Get("send_updates")); err != nil {
		h.fail(w, r, err, "Failed to delete event")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) MoveEvent(w http.ResponseWriter, r *http.Request) {
	var req MoveEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.CalendarID <= 0 {
		response.Error(w, http.StatusBadRequest, "calendar_id is required")
		return
	}
	res, err := h.service.MoveEvent(r.Context(), chi.URLParam(r, "ref"), req)
	if err != nil {
		h.fail(w, r, err, "Failed to move event")
		return
	}
	response.JSON(w, http.StatusOK, res)
}

// --- sync ----------------------------------------------------------------------------

func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	if raw := r.URL.Query().Get("calendar_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || id <= 0 {
			response.Error(w, http.StatusBadRequest, "invalid calendar_id")
			return
		}
		res, err := h.service.SyncCalendar(r.Context(), int32(id), "manual")
		if err != nil {
			h.fail(w, r, err, "Failed to sync calendar")
			return
		}
		response.JSON(w, http.StatusOK, res)
		return
	}
	res, err := h.service.SyncAll(r.Context(), "manual")
	if err != nil {
		h.fail(w, r, err, "Failed to sync")
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *Handler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.SyncStatus(r.Context())
	if err != nil {
		h.fail(w, r, err, "Failed to read sync status")
		return
	}
	response.JSON(w, http.StatusOK, status)
}

/*
Stream is the server-sent events endpoint: it tells a connected client that something moved so
it can refetch, instead of the client polling this API on a timer.

Two details are load-bearing. The server's write timeout has to be cleared for this one
connection, or the stream would be cut after 30 seconds. And the keep-alive comment every 25
seconds is what stops the Cloudflare tunnel in front of this API from dropping an idle
connection.
*/
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Error(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(r.Context(), "calendar: clearing the stream write deadline", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tells any buffering proxy not to hold the stream back.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	messages, unsubscribe := h.service.Subscribe()
	defer unsubscribe()

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, open := <-messages:
			if !open {
				return
			}
			payload, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Type, payload)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// --- helpers -------------------------------------------------------------------------

// parseQueryTime accepts an instant or a plain date, so a client asking for "2026-08-20" does
// not have to invent a time zone to ask about a day.
func parseQueryTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse(dateLayout, raw)
}

func parseIDList(raw string) ([]int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]int32, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 32)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid id %q", p)
		}
		out = append(out, int32(id))
	}
	return out, nil
}

/*
fail maps the domain's sentinel errors onto status codes.

The two 409s are deliberate and distinct in their message: one says the event moved under you
(refetch and retry), the other says the account's grant is gone (a person has to reconnect it).
Both are the client's problem to act on, neither is a server fault, and collapsing them into
500 would hide the only two failures a user can actually resolve.
*/
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, message string) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrNotConfigured):
		response.Error(w, http.StatusServiceUnavailable, "google calendar is not configured on this server")
	case errors.Is(err, ErrNeedsReauth):
		response.Error(w, http.StatusConflict, "account needs to be reconnected")
	case errors.Is(err, ErrConflict):
		response.Error(w, http.StatusConflict, "event changed in google, refetch and retry")
	case errors.Is(err, ErrReadOnly):
		response.Error(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidRange), errors.Is(err, ErrInvalidScope), errors.Is(err, ErrInvalidState):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUpstream):
		slog.ErrorContext(r.Context(), message, "error", err)
		response.Error(w, http.StatusBadGateway, "google rejected the request")
	default:
		response.InternalError(w, r, err, message)
	}
}
