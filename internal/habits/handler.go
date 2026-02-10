package habits

import (
	"context"
	"encoding/json"
	"net/http"

	"gv-api/internal/response"
)

type ServiceInterface interface {
	GetDailyView(ctx context.Context, dateStr string) ([]HabitWithLog, error)
	LogHabit(ctx context.Context, req LogUpsertRequest) error
	CreateHabit(ctx context.Context, req CreateHabitRequest) (CreateHabitResponse, error)
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

	habit, err := h.service.CreateHabit(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create habit")
		return
	}

	response.JSON(w, http.StatusCreated, habit)
}
