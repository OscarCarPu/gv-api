package habits

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error)
	LogHabit(ctx context.Context, req LogUpsertRequest) error
	CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error)
	DeleteHabit(ctx context.Context, id int32) error
	GetHistory(ctx context.Context, habitID int32, frequency, startAt, endAt string) (HistoryResponse, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// GetDaily -> GET /habits?date=2023-10-27
func (h *Handler) GetDaily(w http.ResponseWriter, r *http.Request) {
	dateParam := r.URL.Query().Get("date")

	habits, err := h.service.GetDailyView(r.Context(), dateParam)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	response.JSON(w, http.StatusOK, habits)
}

// UpsertLog -> POST /habits/log
func (h *Handler) UpsertLog(w http.ResponseWriter, r *http.Request) {
	var req LogUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if err := h.service.LogHabit(r.Context(), req); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to log")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteHabit -> DELETE /habits/{id}
func (h *Handler) DeleteHabit(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("invalid %s id", "habit"))
		return
	}

	if err := h.service.DeleteHabit(r.Context(), int32(id)); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete habit")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetHistory -> GET /habits/{id}/history
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid habit id")
		return
	}

	frequency := r.URL.Query().Get("frequency")
	if frequency != "" {
		valid := map[string]bool{"daily": true, "weekly": true, "monthly": true}
		if !valid[frequency] {
			response.Error(w, http.StatusBadRequest, "frequency must be daily, weekly, or monthly")
			return
		}
	}

	startAt := r.URL.Query().Get("start_at")
	endAt := r.URL.Query().Get("end_at")

	history, err := h.service.GetHistory(r.Context(), int32(id), frequency, startAt, endAt)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get history")
		return
	}

	response.JSON(w, http.StatusOK, history)
}

// CreateHabit -> POST /habits
func (h *Handler) CreateHabit(w http.ResponseWriter, r *http.Request) {
	var req CreateHabitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	if len(req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}

	if req.Frequency != nil {
		valid := map[string]bool{"daily": true, "weekly": true, "monthly": true}
		if !valid[*req.Frequency] {
			response.Error(w, http.StatusBadRequest, "frequency must be daily, weekly, or monthly")
			return
		}
	}

	if req.TargetMin != nil && *req.TargetMin < 0 {
		response.Error(w, http.StatusBadRequest, "target_min must be >= 0")
		return
	}

	if req.TargetMax != nil && *req.TargetMax < 0 {
		response.Error(w, http.StatusBadRequest, "target_max must be >= 0")
		return
	}

	if req.TargetMin != nil && req.TargetMax != nil && *req.TargetMin > *req.TargetMax {
		response.Error(w, http.StatusBadRequest, "target_min must be <= target_max")
		return
	}

	habit, err := h.service.CreateHabit(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create habit")
		return
	}

	response.JSON(w, http.StatusCreated, habit)
}
