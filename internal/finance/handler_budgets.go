package finance

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"gv-api/internal/httputil"
	"gv-api/internal/response"

	"github.com/shopspring/decimal"
)

// maxBudgetAmount is the largest value NUMERIC(15,2) holds.
var maxBudgetAmount = decimal.RequireFromString("9999999999999.99")

type setBudgetBody struct {
	Month  string          `json:"month"`
	Amount decimal.Decimal `json:"amount"`
	Scope  BudgetScope     `json:"scope"`
}

// parseBudgetMonth reads a YYYY-MM month. Empty means the current month (zero time).
func parseBudgetMonth(s string, required bool) (time.Time, error) {
	if s == "" {
		if required {
			return time.Time{}, errors.New("month is required (YYYY-MM)")
		}
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, errors.New("month must be YYYY-MM")
	}
	return t, nil
}

// parseBudgetScope defaults to forward when empty.
func parseBudgetScope(s BudgetScope) (BudgetScope, error) {
	if s == "" {
		return BudgetScopeForward, nil
	}
	if !s.Valid() {
		return "", errors.New("scope must be forward or month")
	}
	return s, nil
}

// GetBudgets -> GET /finance/budgets?month=YYYY-MM
func (h *Handler) GetBudgets(w http.ResponseWriter, r *http.Request) {
	month, err := parseBudgetMonth(r.URL.Query().Get("month"), false)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.service.GetBudgetMonth(r.Context(), month)
	if err != nil {
		response.InternalError(w, r, err, "Failed to get budgets")
		return
	}
	response.JSON(w, http.StatusOK, out)
}

// SetBudget -> PUT /finance/budgets/{category_id}
func (h *Handler) SetBudget(w http.ResponseWriter, r *http.Request) {
	categoryID, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var body setBudgetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid Body")
		return
	}
	month, err := parseBudgetMonth(body.Month, true)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := parseBudgetScope(body.Scope)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Amount.IsNegative() || body.Amount.GreaterThan(maxBudgetAmount) {
		response.Error(w, http.StatusBadRequest, "amount must be between 0 and 9999999999999.99")
		return
	}
	amount := body.Amount.Round(2)
	h.writeBudget(w, r, SetBudgetRequest{CategoryID: categoryID, Month: month, Amount: &amount, Scope: scope})
}

// DeleteBudget -> DELETE /finance/budgets/{category_id}?month=YYYY-MM&scope=forward|month
func (h *Handler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	categoryID, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	month, err := parseBudgetMonth(r.URL.Query().Get("month"), true)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := parseBudgetScope(BudgetScope(r.URL.Query().Get("scope")))
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeBudget(w, r, SetBudgetRequest{CategoryID: categoryID, Month: month, Scope: scope})
}

func (h *Handler) writeBudget(w http.ResponseWriter, r *http.Request, req SetBudgetRequest) {
	if err := h.service.SetBudget(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(w, http.StatusNotFound, "category not found")
		case errors.Is(err, ErrBudgetTransfer):
			response.Error(w, http.StatusBadRequest, "transfer categories cannot be budgeted")
		default:
			response.InternalError(w, r, err, "Failed to save budget")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
