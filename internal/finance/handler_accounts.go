package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"gv-api/internal/httputil"
	"gv-api/internal/response"
)

func validateAccount(name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "name is required"
	}
	if len(name) > 40 {
		return "", "name must be at most 40 characters"
	}
	return name, ""
}

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ListAccounts(r.Context())
	if err != nil {
		response.InternalError(w, r, err, "Failed to list accounts")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	a, err := h.service.GetAccount(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "account not found")
			return
		}
		response.InternalError(w, r, err, "Failed to get account")
		return
	}
	response.JSON(w, http.StatusOK, a)
}

func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	name, msg := validateAccount(req.Name)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Name = name

	a, err := h.service.CreateAccount(r.Context(), req)
	if err != nil {
		response.InternalError(w, r, err, "Failed to create account")
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id
	name, msg := validateAccount(req.Name)
	if msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	req.Name = name

	a, err := h.service.UpdateAccount(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "account not found")
			return
		}
		response.InternalError(w, r, err, "Failed to update account")
		return
	}
	response.JSON(w, http.StatusOK, a)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteAccount(r.Context(), id); err != nil {
		if errors.Is(err, ErrAccountInUse) {
			response.Error(w, http.StatusConflict, "account has transactions; delete them first")
			return
		}
		response.InternalError(w, r, err, "Failed to delete account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
