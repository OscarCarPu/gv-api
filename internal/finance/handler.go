package finance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gv-api/internal/finance/txtype"
	"gv-api/internal/response"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetAccount(ctx context.Context, id int32) (Account, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, req CreateAccountRequest) (Account, error)
	UpdateAccount(ctx context.Context, req UpdateAccountRequest) (Account, error)
	DeleteAccount(ctx context.Context, id int32) error

	GetCategory(ctx context.Context, id int32) (Category, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error)
	UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (Category, error)
	DeleteCategory(ctx context.Context, id int32) error

	GetTransaction(ctx context.Context, id int32) (Transaction, error)
	ListTransactions(ctx context.Context) ([]Transaction, error)
	ListTransactionsByAccount(ctx context.Context, accountID int32) ([]Transaction, error)
	CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error)
	UpdateTransaction(ctx context.Context, req UpdateTransactionRequest) (Transaction, error)
	DeleteTransaction(ctx context.Context, id int32) error

	GetOverview(ctx context.Context) (Overview, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

func parseID(r *http.Request, label string) (int32, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid " + label + " id")
	}
	return int32(id), nil
}

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

func validateTransaction(t txtype.Type, amount interface{ IsPositive() bool }, accountID int32, toAccountID *int32, categoryID *int32) string {
	if !t.Valid() {
		return "type must be income, expense, or transfer"
	}
	switch t {
	case txtype.Income, txtype.Expense:
		if toAccountID != nil {
			return "to_account_id must be omitted for " + string(t)
		}
	case txtype.Transfer:
		if toAccountID == nil {
			return "to_account_id is required for transfer"
		}
		if *toAccountID == accountID {
			return "to_account_id must differ from account_id"
		}
	}
	if accountID <= 0 {
		return "account_id is required"
	}
	if !amount.IsPositive() {
		return "amount must be greater than 0"
	}
	if categoryID == nil {
		return "category_id is required"
	}
	return ""
}

// --- Accounts ---

func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ListAccounts(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list accounts")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "account")
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
		response.Error(w, http.StatusInternalServerError, "Failed to get account")
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
		response.Error(w, http.StatusInternalServerError, "Failed to create account")
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

func (h *Handler) UpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "account")
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
		response.Error(w, http.StatusInternalServerError, "Failed to update account")
		return
	}
	response.JSON(w, http.StatusOK, a)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "account")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteAccount(r.Context(), id); err != nil {
		if errors.Is(err, ErrAccountInUse) {
			response.Error(w, http.StatusConflict, "account has transactions; delete them first")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to delete account")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Categories ---

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.ListCategories(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list categories")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "category")
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
		response.Error(w, http.StatusInternalServerError, "Failed to get category")
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
		response.Error(w, http.StatusInternalServerError, "Failed to create category")
		return
	}
	response.JSON(w, http.StatusCreated, c)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "category")
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
		response.Error(w, http.StatusInternalServerError, "Failed to update category")
		return
	}
	response.JSON(w, http.StatusOK, c)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteCategory(r.Context(), id); err != nil {
		if errors.Is(err, ErrCategoryInUse) {
			response.Error(w, http.StatusConflict, "category is referenced by transactions or other categories")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to delete category")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Transactions ---

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	if accStr := r.URL.Query().Get("account_id"); accStr != "" {
		acc, err := strconv.Atoi(accStr)
		if err != nil || acc <= 0 {
			response.Error(w, http.StatusBadRequest, "invalid account_id")
			return
		}
		out, err := h.service.ListTransactionsByAccount(r.Context(), int32(acc))
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Failed to list transactions")
			return
		}
		response.JSON(w, http.StatusOK, out)
		return
	}
	out, err := h.service.ListTransactions(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to list transactions")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "transaction")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := h.service.GetTransaction(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "transaction not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "Failed to get transaction")
		return
	}
	response.JSON(w, http.StatusOK, t)
}

func (h *Handler) CreateTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	if msg := validateTransaction(req.Type, req.Amount, req.AccountID, req.ToAccountID, req.CategoryID); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	t, err := h.service.CreateTransaction(r.Context(), req)
	if err != nil {
		writeTxErr(w, err, "create")
		return
	}
	response.JSON(w, http.StatusCreated, t)
}

func (h *Handler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "transaction")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var req UpdateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	req.ID = id
	if msg := validateTransaction(req.Type, req.Amount, req.AccountID, req.ToAccountID, req.CategoryID); msg != "" {
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	if req.OccurredAt.IsZero() {
		response.Error(w, http.StatusBadRequest, "occurred_at is required")
		return
	}
	t, err := h.service.UpdateTransaction(r.Context(), req)
	if err != nil {
		writeTxErr(w, err, "update")
		return
	}
	response.JSON(w, http.StatusOK, t)
}

func (h *Handler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "transaction")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteTransaction(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to delete transaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetOverview -> GET /finance/overview
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	o, err := h.service.GetOverview(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to get overview")
		return
	}
	response.JSON(w, http.StatusOK, o)
}

func writeTxErr(w http.ResponseWriter, err error, op string) {
	switch {
	case errors.Is(err, ErrNotFound):
		response.Error(w, http.StatusNotFound, "transaction not found")
	case errors.Is(err, ErrCategoryMismatch):
		response.Error(w, http.StatusBadRequest, "category type does not match transaction type")
	case errors.Is(err, ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, "referenced account or category does not exist")
	default:
		response.Error(w, http.StatusInternalServerError, "Failed to "+op+" transaction")
	}
}
