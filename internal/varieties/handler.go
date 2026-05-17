package varieties

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	Get(ctx context.Context, id int32) (Variety, error)
	List(ctx context.Context) ([]Variety, error)
	Create(ctx context.Context, req CreateVarietyRequest) (Variety, error)
	Update(ctx context.Context, req UpdateVarietyRequest) (Variety, error)
	Delete(ctx context.Context, id int32) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func parseID(r *http.Request) (int32, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid variety id")
	}
	return int32(id), nil
}

func validateScores(scent, flavor, power, quality float32) error {
	for name, v := range map[string]float32{"scent": scent, "flavor": flavor, "power": power, "quality": quality} {
		if v < 0 || v > 10 {
			return fmt.Errorf("%s must be between 0 and 10", name)
		}
	}
	return nil
}

// Get -> GET /varieties/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	v, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "variety not found")
			return
		}
		response.InternalError(w, r, err, "Failed to get variety")
		return
	}

	response.JSON(w, http.StatusOK, v)
}

// List -> GET /varieties
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	vs, err := h.service.List(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to list varieties")
		return
	}

	response.JSON(w, http.StatusOK, vs)
}

// Create -> POST /varieties
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateVarietyRequest
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
	req.Judge = strings.TrimSpace(req.Judge)
	if req.Judge == "" {
		response.Error(w, http.StatusBadRequest, "judge is required")
		return
	}
	if len(req.Judge) > 40 {
		response.Error(w, http.StatusBadRequest, "judge must be at most 40 characters")
		return
	}
	if err := validateScores(req.Scent, req.Flavor, req.Power, req.Quality); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	v, err := h.service.Create(r.Context(), req)
	if err != nil {
		response.InternalError(w, r, err, "Failed to create variety")
		return
	}

	response.JSON(w, http.StatusCreated, v)
}

// Update -> PUT /varieties/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateVarietyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}
	req.Judge = strings.TrimSpace(req.Judge)
	if req.Judge == "" {
		response.Error(w, http.StatusBadRequest, "judge is required")
		return
	}
	if len(req.Judge) > 40 {
		response.Error(w, http.StatusBadRequest, "judge must be at most 40 characters")
		return
	}
	if err := validateScores(req.Scent, req.Flavor, req.Power, req.Quality); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	v, err := h.service.Update(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "variety not found")
			return
		}
		response.InternalError(w, r, err, "Failed to update variety")
		return
	}

	response.JSON(w, http.StatusOK, v)
}

// Delete -> DELETE /varieties/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.InternalError(w, r, err, "Failed to delete variety")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
