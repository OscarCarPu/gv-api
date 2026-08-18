package habits

import (
	"context"
	"encoding/json"
	"errors"
	"gv-api/internal/history"
	"net/http"

	"gv-api/internal/httputil"
	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error)
	LogHabit(ctx context.Context, req LogUpsertRequest) error
	CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error)
	UpdateHabit(ctx context.Context, req UpdateHabitRequest) (CreateHabitResponse, error)
	DeleteHabit(ctx context.Context, id int32) error
	GetHistory(ctx context.Context, habitID int32, frequency, startAt, endAt string) (history.Response, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts all habit endpoints. Requires full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/habits", h.GetDaily)
	r.Post("/habits", h.CreateHabit)
	r.Put("/habits/{id}", h.UpdateHabit)
	r.Delete("/habits/{id}", h.DeleteHabit)
	r.Post("/habits/log", h.UpsertLog)
	r.Get("/habits/{id}/history", h.GetHistory)
}

// validateHabitFields covers the checks shared by CreateHabit and UpdateHabit.
// frequency may be nil (create allows omitting it); returns "" when valid.
func validateHabitFields(name string, frequency *string, targetMin, targetMax *float32) string {
	if name == "" {
		return "name is required"
	}
	if len(name) > 40 {
		return "name must be at most 40 characters"
	}
	if frequency != nil {
		valid := map[string]bool{"daily": true, "weekly": true, "monthly": true}
		if !valid[*frequency] {
			return "frequency must be daily, weekly, or monthly"
		}
	}
	if targetMin != nil && *targetMin < 0 {
		return "target_min must be >= 0"
	}
	if targetMax != nil && *targetMax < 0 {
		return "target_max must be >= 0"
	}
	if targetMin != nil && targetMax != nil && *targetMin > *targetMax {
		return "target_min must be <= target_max"
	}
	return ""
}

// GetDaily -> GET /habits?date=2023-10-27
func (h *Handler) GetDaily(w http.ResponseWriter, r *http.Request) {
	dateParam := r.URL.Query().Get("date")

	habits, err := h.service.GetDailyView(r.Context(), dateParam)
	if err != nil {
		response.InternalError(w, r, err, "Failed to get daily habits")
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
		response.InternalError(w, r, err, "Failed to log habit")
		return
	}

	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteHabit -> DELETE /habits/{id}
func (h *Handler) DeleteHabit(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "habit")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.DeleteHabit(r.Context(), id); err != nil {
		response.InternalError(w, r, err, "Failed to delete habit")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetHistory -> GET /habits/{id}/history
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "habit")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
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

	history, err := h.service.GetHistory(r.Context(), id, frequency, startAt, endAt)
	if err != nil {
		response.InternalError(w, r, err, "Failed to get habit history")
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

	if msg := validateHabitFields(req.Name, req.Frequency, req.TargetMin, req.TargetMax); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	habit, err := h.service.CreateHabit(r.Context(), req)
	if err != nil {
		response.InternalError(w, r, err, "Failed to create habit")
		return
	}

	response.JSON(w, http.StatusCreated, habit)
}

// UpdateHabit -> PUT /habits/{id}
func (h *Handler) UpdateHabit(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "habit")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateHabitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	if msg := validateHabitFields(req.Name, &req.Frequency, req.TargetMin, req.TargetMax); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}

	habit, err := h.service.UpdateHabit(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "habit not found")
			return
		}
		response.InternalError(w, r, err, "Failed to update habit")
		return
	}

	response.JSON(w, http.StatusOK, habit)
}
