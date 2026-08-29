package capacity

import (
	"context"
	"net/http"
	"time"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

const maxRangeSpan = 90 * 24 * time.Hour

type ServiceInterface interface {
	FreeBusyRange(ctx context.Context, from, to time.Time) ([]DayFreeBusy, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts the capacity endpoint. Requires full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/capacity/free-busy", h.GetFreeBusy)
}

func (h *Handler) GetFreeBusy(w http.ResponseWriter, r *http.Request) {
	from, err := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid or missing from")
		return
	}
	to, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid or missing to")
		return
	}
	if !to.After(from) {
		response.Error(w, http.StatusBadRequest, "to must be after from")
		return
	}
	if to.Sub(from) > maxRangeSpan {
		response.Error(w, http.StatusBadRequest, "range is longer than 90 days")
		return
	}

	days, err := h.service.FreeBusyRange(r.Context(), from, to)
	if err != nil {
		response.InternalError(w, r, err, "Failed to get free/busy range")
		return
	}
	response.JSON(w, http.StatusOK, FreeBusyRangeResponse{
		From: from.Format("2006-01-02"),
		To:   to.Format("2006-01-02"),
		Days: days,
	})
}
