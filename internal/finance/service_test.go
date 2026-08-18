package finance_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/finance"
	"gv-api/internal/finance/mocks"
	"gv-api/internal/finance/txtype"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newSvc(repo finance.Repository) *finance.Service {
	return finance.NewService(repo, time.UTC)
}

func ptr[T any](v T) *T { return &v }

func matchTime(t time.Time) interface{} {
	return mock.MatchedBy(func(got time.Time) bool { return got.Equal(t) })
}

// --- CreateTransaction ---

func TestService_CreateTransaction_CategoryMismatch(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(5)).Return(txtype.Type("expense"), nil)

	_, err := newSvc(repo).CreateTransaction(context.Background(), finance.CreateTransactionRequest{
		Type:       txtype.Type("income"),
		Amount:     decimal.NewFromInt(100),
		AccountID:  1,
		CategoryID: ptr[int32](5),
	})
	assert.ErrorIs(t, err, finance.ErrCategoryMismatch)
}

func TestService_CreateTransaction_CategoryNotFound(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(5)).Return(txtype.Type(""), finance.ErrNotFound)

	_, err := newSvc(repo).CreateTransaction(context.Background(), finance.CreateTransactionRequest{
		Type:       txtype.Type("income"),
		Amount:     decimal.NewFromInt(100),
		AccountID:  1,
		CategoryID: ptr[int32](5),
	})
	assert.ErrorIs(t, err, finance.ErrInvalidInput)
}

func TestService_CreateTransaction_NilCategory(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().CreateTransaction(mock.Anything, mock.Anything).Return(finance.Transaction{ID: 1}, nil)

	tx, err := newSvc(repo).CreateTransaction(context.Background(), finance.CreateTransactionRequest{
		Type:      txtype.Type("income"),
		Amount:    decimal.NewFromInt(50),
		AccountID: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), tx.ID)
}

func TestService_UpdateTransaction_CategoryMismatch(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(7)).Return(txtype.Type("income"), nil)

	_, err := newSvc(repo).UpdateTransaction(context.Background(), finance.UpdateTransactionRequest{
		ID:         1,
		Type:       txtype.Type("expense"),
		Amount:     decimal.NewFromInt(20),
		AccountID:  1,
		CategoryID: ptr[int32](7),
	})
	assert.ErrorIs(t, err, finance.ErrCategoryMismatch)
}

// --- GetOverview ---

func TestService_GetOverview_AssemblesCorrectly(t *testing.T) {
	loc := time.UTC
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	prevStart := monthStart.AddDate(0, -1, 0)

	income := decimal.NewFromInt(1000)
	expense := decimal.NewFromInt(400)
	prevPlusCurIncome := decimal.NewFromInt(1800)
	prevPlusCurExpense := decimal.NewFromInt(700)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetAccountsTotal(mock.Anything).Return(decimal.NewFromInt(5000), nil)
	repo.EXPECT().GetMonthlyTotals(mock.Anything, matchTime(monthStart)).Return(income, expense, nil)
	repo.EXPECT().GetMonthlyTotals(mock.Anything, matchTime(prevStart)).Return(prevPlusCurIncome, prevPlusCurExpense, nil)
	repo.EXPECT().ListRecentTransactions(mock.Anything, mock.Anything).Return(nil, nil)

	got, err := finance.NewService(repo, loc).GetOverview(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "5000", got.AccountsTotal.String())
	assert.Equal(t, "1000", got.Month.Income.String())
	assert.Equal(t, "400", got.Month.Expense.String())
	assert.Equal(t, "600", got.Month.Balance.String())
	assert.Equal(t, "800", got.PreviousMonth.Income.String())
	assert.Equal(t, "300", got.PreviousMonth.Expense.String())
	assert.Equal(t, "500", got.PreviousMonth.Balance.String())
}

// --- GetEstimation ---

func TestService_GetEstimation_SavingMode_WithActuals(t *testing.T) {
	loc := time.UTC
	now := time.Now().In(loc)
	startMonth := time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, loc)
	endMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
	earliestTx := startMonth.AddDate(0, 0, 5)

	actualPts := []finance.NetWorthPoint{
		{Date: startMonth.Format("2006-01-02"), Total: decimal.NewFromInt(10000)},
		{Date: startMonth.AddDate(0, 1, 0).Format("2006-01-02"), Total: decimal.NewFromInt(10500)},
	}

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEarliestTransactionDate(mock.Anything).Return(earliestTx, true, nil)
	repo.EXPECT().GetNetWorthSeries(mock.Anything, mock.Anything).Return(actualPts, nil)

	got, err := finance.NewService(repo, loc).GetEstimation(context.Background(), finance.EstimationQuery{
		StartMonth: startMonth,
		EndMonth:   endMonth,
		Mode:       finance.EstimationModeSaving,
	})
	require.NoError(t, err)
	assert.Equal(t, "500", got.Saving.String())

	var hasEstimated bool
	for _, p := range got.Points {
		if p.Estimated {
			hasEstimated = true
		}
	}
	assert.True(t, hasEstimated)
}

func TestService_GetEstimation_RateMode_NoActuals(t *testing.T) {
	loc := time.UTC
	now := time.Now().In(loc)
	// Start in the future so there are no actual points.
	startMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
	endMonth := startMonth.AddDate(0, 2, 0)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEarliestTransactionDate(mock.Anything).Return(time.Time{}, false, nil)

	got, err := finance.NewService(repo, loc).GetEstimation(context.Background(), finance.EstimationQuery{
		StartMonth: startMonth,
		EndMonth:   endMonth,
		Mode:       finance.EstimationModeRate,
	})
	require.NoError(t, err)
	assert.True(t, got.Rate.IsZero())
	for _, p := range got.Points {
		assert.True(t, p.Estimated)
	}
}

// --- normalizeStatsRange (via GetNetWorthSeries) ---

func TestService_GetNetWorthSeries_DefaultsFromEarliestTx(t *testing.T) {
	// When From is zero and transactions exist, From should be set to
	// one day before the earliest transaction (day granularity default).
	earliest := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	expected := earliest.AddDate(0, 0, -1)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEarliestTransactionDate(mock.Anything).Return(earliest, true, nil)
	repo.EXPECT().
		GetNetWorthSeries(mock.Anything, mock.MatchedBy(func(q finance.NetWorthQuery) bool {
			return q.From.Equal(expected)
		})).
		Return(nil, nil)

	_, err := finance.NewService(repo, time.UTC).GetNetWorthSeries(context.Background(), finance.NetWorthQuery{})
	require.NoError(t, err)
}

func TestService_GetNetWorthSeries_DefaultsFrom6MonthsWhenNoTx(t *testing.T) {
	// When From is zero and there are no transactions, From defaults to ~6 months ago.
	// We only assert the repo is called (From is non-zero); exact value is time-sensitive.
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetEarliestTransactionDate(mock.Anything).Return(time.Time{}, false, nil)
	repo.EXPECT().
		GetNetWorthSeries(mock.Anything, mock.MatchedBy(func(q finance.NetWorthQuery) bool {
			return !q.From.IsZero()
		})).
		Return(nil, nil)

	_, err := finance.NewService(repo, time.UTC).GetNetWorthSeries(context.Background(), finance.NetWorthQuery{})
	require.NoError(t, err)
}
