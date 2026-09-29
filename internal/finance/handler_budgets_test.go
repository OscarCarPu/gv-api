package finance_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gv-api/internal/finance"
	"gv-api/internal/finance/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandler_GetBudgets_InvalidMonth(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/budgets?month=2026-13", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).GetBudgets(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetBudgets_DefaultsToCurrentMonth(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().GetBudgetMonth(mock.Anything, mock.MatchedBy(func(m time.Time) bool { return m.IsZero() })).
		Return(finance.BudgetMonth{}, nil)

	rec := httptest.NewRecorder()
	finance.NewHandler(svc).GetBudgets(rec, httptest.NewRequest(http.MethodGet, "/finance/budgets", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_SetBudget_Validation(t *testing.T) {
	cases := map[string]string{
		"missing month":  `{"amount":"10"}`,
		"bad month":      `{"month":"2026-9","amount":"10"}`,
		"negative":       `{"month":"2026-09","amount":"-1"}`,
		"too big":        `{"month":"2026-09","amount":"10000000000000"}`,
		"bad scope":      `{"month":"2026-09","amount":"10","scope":"all"}`,
		"bad period":     `{"month":"2026-09","amount":"10","period":"weekly"}`,
		"monthly year":   `{"month":"2026","amount":"10"}`,
		"malformed body": `{`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := withFinIDParam(newFinReq(http.MethodPut, "/", body), "id", "3")
			finance.NewHandler(mocks.NewMockServiceInterface(t)).SetBudget(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_SetBudget_DefaultsToForwardAndRounds(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().SetBudget(mock.Anything, mock.MatchedBy(func(req finance.SetBudgetRequest) bool {
		return req.CategoryID == 3 &&
			req.Period == finance.BudgetPeriodMonthly &&
			req.Month.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) &&
			req.Scope == finance.BudgetScopeForward &&
			req.Amount != nil && req.Amount.Equal(dec("10.13"))
	})).Return(nil)

	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodPut, "/", `{"month":"2026-09","amount":"10.126"}`), "id", "3")
	finance.NewHandler(svc).SetBudget(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_SetBudget_Errors(t *testing.T) {
	cases := map[error]int{
		finance.ErrNotFound:       http.StatusNotFound,
		finance.ErrBudgetTransfer: http.StatusBadRequest,
	}
	for err, code := range cases {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().SetBudget(mock.Anything, mock.Anything).Return(err)
		rec := httptest.NewRecorder()
		req := withFinIDParam(newFinReq(http.MethodPut, "/", `{"month":"2026-09","amount":"10"}`), "id", "3")
		finance.NewHandler(svc).SetBudget(rec, req)
		assert.Equal(t, code, rec.Code, err.Error())
	}
}

func TestHandler_DeleteBudget_SendsNilAmount(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().SetBudget(mock.Anything, mock.MatchedBy(func(req finance.SetBudgetRequest) bool {
		return req.CategoryID == 3 && req.Amount == nil && req.Scope == finance.BudgetScopeMonth
	})).Return(nil)

	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodDelete, "/?month=2026-09&scope=month", ""), "id", "3")
	finance.NewHandler(svc).DeleteBudget(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_DeleteBudget_MissingMonth(t *testing.T) {
	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodDelete, "/", ""), "id", "3")
	finance.NewHandler(mocks.NewMockServiceInterface(t)).DeleteBudget(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_SetBudget_Yearly(t *testing.T) {
	for _, month := range []string{"2027", "2027-05"} {
		svc := mocks.NewMockServiceInterface(t)
		svc.EXPECT().SetBudget(mock.Anything, mock.MatchedBy(func(req finance.SetBudgetRequest) bool {
			return req.Period == finance.BudgetPeriodYearly && req.Month.Year() == 2027 && req.Scope == finance.BudgetScopeOnce
		})).Return(nil)

		rec := httptest.NewRecorder()
		body := `{"month":"` + month + `","amount":"450","period":"yearly","scope":"once"}`
		req := withFinIDParam(newFinReq(http.MethodPut, "/", body), "id", "3")
		finance.NewHandler(svc).SetBudget(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code, month)
	}
}

func TestHandler_DeleteBudget_Yearly(t *testing.T) {
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().SetBudget(mock.Anything, mock.MatchedBy(func(req finance.SetBudgetRequest) bool {
		return req.Period == finance.BudgetPeriodYearly && req.Amount == nil && req.Month.Year() == 2026
	})).Return(nil)

	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodDelete, "/?month=2026&period=yearly", ""), "id", "3")
	finance.NewHandler(svc).DeleteBudget(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}
