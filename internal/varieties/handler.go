package varieties

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gv-api/internal/httputil"
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

// RegisterRoutes mounts all variety endpoints. Unlike the other domains,
// these are mounted under the semiprivate auth group (semi or full token).
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/varieties", h.List)
	r.Get("/varieties/{id}", h.Get)
	r.Post("/varieties", h.Create)
	r.Put("/varieties/{id}", h.Update)
	r.Delete("/varieties/{id}", h.Delete)
}


func validateScores(scent, flavor, power, quality float32) error {
	for name, v := range map[string]float32{"scent": scent, "flavor": flavor, "power": power, "quality": quality} {
		if v < 0 || v > 10 {
			return fmt.Errorf("%s must be between 0 and 10", name)
		}
	}
	return nil
}

// validateVarietyFields covers the checks shared by Create and Update.
// Returns the trimmed judge and "" when valid, or the client-facing error message.
func validateVarietyFields(name, judge string, scent, flavor, power, quality float32) (string, string) {
	if name == "" {
		return "", "name is required"
	}
	if len(name) > 40 {
		return "", "name must be at most 40 characters"
	}
	judge = strings.TrimSpace(judge)
	if judge == "" {
		return "", "judge is required"
	}
	if len(judge) > 40 {
		return "", "judge must be at most 40 characters"
	}
	if err := validateScores(scent, flavor, power, quality); err != nil {
		return "", err.Error()
	}
	return judge, ""
}

// Get -> GET /varieties/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "variety")
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

	judge, msg := validateVarietyFields(req.Name, req.Judge, req.Scent, req.Flavor, req.Power, req.Quality)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Judge = judge

	v, err := h.service.Create(r.Context(), req)
	if err != nil {
		response.InternalError(w, r, err, "Failed to create variety")
		return
	}

	response.JSON(w, http.StatusCreated, v)
}

// Update -> PUT /varieties/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "variety")
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

	judge, msg := validateVarietyFields(req.Name, req.Judge, req.Scent, req.Flavor, req.Power, req.Quality)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Judge = judge

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
	id, err := httputil.ParseIDParam(r, "variety")
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
