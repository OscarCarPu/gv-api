package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gv-api/internal/core"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	ListEvents(ctx context.Context, from, to time.Time) ([]Event, error)
	GetEvent(ctx context.Context, id int32) (Event, error)
	CreateEvent(ctx context.Context, req CreateEventRequest) (Event, error)
	UpdateEvent(ctx context.Context, id int32, req UpdateEventRequest) (Event, error)
	DeleteEvent(ctx context.Context, id int32) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/calendar/events", h.ListEvents)
	r.Post("/calendar/events", h.CreateEvent)
	r.Get("/calendar/events/{id}", h.GetEvent)
	r.Patch("/calendar/events/{id}", h.UpdateEvent)
	r.Delete("/calendar/events/{id}", h.DeleteEvent)
}

// parseQueryTime accepts an instant or a plain date.
func parseQueryTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse(time.DateOnly, raw)
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	from, err := parseQueryTime(r.URL.Query().Get("from"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "from is required (RFC3339 or YYYY-MM-DD)")
		return
	}
	to, err := parseQueryTime(r.URL.Query().Get("to"))
	if err != nil {
		core.Error(w, http.StatusBadRequest, "to is required (RFC3339 or YYYY-MM-DD)")
		return
	}
	events, err := h.service.ListEvents(r.Context(), from, to)
	if err != nil {
		h.fail(w, r, err, "Failed to list events")
		return
	}
	core.JSON(w, http.StatusOK, events)
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "event")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	ev, err := h.service.GetEvent(r.Context(), id)
	if err != nil {
		h.fail(w, r, err, "Failed to get event")
		return
	}
	core.JSON(w, http.StatusOK, ev)
}

func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	ev, err := h.service.CreateEvent(r.Context(), req)
	if err != nil {
		h.fail(w, r, err, "Failed to create event")
		return
	}
	core.JSON(w, http.StatusCreated, ev)
}

func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "event")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	ev, err := h.service.UpdateEvent(r.Context(), id, req)
	if err != nil {
		h.fail(w, r, err, "Failed to update event")
		return
	}
	core.JSON(w, http.StatusOK, ev)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "event")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteEvent(r.Context(), id); err != nil {
		h.fail(w, r, err, "Failed to delete event")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, message string) {
	switch {
	case errors.Is(err, ErrNotFound):
		core.Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrInvalid):
		core.Error(w, http.StatusBadRequest, err.Error())
	default:
		core.InternalError(w, r, err, message)
	}
}
