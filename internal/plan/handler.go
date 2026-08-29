package plan

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gv-api/internal/httputil"
	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetToday(ctx context.Context) (PlanTodayResponse, error)
	GetRange(ctx context.Context, from, to time.Time) (PlanRangeResponse, error)
	Create(ctx context.Context, req CreatePlanBlockRequest) (PlanBlockResponse, error)
	Update(ctx context.Context, req UpdatePlanBlockRequest) (PlanBlockResponse, error)
	Delete(ctx context.Context, id int32) error
	DeleteFuture(ctx context.Context) error

	ListCommitments(ctx context.Context) ([]RecurringCommitmentResponse, error)
	CreateCommitment(ctx context.Context, req CreateCommitmentRequest) (RecurringCommitmentResponse, error)
	UpdateCommitment(ctx context.Context, req UpdateCommitmentRequest) (RecurringCommitmentResponse, error)
	DeleteCommitment(ctx context.Context, id int32) error
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts all plan endpoints. Requires full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/plan/today", h.GetToday)
	r.Get("/plan/range", h.GetRange)
	r.Post("/plan/blocks", h.Create)
	r.Delete("/plan/blocks/future", h.DeleteFuture)
	r.Put("/plan/blocks/{id}", h.Update)
	r.Delete("/plan/blocks/{id}", h.Delete)

	r.Get("/plan/commitments", h.ListCommitments)
	r.Post("/plan/commitments", h.CreateCommitment)
	r.Put("/plan/commitments/{id}", h.UpdateCommitment)
	r.Delete("/plan/commitments/{id}", h.DeleteCommitment)
}

func (h *Handler) GetToday(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetToday(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to get today's plan")
		return
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *Handler) GetRange(w http.ResponseWriter, r *http.Request) {
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

	resp, err := h.service.GetRange(r.Context(), from, to)
	if err != nil {
		response.InternalError(w, r, err, "Failed to get plan range")
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
	id, err := httputil.ParseIDParam(r, "plan block")
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
	id, err := httputil.ParseIDParam(r, "plan block")
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

func (h *Handler) ListCommitments(w http.ResponseWriter, r *http.Request) {
	commitments, err := h.service.ListCommitments(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to list commitments")
		return
	}
	response.JSON(w, http.StatusOK, commitments)
}

func (h *Handler) CreateCommitment(w http.ResponseWriter, r *http.Request) {
	var req CreateCommitmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	commitment, err := h.service.CreateCommitment(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrLabelRequired), errors.Is(err, ErrLabelTooLong), errors.Is(err, ErrDaysOfWeekRequired):
			response.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrTaskNotFound):
			response.Error(w, http.StatusBadRequest, err.Error())
		default:
			response.InternalError(w, r, err, "Failed to create commitment")
		}
		return
	}
	response.JSON(w, http.StatusCreated, commitment)
}

func (h *Handler) UpdateCommitment(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "commitment")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateCommitmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	commitment, err := h.service.UpdateCommitment(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(w, http.StatusNotFound, "commitment not found")
		case errors.Is(err, ErrLabelRequired), errors.Is(err, ErrLabelTooLong), errors.Is(err, ErrDaysOfWeekRequired):
			response.Error(w, http.StatusBadRequest, err.Error())
		default:
			response.InternalError(w, r, err, "Failed to update commitment")
		}
		return
	}
	response.JSON(w, http.StatusOK, commitment)
}

func (h *Handler) DeleteCommitment(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "commitment")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteCommitment(r.Context(), id); err != nil {
		response.InternalError(w, r, err, "Failed to delete commitment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
