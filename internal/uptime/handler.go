package uptime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gv-api/internal/core"

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

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/domotics/uptime", h.Overview)
	r.Get("/domotics/uptime/windows", h.Windows)
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context())
	if err != nil {
		core.InternalError(w, r, err, "Failed to read uptime")
		return
	}
	core.JSON(w, http.StatusOK, overview)
}

func (h *Handler) Windows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var query WindowsQuery
	from, err := parseTimeParam(q.Get("from"), "from")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseTimeParam(q.Get("to"), "to")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	query.From, query.To = from, to

	if raw := q.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			core.Error(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		query.Limit = limit
	}

	report, err := h.service.Windows(r.Context(), query)
	if err != nil {
		if errors.Is(err, ErrInvalidRange) {
			core.Error(w, http.StatusBadRequest, "from must be before to")
			return
		}
		core.InternalError(w, r, err, "Failed to read uptime windows")
		return
	}
	core.JSON(w, http.StatusOK, report)
}

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
