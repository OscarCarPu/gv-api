package rutas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"gv-api/internal/core"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	List(ctx context.Context) ([]ConcelloMark, error)
	Get(ctx context.Context, id int32) (ConcelloMark, error)
	Create(ctx context.Context, req CreateMarkRequest) (ConcelloMark, error)
	Update(ctx context.Context, req UpdateMarkRequest) (ConcelloMark, error)
	Delete(ctx context.Context, id int32) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts all rutas endpoints under semiprivate auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/rutas/marks", h.List)
	r.Get("/rutas/marks/{id}", h.Get)
	r.Post("/rutas/marks", h.Create)
	r.Put("/rutas/marks/{id}", h.Update)
	r.Delete("/rutas/marks/{id}", h.Delete)
}

// List -> GET /rutas/marks
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	marks, err := h.service.List(r.Context())
	if err != nil {
		core.InternalError(w, r, err, "Failed to list marks")
		return
	}
	core.JSON(w, http.StatusOK, marks)
}

// Get -> GET /rutas/marks/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "mark")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	mark, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			core.Error(w, http.StatusNotFound, "mark not found")
			return
		}
		core.InternalError(w, r, err, "Failed to get mark")
		return
	}
	core.JSON(w, http.StatusOK, mark)
}

// Create -> POST /rutas/marks
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateMarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		core.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 200 {
		core.Error(w, http.StatusBadRequest, "name too long")
		return
	}
	if req.VisitedOn == "" {
		core.Error(w, http.StatusBadRequest, "visited_on is required")
		return
	}
	if _, err := parseDate(req.VisitedOn); err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	mark, err := h.service.Create(r.Context(), req)
	if err != nil {
		core.InternalError(w, r, err, "Failed to create mark")
		return
	}
	core.JSON(w, http.StatusCreated, mark)
}

// Update -> PUT /rutas/marks/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "mark")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateMarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.Error(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.ID = id
	if req.VisitedOn == "" {
		core.Error(w, http.StatusBadRequest, "visited_on is required")
		return
	}
	if _, err := parseDate(req.VisitedOn); err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	mark, err := h.service.Update(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			core.Error(w, http.StatusNotFound, "mark not found")
			return
		}
		core.InternalError(w, r, err, "Failed to update mark")
		return
	}
	core.JSON(w, http.StatusOK, mark)
}

// Delete -> DELETE /rutas/marks/{id}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := core.ParseIDParam(r, "mark")
	if err != nil {
		core.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		core.InternalError(w, r, err, "Failed to delete mark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
