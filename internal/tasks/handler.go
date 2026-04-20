package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gv-api/internal/history"
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
	ListProjectsFast(ctx context.Context) ([]ProjectFastResponse, error)
	ListTasksFast(ctx context.Context) ([]TaskFastResponse, error)
	GetRootProjects(ctx context.Context) ([]ProjectResponse, error)
	GetActiveTree(ctx context.Context, minPriority *int32) ([]ActiveTreeNode, error)
	GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error)
	GetTask(ctx context.Context, id int32) (TaskFullResponse, error)
	GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error)
	GetTaskTimeEntries(ctx context.Context, taskID int32) (TaskTimeEntriesResponse, error)
	GetTasksByDueDate(ctx context.Context, minPriority *int32) ([]TaskByDueDateResponse, error)
	DeleteProject(ctx context.Context, id int32) error
	DeleteTask(ctx context.Context, id int32) error
	DeleteTodo(ctx context.Context, id int32) error
	DeleteTimeEntry(ctx context.Context, id int32) error
	GetActiveTimeEntry(ctx context.Context) (ActiveTimeEntryResponse, error)
	GetTimeEntrySummary(ctx context.Context) (TimeEntrySummaryResponse, error)
	GetTimeEntryHistory(ctx context.Context, frequency, startAt, endAt string) (history.Response, error)
	GetTimeEntriesByDateRange(ctx context.Context, startTime, endTime string) ([]TimeEntryWithTaskResponse, error)
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

	if len(req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
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

	if len(req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}

	if req.TaskType != nil {
		switch *req.TaskType {
		case "standard", "continuous", "recurring":
		default:
			response.Error(w, http.StatusBadRequest, "task_type must be standard, continuous, or recurring")
			return
		}
	}

	isRecurring := req.TaskType != nil && *req.TaskType == "recurring"
	if isRecurring {
		if req.Recurrence == nil {
			response.Error(w, http.StatusBadRequest, "recurrence is required when task_type is recurring")
			return
		}
		if *req.Recurrence <= 0 {
			response.Error(w, http.StatusBadRequest, "recurrence must be a positive number of days")
			return
		}
	} else if req.Recurrence != nil {
		response.Error(w, http.StatusBadRequest, "recurrence is only valid when task_type is recurring")
		return
	}

	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > 5) {
		response.Error(w, http.StatusBadRequest, "priority must be between 1 and 5")
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

	if len(req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
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
		if errors.Is(err, ErrActiveTimeEntryExists) {
			response.Error(w, http.StatusConflict, "an active time entry already exists")
			return
		}
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

	if req.Name != nil && len(*req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}

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

	if req.Name != nil && len(*req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}

	if req.TaskType != nil {
		switch *req.TaskType {
		case "standard", "continuous", "recurring":
		default:
			response.Error(w, http.StatusBadRequest, "task_type must be standard, continuous, or recurring")
			return
		}
		if *req.TaskType == "recurring" && req.Recurrence == nil {
			response.Error(w, http.StatusBadRequest, "recurrence is required when task_type is recurring")
			return
		}
		if *req.TaskType != "recurring" && req.Recurrence != nil {
			response.Error(w, http.StatusBadRequest, "recurrence is only valid when task_type is recurring")
			return
		}
	}

	if req.Recurrence != nil && *req.Recurrence <= 0 {
		response.Error(w, http.StatusBadRequest, "recurrence must be a positive number of days")
		return
	}

	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > 5) {
		response.Error(w, http.StatusBadRequest, "priority must be between 1 and 5")
		return
	}

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

	if req.Name != nil && len(*req.Name) > 40 {
		response.Error(w, http.StatusBadRequest, "name must be at most 40 characters")
		return
	}

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
	minPriority, err := parseMinPriority(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	tree, err := h.service.GetActiveTree(r.Context(), minPriority)
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

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "project")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.GetProject(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "project not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get project")
		return
	}

	response.JSON(w, http.StatusOK, result)
}

func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "task")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.service.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "task not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get task")
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

func (h *Handler) GetTasksByDueDate(w http.ResponseWriter, r *http.Request) {
	minPriority, err := parseMinPriority(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	tasks, err := h.service.GetTasksByDueDate(r.Context(), minPriority)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get tasks by due date")
		return
	}
	response.JSON(w, http.StatusOK, tasks)
}

func parseMinPriority(r *http.Request) (*int32, error) {
	s := r.URL.Query().Get("min_priority")
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 5 {
		return nil, fmt.Errorf("min_priority must be between 1 and 5")
	}
	v := int32(n)
	return &v, nil
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "project")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.DeleteProject(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "task")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.DeleteTask(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteTodo(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "todo")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.DeleteTodo(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete todo")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteTimeEntry(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "time entry")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.service.DeleteTimeEntry(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete time entry")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetActiveTimeEntry(w http.ResponseWriter, r *http.Request) {
	entry, err := h.service.GetActiveTimeEntry(r.Context())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "no active time entry")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get active time entry")
		return
	}

	response.JSON(w, http.StatusOK, entry)
}

func (h *Handler) GetTimeEntrySummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetTimeEntrySummary(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get time entry summary")
		return
	}

	response.JSON(w, http.StatusOK, summary)
}

func (h *Handler) GetTimeEntryHistory(w http.ResponseWriter, r *http.Request) {
	frequency := r.URL.Query().Get("frequency")
	if frequency == "" {
		response.Error(w, http.StatusBadRequest, "frequency is required")
		return
	}
	valid := map[string]bool{"daily": true, "weekly": true, "monthly": true}
	if !valid[frequency] {
		response.Error(w, http.StatusBadRequest, "frequency must be daily, weekly, or monthly")
		return
	}

	startAt := r.URL.Query().Get("start_at")
	endAt := r.URL.Query().Get("end_at")

	history, err := h.service.GetTimeEntryHistory(r.Context(), frequency, startAt, endAt)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get time entry history")
		return
	}

	response.JSON(w, http.StatusOK, history)
}

func (h *Handler) GetTimeEntriesByDateRange(w http.ResponseWriter, r *http.Request) {
	startTime := r.URL.Query().Get("start_time")
	if startTime == "" {
		response.Error(w, http.StatusBadRequest, "start_time is required")
		return
	}

	if _, err := time.Parse("2006-01-02", startTime); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid start_time format, expected YYYY-MM-DD")
		return
	}

	endTime := r.URL.Query().Get("end_time")
	if endTime != "" {
		if _, err := time.Parse("2006-01-02", endTime); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid end_time format, expected YYYY-MM-DD")
			return
		}
	}

	entries, err := h.service.GetTimeEntriesByDateRange(r.Context(), startTime, endTime)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get time entries")
		return
	}

	response.JSON(w, http.StatusOK, entries)
}

func (h *Handler) ListTasksFast(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.service.ListTasksFast(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list tasks")
		return
	}

	response.JSON(w, http.StatusOK, tasks)
}

func (h *Handler) ListProjectsFast(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.ListProjectsFast(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list projects")
		return
	}

	response.JSON(w, http.StatusOK, projects)
}

func (h *Handler) GetRootProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.GetRootProjects(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get projects")
		return
	}

	response.JSON(w, http.StatusOK, projects)
}

