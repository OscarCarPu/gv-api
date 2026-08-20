package uptime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	Overview(ctx context.Context) (Overview, error)
	Windows(ctx context.Context, q WindowsQuery) (WindowsReport, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts the uptime endpoints. Semiprivate: reading how long the lab has
// been up gives nothing away, and it sits next to lights in the same section.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/domotics/uptime", h.Overview)
	r.Get("/domotics/uptime/windows", h.Windows)
}

// Overview -> GET /domotics/uptime
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context())
	if err != nil {
		h.writeError(w, r, err, "Failed to read uptime")
		return
	}
	response.JSON(w, http.StatusOK, overview)
}

// Windows -> GET /domotics/uptime/windows?device=&from=&to=&limit=
func (h *Handler) Windows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var query WindowsQuery
	if raw := q.Get("device"); raw != "" {
		device, err := ParseDevice(raw)
		if err != nil {
			response.Error(w, http.StatusBadRequest, fmt.Sprintf("device must be one of %s", deviceList()))
			return
		}
		query.Device = &device
	}
	from, err := parseTimeParam(q.Get("from"), "from")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseTimeParam(q.Get("to"), "to")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	query.From, query.To = from, to

	if raw := q.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			response.Error(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		query.Limit = limit
	}

	report, err := h.service.Windows(r.Context(), query)
	if err != nil {
		if errors.Is(err, ErrInvalidRange) {
			response.Error(w, http.StatusBadRequest, "from must be before to")
			return
		}
		h.writeError(w, r, err, "Failed to read uptime windows")
		return
	}
	response.JSON(w, http.StatusOK, report)
}

// writeError maps the one failure that is a deployment state rather than a bug: with no
// pipeline database wired up there is nothing to read, and that is not a 500.
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error, message string) {
	if errors.Is(err, ErrNotConfigured) {
		response.Error(w, http.StatusServiceUnavailable, ErrNotConfigured.Error())
		return
	}
	response.InternalError(w, r, err, message)
}

// parseTimeParam accepts RFC 3339 or a plain date; an empty value leaves the default to
// the service. A bare date is read as UTC midnight, which is what the pipeline's own
// timestamps are in.
func parseTimeParam(raw, name string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid %s: must be RFC 3339 or YYYY-MM-DD", name)
}

func deviceList() string {
	out := ""
	for i, d := range Devices {
		if i > 0 {
			out += ", "
		}
		out += string(d)
	}
	return out
}
