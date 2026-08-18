package finance

import (
	"encoding/json"
	"errors"
	"net/http"

	"gv-api/internal/finance/txtype"
	"gv-api/internal/httputil"
	"gv-api/internal/response"
)

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

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	accountID, err := parseOptionalIntParam(q.Get("account_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid account_id")
		return
	}
	categoryID, err := parseOptionalIntParam(q.Get("category_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid category_id")
		return
	}
	var typePtr *txtype.Type
	if v := q.Get("type"); v != "" {
		t := txtype.Type(v)
		if !t.Valid() {
			response.Error(w, http.StatusBadRequest, "type must be income, expense, or transfer")
			return
		}
		typePtr = &t
	}
	from, err := parseDateParam(q.Get("from"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid from")
		return
	}
	to, err := parseDateEndParam(q.Get("to"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid to")
		return
	}
	out, err := h.service.ListTransactions(r.Context(), ListTransactionsQuery{
		AccountID:  accountID,
		CategoryID: categoryID,
		Type:       typePtr,
		From:       from,
		To:         to,
	})
	if err != nil {
		response.InternalError(w, r, err, "Failed to list transactions")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "transaction")
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
		response.InternalError(w, r, err, "Failed to get transaction")
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
		writeTxErr(w, r, err, "create")
		return
	}
	response.JSON(w, http.StatusCreated, t)
}

func (h *Handler) UpdateTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "transaction")
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
		writeTxErr(w, r, err, "update")
		return
	}
	response.JSON(w, http.StatusOK, t)
}

func (h *Handler) DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseIDParam(r, "transaction")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.DeleteTransaction(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Error(w, http.StatusNotFound, "transaction not found")
			return
		}
		response.InternalError(w, r, err, "Failed to delete transaction")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
