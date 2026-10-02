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
	Period BudgetPeriod    `json:"period"`
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

// parseBudgetPeriodMonth reads YYYY-MM, or a bare YYYY for yearly changes.
func parseBudgetPeriodMonth(s string, period BudgetPeriod) (time.Time, error) {
	if period == BudgetPeriodYearly {
		if t, err := time.Parse("2006", s); err == nil {
			return t, nil
		}
	}
	return parseBudgetMonth(s, true)
}

// parseBudgetScope defaults to forward when empty.
func parseBudgetScope(s BudgetScope) (BudgetScope, error) {
	if s == "" {
		return BudgetScopeForward, nil
	}
	if !s.Valid() {
		return "", errors.New("scope must be forward or once")
	}
	return s, nil
}

// parseBudgetPeriod defaults to monthly when empty.
func parseBudgetPeriod(p BudgetPeriod) (BudgetPeriod, error) {
	if p == "" {
		return BudgetPeriodMonthly, nil
	}
	if !p.Valid() {
		return "", errors.New("period must be monthly or yearly")
	}
	return p, nil
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

// GetBudgetTransactions -> GET /finance/budgets/{category_id}/transactions?month=YYYY-MM&period=monthly|yearly
func (h *Handler) GetBudgetTransactions(w http.ResponseWriter, r *http.Request) {
	categoryID, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	period, err := parseBudgetPeriod(BudgetPeriod(r.URL.Query().Get("period")))
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	month, err := parseBudgetMonth(r.URL.Query().Get("month"), false)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.service.GetBudgetTransactions(r.Context(), categoryID, period, month)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			response.Error(w, http.StatusNotFound, "category not found")
		case errors.Is(err, ErrBudgetTransfer):
			response.Error(w, http.StatusBadRequest, "transfer categories cannot be budgeted")
		default:
			response.InternalError(w, r, err, "Failed to list budget transactions")
		}
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
	period, err := parseBudgetPeriod(body.Period)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	month, err := parseBudgetPeriodMonth(body.Month, period)
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
	h.writeBudget(w, r, SetBudgetRequest{
		CategoryID: categoryID, Period: period, Month: month, Amount: &amount, Scope: scope,
	})
}

// DeleteBudget -> DELETE /finance/budgets/{category_id}?month=YYYY-MM&scope=forward|once&period=monthly|yearly
func (h *Handler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	categoryID, err := httputil.ParseIDParam(r, "category")
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	period, err := parseBudgetPeriod(BudgetPeriod(r.URL.Query().Get("period")))
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	month, err := parseBudgetPeriodMonth(r.URL.Query().Get("month"), period)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	scope, err := parseBudgetScope(BudgetScope(r.URL.Query().Get("scope")))
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeBudget(w, r, SetBudgetRequest{CategoryID: categoryID, Period: period, Month: month, Scope: scope})
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
