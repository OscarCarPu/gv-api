package assistant

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service ServiceInterface
	loc     *time.Location
}

func NewHandler(s ServiceInterface, loc *time.Location) *Handler {
	if loc == nil {
		loc = time.UTC
	}
	return &Handler{service: s, loc: loc}
}

// RegisterRoutes mounts the assistant endpoints. Requires full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/assistant/suggest", h.Suggest)
	r.Post("/assistant/execute", h.Execute)
	r.Get("/assistant/usage", h.Usage)
}

// Suggest -> POST /assistant/suggest
func (h *Handler) Suggest(w http.ResponseWriter, r *http.Request) {
	var req SuggestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		response.Error(w, http.StatusBadRequest, "text is required")
		return
	}
	out, err := h.service.Suggest(r.Context(), req)
	if err != nil {
		h.writeErr(w, r, err, "Failed to build suggestion")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

// Execute -> POST /assistant/execute
func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		response.Error(w, http.StatusBadRequest, "token is required")
		return
	}
	out, err := h.service.Execute(r.Context(), req)
	if err != nil {
		h.writeErr(w, r, err, "Failed to execute suggestion")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

// Usage -> GET /assistant/usage?month=YYYY-MM
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	month := time.Now().In(h.loc)
	if s := r.URL.Query().Get("month"); s != "" {
		parsed, err := time.ParseInLocation("2006-01", s, h.loc)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "month must be YYYY-MM")
			return
		}
		month = parsed
	}
	out, err := h.service.MonthlyUsage(r.Context(), month)
	if err != nil {
		response.InternalError(w, r, err, "Failed to load usage")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

// writeErr maps assistant sentinel errors to appropriate status codes; unknown
// errors become a 500.
func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error, msg string) {
	switch {
	case errors.Is(err, ErrBadToken):
		response.Error(w, http.StatusBadRequest, "invalid or expired token")
	case errors.Is(err, ErrUnsafeSQL):
		response.Error(w, http.StatusBadRequest, "the proposed query is not a safe read")
	case errors.Is(err, ErrReadTooLarge):
		response.Error(w, http.StatusBadRequest, "the result is too large")
	case errors.Is(err, ErrUnsupportedAction):
		response.Error(w, http.StatusBadRequest, "unsupported action")
	case errors.Is(err, ErrInvalidAction):
		response.Error(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrActionFailed):
		response.Error(w, http.StatusUnprocessableEntity, "No se pudo completar la acción (puede haber dependencias o datos que lo impiden).")
	case errors.Is(err, ErrProvider):
		response.Error(w, http.StatusBadGateway, "the assistant is unavailable right now")
	default:
		response.InternalError(w, r, err, msg)
	}
}
