package finance_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/finance"
	"gv-api/internal/finance/txtype"
	"gv-api/internal/testutil"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func newFinRepo(t *testing.T) *finance.PostgresRepository {
	t.Helper()
	pool := testutil.NewPool(t)
	// CASCADE handles FK order, but truncate transactions first for clarity.
	testutil.Truncate(t, pool, "transactions", "categories", "accounts")
	return finance.NewRepository(pool)
}

func utc(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

// mustTx creates an income/expense transaction without a category.
func mustTx(t *testing.T, repo *finance.PostgresRepository, typ txtype.Type, amount int64, accountID int32, at time.Time) finance.Transaction {
	t.Helper()
	tx, err := repo.CreateTransaction(context.Background(), finance.CreateTransactionRequest{
		Type:       typ,
		Amount:     decimal.NewFromInt(amount),
		AccountID:  accountID,
		OccurredAt: &at,
	})
	require.NoError(t, err)
	return tx
}

func requireDecimal(t *testing.T, want int64, got decimal.Decimal) {
	t.Helper()
	require.True(t, decimal.NewFromInt(want).Equal(got), "want %d, got %s", want, got)
}

func TestIntegration_Accounts_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	created, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "Checking"})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	got, err := repo.GetAccount(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Checking", got.Name)
	requireDecimal(t, 0, got.Total)

	updated, err := repo.UpdateAccount(ctx, finance.UpdateAccountRequest{ID: created.ID, Name: "Renamed"})
	require.NoError(t, err)
	require.Equal(t, "Renamed", updated.Name)

	require.NoError(t, repo.DeleteAccount(ctx, created.ID))

	_, err = repo.GetAccount(ctx, created.ID)
	require.ErrorIs(t, err, finance.ErrNotFound)

	_, err = repo.UpdateAccount(ctx, finance.UpdateAccountRequest{ID: created.ID, Name: "X"})
	require.ErrorIs(t, err, finance.ErrNotFound)
}

func TestIntegration_DeleteAccount_WithTransactions(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	acc, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "Busy"})
	require.NoError(t, err)
	mustTx(t, repo, txtype.Income, 100, acc.ID, utc(2026, 5, 10, 12, 0, 0))

	err = repo.DeleteAccount(ctx, acc.ID)
	require.ErrorIs(t, err, finance.ErrAccountInUse)
}

func TestIntegration_Categories_CRUD_AndInUse(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	parent, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)

	child, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Groceries", ParentID: &parent.ID, Type: txtype.Expense})
	require.NoError(t, err)
	require.NotNil(t, child.ParentID)
	require.Equal(t, parent.ID, *child.ParentID)

	// Parent referenced by child → ErrCategoryInUse.
	err = repo.DeleteCategory(ctx, parent.ID)
	require.ErrorIs(t, err, finance.ErrCategoryInUse)

	// Category referenced by a transaction → ErrCategoryInUse.
	acc, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "A"})
	require.NoError(t, err)
	at := utc(2026, 5, 10, 12, 0, 0)
	_, err = repo.CreateTransaction(ctx, finance.CreateTransactionRequest{
		Type: txtype.Expense, Amount: decimal.NewFromInt(5), AccountID: acc.ID,
		CategoryID: &child.ID, OccurredAt: &at,
	})
	require.NoError(t, err)
	err = repo.DeleteCategory(ctx, child.ID)
	require.ErrorIs(t, err, finance.ErrCategoryInUse)

	// Nonexistent parent → ErrInvalidInput (mapCategoryError FK path).
	bogus := int32(99999)
	_, err = repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Orphan", ParentID: &bogus, Type: txtype.Expense})
	require.ErrorIs(t, err, finance.ErrInvalidInput)

	_, err = repo.GetCategory(ctx, 99999)
	require.ErrorIs(t, err, finance.ErrNotFound)
}

func TestIntegration_CreateTransaction_InvalidRefs(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	// FK violation on account_id → ErrInvalidInput (mapTxError path).
	// Note: category/type mismatch is enforced in the service
	// (assertCategoryMatchesType), not here — see service tests.
	at := utc(2026, 5, 10, 12, 0, 0)
	_, err := repo.CreateTransaction(ctx, finance.CreateTransactionRequest{
		Type: txtype.Income, Amount: decimal.NewFromInt(10), AccountID: 99999, OccurredAt: &at,
	})
	require.ErrorIs(t, err, finance.ErrInvalidInput)

	require.ErrorIs(t, repo.DeleteTransaction(ctx, 99999), finance.ErrNotFound)
}

