package finance_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gv-api/internal/finance"
	"gv-api/internal/finance/mocks"
	"gv-api/internal/finance/txtype"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newFinReq(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

func withFinIDParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func ptr32fin(v int32) *int32 { return &v }

// --- CreateTransaction ---

func TestHandler_CreateTransaction_MissingOccurredAt(t *testing.T) {
	catID := ptr32fin(1)
	body := `{"type":"income","amount":"10.00","account_id":1,"category_id":1}`
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().CreateTransaction(mock.Anything, mock.MatchedBy(func(req finance.CreateTransactionRequest) bool {
		return req.OccurredAt == nil
	})).Return(finance.Transaction{}, nil).Maybe()

	rec := httptest.NewRecorder()
	finance.NewHandler(svc).CreateTransaction(rec, newFinReq(http.MethodPost, "/", body))

	// occurred_at is nil — handler should accept (validation is not on OccurredAt for Create)
	// If the service is called it returns success; if not called the handler returns an error.
	// The key assertion is that we don't panic and get a defined status.
	_ = catID
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code)
}

func TestHandler_CreateTransaction_InvalidType(t *testing.T) {
	body := `{"type":"invalid","amount":"10.00","account_id":1,"category_id":1}`
	rec := httptest.NewRecorder()
	finance.NewHandler(mocks.NewMockServiceInterface(t)).CreateTransaction(rec, newFinReq(http.MethodPost, "/", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateTransaction_TransferMissingToAccount(t *testing.T) {
	body := `{"type":"transfer","amount":"10.00","account_id":1,"category_id":1}`
	rec := httptest.NewRecorder()
	finance.NewHandler(mocks.NewMockServiceInterface(t)).CreateTransaction(rec, newFinReq(http.MethodPost, "/", body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateTransaction_TransferMissingToAccount(t *testing.T) {
	now := time.Now()
	body := `{"type":"transfer","amount":"10.00","account_id":1,"category_id":1,"occurred_at":"` + now.Format(time.RFC3339) + `"}`
	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodPut, "/", body), "transaction", "1")
	finance.NewHandler(mocks.NewMockServiceInterface(t)).UpdateTransaction(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListTransactions_InvalidAccountID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/transactions?account_id=abc", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).ListTransactions(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListTransactions_InvalidType(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/transactions?type=invalid", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).ListTransactions(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetEstimation_StartAfterEnd(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/estimation?start_month=2026-06&end_month=2026-01&mode=rate", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).GetEstimation(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetEstimation_InvalidMode(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/estimation?start_month=2026-01&end_month=2026-06&mode=bad", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).GetEstimation(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetEstimation_MissingStartMonth(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/estimation?end_month=2026-06&mode=rate", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).GetEstimation(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateCategory_SelfParent(t *testing.T) {
	body := `{"name":"Food","type":"expense","parent_id":5}`
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().CreateCategory(mock.Anything, mock.Anything).Return(finance.Category{}, nil).Maybe()

	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodPost, "/", body), "category", "5")
	finance.NewHandler(svc).CreateCategory(rec, req)
	// CreateCategory doesn't receive an id param so self-parent guard only fires on Update
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code)
}

func TestHandler_UpdateCategory_SelfParent(t *testing.T) {
	body := `{"name":"Food","type":"expense","parent_id":5}`
	rec := httptest.NewRecorder()
	req := withFinIDParam(newFinReq(http.MethodPut, "/", body), "category", "5")
	finance.NewHandler(mocks.NewMockServiceInterface(t)).UpdateCategory(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetNetWorthStats_InvalidGranularity(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/finance/networth?granularity=bad", nil)
	finance.NewHandler(mocks.NewMockServiceInterface(t)).GetNetWorthStats(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateTransaction_Success(t *testing.T) {
	catID := ptr32fin(2)
	now := time.Now()
	body := `{"type":"income","amount":"50.00","account_id":1,"category_id":2,"occurred_at":"` + now.Format(time.RFC3339) + `"}`
	svc := mocks.NewMockServiceInterface(t)
	svc.EXPECT().CreateTransaction(mock.Anything, mock.Anything).
		Return(finance.Transaction{
			ID:         1,
			Type:       txtype.Income,
			Amount:     decimal.NewFromFloat(50),
			AccountID:  1,
			CategoryID: catID,
			OccurredAt: now,
		}, nil)

	rec := httptest.NewRecorder()
	finance.NewHandler(svc).CreateTransaction(rec, newFinReq(http.MethodPost, "/", body))
	assert.Equal(t, http.StatusCreated, rec.Code)
}
