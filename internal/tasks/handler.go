package tasks

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
	CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectResponse, error)
	CreateTask(ctx context.Context, req CreateTaskRequest) (TaskResponse, error)
	CreateTodo(ctx context.Context, req CreateTodoRequest) (TodoResponse, error)
	CreateTimeEntry(ctx context.Context, req CreateTimeEntryRequest) (TimeEntryResponse, error)
	UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error)
	UpdateTask(ctx context.Context, req UpdateTaskRequest) (TaskResponse, error)
	UpdateTodo(ctx context.Context, req UpdateTodoRequest) (TodoResponse, error)
	UpdateTimeEntry(ctx context.Context, req UpdateTimeEntryRequest) (TimeEntryResponse, error)
	GetRootProjects(ctx context.Context) ([]ProjectResponse, error)
	GetActiveTree(ctx context.Context) ([]ActiveTreeNode, error)
	GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func parseIDParam(r *http.Request, entity string) (int32, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id", entity)
	}
	return int32(id), nil
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

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	task, err := h.service.CreateTask(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create task")
		return
	}

	response.JSON(w, http.StatusCreated, task)
}

func (h *Handler) CreateTodo(w http.ResponseWriter, r *http.Request) {
	var req CreateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if req.TaskID == 0 {
		response.Error(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if req.Name == "" {
		response.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	todo, err := h.service.CreateTodo(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create todo")
		return
	}

	response.JSON(w, http.StatusCreated, todo)
}

func (h *Handler) CreateTimeEntry(w http.ResponseWriter, r *http.Request) {
	var req CreateTimeEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}

	if req.TaskID == 0 {
		response.Error(w, http.StatusBadRequest, "task_id is required")
		return
	}

	if req.StartedAt.IsZero() {
		response.Error(w, http.StatusBadRequest, "started_at is required")
		return
	}

	entry, err := h.service.CreateTimeEntry(r.Context(), req)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to create time entry")
		return
	}

	response.JSON(w, http.StatusCreated, entry)
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "project")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	project, err := h.service.UpdateProject(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "project not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to update project")
		return
	}

	response.JSON(w, http.StatusOK, project)
}

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "task")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	task, err := h.service.UpdateTask(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to update task")
		return
	}

	response.JSON(w, http.StatusOK, task)
}

func (h *Handler) UpdateTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "todo")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateTodoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	todo, err := h.service.UpdateTodo(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "todo not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to update todo")
		return
	}

	response.JSON(w, http.StatusOK, todo)
}

func (h *Handler) UpdateTimeEntry(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "time entry")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	var req UpdateTimeEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id

	entry, err := h.service.UpdateTimeEntry(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "time entry not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to update time entry")
		return
	}

	response.JSON(w, http.StatusOK, entry)
}

func (h *Handler) GetActiveTree(w http.ResponseWriter, r *http.Request) {
	tree, err := h.service.GetActiveTree(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get active tree")
		return
	}

	response.JSON(w, http.StatusOK, tree)
}

func (h *Handler) GetProjectChildren(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "project")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.GetProjectChildren(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "project not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get project children")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) GetTaskTimeEntries(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "task")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.GetTaskTimeEntries(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get task time entries")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) GetRootProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.GetRootProjects(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get projects")
		return
	}

	response.JSON(w, http.StatusOK, projects)
}

