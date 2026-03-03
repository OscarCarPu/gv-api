package tasks

import (
	"context"
	"encoding/json"
	"net/http"

	"gv-api/internal/response"
)

type ServiceInterface interface {
	CreateProject(ctx context.Context, req CreateProjectRequest) (CreateProjectResponse, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	project, err := h.service.CreateProject(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create project")
		return
	}

	response.JSON(w, http.StatusCreated, project)
}
