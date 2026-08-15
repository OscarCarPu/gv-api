package lights

import (
	"encoding/json"
	"errors"
	"net/http"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts the lights endpoints. Semiprivate auth, matching the rest of the
// Domotics section — it is house control, not personal data.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/domotics/lights", h.List)
	r.Get("/domotics/lights/state", h.States)
	r.Get("/domotics/lights/{id}", h.State)
	r.Post("/domotics/lights/{id}", h.Send)
}

// List -> GET /domotics/lights
func (h *Handler) List(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, h.service.List())
}

// States -> GET /domotics/lights/state
//
// `?force=1` skips the read cache. An unreachable bulb comes back inside a 200 with
// online:false, so one dead bulb never fails the request.
func (h *Handler) States(w http.ResponseWriter, r *http.Request) {
	states := h.service.States(r.Context(), r.URL.Query().Get("force") == "1")
	response.JSON(w, http.StatusOK, StatesResponse{States: states})
}

// State -> GET /domotics/lights/{id}
func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	state, err := h.service.State(r.Context(), id, r.URL.Query().Get("force") == "1")
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "light not found")
			return
		}
		response.InternalError(w, r, err, "Failed to read light")
		return
	}
	response.JSON(w, http.StatusOK, state)
}

// Send -> POST /domotics/lights/{id}
func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cmd Command
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}

	state, err := h.service.Send(r.Context(), id, cmd)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(w, http.StatusNotFound, "light not found")
		case errors.Is(err, ErrInvalidCommand):
			response.Error(w, http.StatusBadRequest, err.Error())
		default:
			response.InternalError(w, r, err, "Failed to send command")
		}
		return
	}
	response.JSON(w, http.StatusOK, state)
}
