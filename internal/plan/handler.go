package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetToday(ctx context.Context) (PlanTodayResponse, error)
	Create(ctx context.Context, req CreatePlanBlockRequest) (PlanBlockResponse, error)
	Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error)
	Delete(ctx context.Context, id int32) error
	DeleteFuture(ctx context.Context) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func parseIDParam(r *http.Request) (int32, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid plan block id")
	}
	return int32(id), nil
}

func (h *Handler) GetToday(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetToday(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to get today's plan")
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreatePlanBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	block, err := h.service.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTimeRange),
			errors.Is(err, ErrLabelRequired),
			errors.Is(err, ErrLabelTooLong),
			errors.Is(err, ErrOverlap):
			response.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrTaskNotFound):
			response.Error(w, http.StatusBadRequest, "task not found")
		default:
			response.InternalError(w, r, err, "Failed to create plan block")
		}
		return
	}

	response.JSON(w, http.StatusCreated, block)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdatePlanBlockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	block, err := h.service.Update(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(w, http.StatusNotFound, "plan block not found")
		case errors.Is(err, ErrInvalidTimeRange),
			errors.Is(err, ErrLabelRequired),
			errors.Is(err, ErrLabelTooLong),
			errors.Is(err, ErrOverlap):
			response.Error(w, http.StatusBadRequest, err.Error())
		default:
			response.InternalError(w, r, err, "Failed to update plan block")
		}
		return
	}

	response.JSON(w, http.StatusOK, block)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		response.InternalError(w, r, err, "Failed to delete plan block")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteFuture(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteFuture(r.Context()); err != nil {
		response.InternalError(w, r, err, "Failed to delete future plan blocks")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