func TestIntegration_ListTransactions_DateFiltering(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	accA, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "A"})
	require.NoError(t, err)
	accB, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "B"})
	require.NoError(t, err)
	cat, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)

	dayStart := mustTx(t, repo, txtype.Expense, 1, accA.ID, utc(2026, 5, 10, 0, 0, 0))
	dayEnd := mustTx(t, repo, txtype.Expense, 2, accA.ID, utc(2026, 5, 10, 23, 59, 0))
	before := mustTx(t, repo, txtype.Expense, 3, accA.ID, utc(2026, 5, 9, 12, 0, 0))
	after := mustTx(t, repo, txtype.Expense, 4, accA.ID, utc(2026, 5, 11, 12, 0, 0))

	ids := func(txs []finance.Transaction) []int32 {
		out := make([]int32, len(txs))
		for i, tx := range txs {
			out[i] = tx.ID
		}
		return out
	}

	t.Run("from/to covering a day include 00:00 and 23:59", func(t *testing.T) {
		got, err := repo.ListTransactions(ctx, finance.ListTransactionsQuery{
			From: utc(2026, 5, 10, 0, 0, 0),
			// The handler expands a bare `to` date to the end of that day
			// (parseDateEndParam); the repository contract is inclusive <=.
			To: utc(2026, 5, 10, 23, 59, 59),
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []int32{dayStart.ID, dayEnd.ID}, ids(got))
	})

	t.Run("only from", func(t *testing.T) {
		got, err := repo.ListTransactions(ctx, finance.ListTransactionsQuery{From: utc(2026, 5, 11, 0, 0, 0)})
		require.NoError(t, err)
		require.ElementsMatch(t, []int32{after.ID}, ids(got))
	})

	t.Run("only to", func(t *testing.T) {
		got, err := repo.ListTransactions(ctx, finance.ListTransactionsQuery{To: utc(2026, 5, 9, 23, 59, 59)})
		require.NoError(t, err)
		require.ElementsMatch(t, []int32{before.ID}, ids(got))
	})

	t.Run("account filter combines with date range", func(t *testing.T) {
		inB := mustTx(t, repo, txtype.Expense, 5, accB.ID, utc(2026, 5, 10, 12, 0, 0))
		got, err := repo.ListTransactions(ctx, finance.ListTransactionsQuery{
			AccountID: &accB.ID,
			From:      utc(2026, 5, 10, 0, 0, 0),
			To:        utc(2026, 5, 10, 23, 59, 59),
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []int32{inB.ID}, ids(got))
	})

	t.Run("category filter combines with date range", func(t *testing.T) {
		at := utc(2026, 5, 10, 12, 0, 0)
		withCat, err := repo.CreateTransaction(ctx, finance.CreateTransactionRequest{
			Type: txtype.Expense, Amount: decimal.NewFromInt(6), AccountID: accA.ID,
			CategoryID: &cat.ID, OccurredAt: &at,
		})
		require.NoError(t, err)
		got, err := repo.ListTransactions(ctx, finance.ListTransactionsQuery{
			CategoryID: &cat.ID,
			From:       utc(2026, 5, 10, 0, 0, 0),
			To:         utc(2026, 5, 10, 23, 59, 59),
		})
		require.NoError(t, err)
		require.ElementsMatch(t, []int32{withCat.ID}, ids(got))
	})
}

func TestIntegration_Stats_Smoke(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)

	acc, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "Main"})
	require.NoError(t, err)

	// April: +100 income, -40 expense. May: +50 income, -20 expense.
	mustTx(t, repo, txtype.Income, 100, acc.ID, utc(2026, 4, 5, 10, 0, 0))
	mustTx(t, repo, txtype.Expense, 40, acc.ID, utc(2026, 4, 20, 10, 0, 0))
	mustTx(t, repo, txtype.Income, 50, acc.ID, utc(2026, 5, 5, 10, 0, 0))
	mustTx(t, repo, txtype.Expense, 20, acc.ID, utc(2026, 5, 20, 10, 0, 0))

	t.Run("GetMonthlyStats buckets and totals", func(t *testing.T) {
		stats, err := repo.GetMonthlyStats(ctx, finance.MonthlyStatsQuery{
			From: utc(2026, 4, 1, 0, 0, 0),
			To:   utc(2026, 5, 31, 23, 59, 59),
		})
		require.NoError(t, err)
		require.Len(t, stats, 2)

		byMonth := map[string]finance.MonthlyStat{}
		for _, s := range stats {
			byMonth[s.Month] = s
		}
		requireDecimal(t, 100, byMonth["2026-04"].Income)
		requireDecimal(t, 40, byMonth["2026-04"].Expense)
		requireDecimal(t, 60, byMonth["2026-04"].Balance)
		requireDecimal(t, 50, byMonth["2026-05"].Income)
		requireDecimal(t, 20, byMonth["2026-05"].Expense)
		requireDecimal(t, 30, byMonth["2026-05"].Balance)
	})

	t.Run("GetNetWorthSeries walks back from current total", func(t *testing.T) {
		points, err := repo.GetNetWorthSeries(ctx, finance.NetWorthQuery{
			From:        utc(2026, 4, 1, 0, 0, 0),
			To:          utc(2026, 5, 31, 23, 59, 59),
			Granularity: finance.GranularityMonth,
		})
		require.NoError(t, err)
		require.Len(t, points, 2)
		// Account total after the trigger applied all 4 transactions: 90.
		// April bucket = 90 - May net (+30) = 60.
		require.Equal(t, "2026-04-01", points[0].Date)
		requireDecimal(t, 60, points[0].Total)
		require.Equal(t, "2026-05-01", points[1].Date)
		requireDecimal(t, 90, points[1].Total)
	})
}
