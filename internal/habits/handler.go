package habits

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	service *Service
}

func NewHandler(s *Service) *Handler {
	return &Handler{service: s}
}

// GET /habits?date=2023-10-27
func (h *Handler) GetDaily(w http.ResponseWriter, r *http.Request) {
	dateParam := r.URL.Query().Get("date")

	domainData, err := h.service.GetDailyView(r.Context(), dateParam)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var response []HabitWithLog
	for _, item := range domainData {
		response = append(response, HabitWithLog{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			LogValue:    item.LogValue,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// POST /habits/log
func (h *Handler) UpsertLog(w http.ResponseWriter, r *http.Request) {
	var req LogUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid Body", http.StatusBadRequest)
		return
	}

	if err := h.service.LogHabit(r.Context(), req); err != nil {
		http.Error(w, "Failed to log", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
