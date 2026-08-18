package lights

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

// ServiceInterface is the seam the handler depends on, so it can be mocked in tests.
type ServiceInterface interface {
	List(ctx context.Context) ([]PublicLight, error)
	States(ctx context.Context, force bool) ([]State, error)
	State(ctx context.Context, id string, force bool) (State, error)
	Send(ctx context.Context, id string, cmd Command) (State, error)

	Create(ctx context.Context, req CreateLightRequest) (PublicLight, error)
	Update(ctx context.Context, id string, req UpdateLightRequest) (PublicLight, error)
	Delete(ctx context.Context, id string) error
	Discover(ctx context.Context, window time.Duration) ([]Discovered, error)
	Protocols() []ProtocolInfo
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// Scan window bounds. Long enough to find a bulb across a flat, short enough that nobody
// wonders whether the button worked.
const (
	defaultScanWindow = 8 * time.Second
	maxScanWindow     = 30 * time.Second
)

// RegisterRoutes mounts the lights endpoints under semiprivate auth: house
// control, not personal data.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/domotics/lights", h.List)
	r.Post("/domotics/lights", h.Create)
	r.Get("/domotics/lights/state", h.States)
	r.Get("/domotics/lights/discover", h.Discover)
	r.Get("/domotics/lights/protocols", h.Protocols)
	r.Get("/domotics/lights/{id}", h.State)
	r.Post("/domotics/lights/{id}", h.Send)
	r.Patch("/domotics/lights/{id}", h.Update)
	r.Delete("/domotics/lights/{id}", h.Delete)
}

// List -> GET /domotics/lights
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	lights, err := h.service.List(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to list lights")
		return
	}
	response.JSON(w, http.StatusOK, lights)
}

// States -> GET /domotics/lights/state
//
// `?force=1` skips the read cache. An unreachable bulb comes back inside a 200 with
// online:false, so one dead bulb never fails the request.
func (h *Handler) States(w http.ResponseWriter, r *http.Request) {
	states, err := h.service.States(r.Context(), r.URL.Query().Get("force") == "1")
	if err != nil {
		response.InternalError(w, r, err, "Failed to read lights")
		return
	}
	response.JSON(w, http.StatusOK, StatesResponse{States: states})
}

// State -> GET /domotics/lights/{id}
func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	state, err := h.service.State(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("force") == "1")
	if err != nil {
		h.fail(w, r, err, "Failed to read light")
		return
	}
	response.JSON(w, http.StatusOK, state)
}

// Send -> POST /domotics/lights/{id}
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	var cmd Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	state, err := h.service.Send(r.Context(), chi.URLParam(r, "id"), cmd)
	if err != nil {
		h.fail(w, r, err, "Failed to send command")
		return
	}
	response.JSON(w, http.StatusOK, state)
}

// Create -> POST /domotics/lights
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateLightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	light, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.fail(w, r, err, "Failed to add light")
		return
	}
	response.JSON(w, http.StatusCreated, light)
}

// Update -> PATCH /domotics/lights/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req UpdateLightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	light, err := h.service.Update(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		h.fail(w, r, err, "Failed to update light")
		return
	}
	response.JSON(w, http.StatusOK, light)
}

// Delete -> DELETE /domotics/lights/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		h.fail(w, r, err, "Failed to remove light")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Discover -> GET /domotics/lights/discover?seconds=8
//
// Holds the request open for the length of the scan: the answer does not exist until the
// radio has been listening for a while, and a start-then-poll job is more moving parts than
// a button pressed once in a while deserves.
func (h *Handler) Discover(w http.ResponseWriter, r *http.Request) {
	devices, err := h.service.Discover(r.Context(), scanWindow(r))
	if err != nil {
		// Unlike a bulb read, this one has nothing useful to degrade to: an empty list would
		// read as "no bulbs here" when the truth is "this host cannot look".
		response.Error(w, http.StatusServiceUnavailable, bleErrorMessage(err))
		return
	}
	response.JSON(w, http.StatusOK, DiscoveredResponse{Devices: devices})
}

// Protocols -> GET /domotics/lights/protocols
func (h *Handler) Protocols(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, h.service.Protocols())
}

func scanWindow(r *http.Request) time.Duration {
	seconds, err := strconv.ParseFloat(r.URL.Query().Get("seconds"), 64)
	if err != nil || seconds <= 0 {
		return defaultScanWindow
	}
	return min(time.Duration(seconds*float64(time.Second)), maxScanWindow)
}

// fail maps the domain's errors onto status codes. Anything unrecognised is a 500 with the
// detail in the log rather than in the response.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, message string) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "light not found")
	case errors.Is(err, ErrInvalidCommand):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrDuplicateAddress):
		response.Error(w, http.StatusConflict, ErrDuplicateAddress.Error())
	default:
		response.InternalError(w, r, err, message)
	}
}
