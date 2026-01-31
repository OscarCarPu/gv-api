package habits

import (
	"encoding/json"
	"net/http"

	"gv-api/internal/response"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

// GetDaily -> GET /habits?date=2023-10-27
func (h *Handler) GetDaily(w http.ResponseWriter, r *http.Request) {
	dateParam := r.URL.Query().Get("date")

	domainData, err := h.service.GetDailyView(r.Context(), dateParam)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var habits []HabitWithLog
	for _, item := range domainData {
		habits = append(habits, HabitWithLog{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			LogValue:    item.LogValue,
		})
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
