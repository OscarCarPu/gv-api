package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"gv-api/internal/finance/txtype"
	"gv-api/internal/httputil"
	"gv-api/internal/response"
)

func validateCategory(name string, t txtype.Type, parentID *int32, selfID int32) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "name is required"
	}
	if len(name) > 40 {
		return "", "name must be at most 40 characters"
	}
	if !t.Valid() {
		return "", "type must be income, expense, or transfer"
	}
	if parentID != nil && selfID != 0 && *parentID == selfID {
		return "", "parent_id must not equal id"
	}
	return name, ""
}

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ListCategories(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to list categories")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := h.service.GetCategory(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "category not found")
			return
		}
		response.InternalError(w, r, err, "Failed to get category")
		return
	}
	response.JSON(w, http.StatusOK, c)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	name, msg := validateCategory(req.Name, req.Type, req.ParentID, 0)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Name = name

	c, err := h.service.CreateCategory(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "parent_id is invalid")
			return
		}
		response.InternalError(w, r, err, "Failed to create category")
		return
	}
	response.JSON(w, http.StatusCreated, c)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id
	name, msg := validateCategory(req.Name, req.Type, req.ParentID, id)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Name = name

	c, err := h.service.UpdateCategory(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "category not found")
			return
		}
		if errors.Is(err, ErrInvalidInput) {
			response.Error(w, http.StatusBadRequest, "parent_id is invalid")
			return
		}
		response.InternalError(w, r, err, "Failed to update category")
		return
	}
	response.JSON(w, http.StatusOK, c)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteCategory(r.Context(), id); err != nil {
		if errors.Is(err, ErrCategoryInUse) {
			response.Error(w, http.StatusConflict, "category is referenced by transactions or other categories")
			return
		}
		response.InternalError(w, r, err, "Failed to delete category")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
