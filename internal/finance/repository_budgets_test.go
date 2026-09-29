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

func month(m time.Month) time.Time { return time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC) }

// budgetAt returns the monthly budget in effect for a category at a month, "" when there is none.
func budgetAt(t *testing.T, repo *finance.PostgresRepository, cat int32, m time.Time) string {
	t.Helper()
	return periodBudgetAt(t, repo, finance.BudgetPeriodMonthly, cat, m)
}

func periodBudgetAt(t *testing.T, repo *finance.PostgresRepository, period finance.BudgetPeriod, cat int32, m time.Time) string {
	t.Helper()
	list, err := repo.ListEffectiveBudgets(context.Background(), m)
	require.NoError(t, err)
	for _, b := range list {
		if b.CategoryID == cat && b.Period == period {
			return b.Amount.StringFixed(2)
		}
	}
	return ""
}

func setBudget(t *testing.T, repo *finance.PostgresRepository, cat int32, m time.Time, amount string, scope finance.BudgetScope) {
	t.Helper()
	setPeriodBudget(t, repo, finance.BudgetPeriodMonthly, cat, m, amount, scope)
}

func setPeriodBudget(t *testing.T, repo *finance.PostgresRepository, period finance.BudgetPeriod, cat int32, m time.Time, amount string, scope finance.BudgetScope) {
	t.Helper()
	var a *decimal.Decimal
	if amount != "" {
		d := decimal.RequireFromString(amount)
		a = &d
	}
	require.NoError(t, repo.SetBudget(context.Background(), finance.SetBudgetRequest{
		CategoryID: cat, Period: period, Month: m, Amount: a, Scope: scope,
	}))
}

func TestIntegration_Budgets_Scopes(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	cat, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)

	// Forward carries into every later month.
	setBudget(t, repo, cat.ID, month(3), "100", finance.BudgetScopeForward)
	require.Equal(t, "", budgetAt(t, repo, cat.ID, month(2)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(3)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(12)))

	// Month-only is an exception; the month after goes back to the previous value.
	setBudget(t, repo, cat.ID, month(6), "300", finance.BudgetScopeOnce)
	require.Equal(t, "300.00", budgetAt(t, repo, cat.ID, month(6)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(7)))

	// A later forward change replaces everything after it.
	setBudget(t, repo, cat.ID, month(5), "150", finance.BudgetScopeForward)
	require.Equal(t, "150.00", budgetAt(t, repo, cat.ID, month(6)))
	require.Equal(t, "150.00", budgetAt(t, repo, cat.ID, month(9)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(4)))

	// Removing for one month skips it only.
	setBudget(t, repo, cat.ID, month(8), "", finance.BudgetScopeOnce)
	require.Equal(t, "", budgetAt(t, repo, cat.ID, month(8)))
	require.Equal(t, "150.00", budgetAt(t, repo, cat.ID, month(9)))

	// Zero is a real budget, not a removal.
	setBudget(t, repo, cat.ID, month(10), "0", finance.BudgetScopeForward)
	require.Equal(t, "0.00", budgetAt(t, repo, cat.ID, month(11)))

	// Removing forward ends it.
	setBudget(t, repo, cat.ID, month(4), "", finance.BudgetScopeForward)
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(3)))
	require.Equal(t, "", budgetAt(t, repo, cat.ID, month(4)))
	require.Equal(t, "", budgetAt(t, repo, cat.ID, month(12)))
}

func TestIntegration_Budgets_CollapseRedundantRows(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	pool := testutil.NewPool(t)
	cat, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)

	count := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM budgets WHERE category_id = $1", cat.ID).Scan(&n))
		return n
	}

	setBudget(t, repo, cat.ID, month(3), "100", finance.BudgetScopeForward)
	setBudget(t, repo, cat.ID, month(6), "300", finance.BudgetScopeOnce)
	require.Equal(t, 3, count()) // 03:100, 06:300, 07:100

	// Undoing the exception leaves a single row again.
	setBudget(t, repo, cat.ID, month(6), "100", finance.BudgetScopeOnce)
	require.Equal(t, 1, count())

	// Removing a budget that never started leaves nothing behind.
	setBudget(t, repo, cat.ID, month(3), "", finance.BudgetScopeForward)
	require.Equal(t, 0, count())
	setBudget(t, repo, cat.ID, month(5), "", finance.BudgetScopeOnce)
	require.Equal(t, 0, count())
}

func TestIntegration_Budgets_CategoryDeleteCascades(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	cat, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)
	setBudget(t, repo, cat.ID, month(3), "100", finance.BudgetScopeForward)

	require.NoError(t, repo.DeleteCategory(ctx, cat.ID))
	require.Equal(t, "", budgetAt(t, repo, cat.ID, month(3)))
}

func TestIntegration_Budgets_UnknownCategory(t *testing.T) {
	repo := newFinRepo(t)
	d := decimal.NewFromInt(10)
	err := repo.SetBudget(context.Background(), finance.SetBudgetRequest{
		CategoryID: 9999, Period: finance.BudgetPeriodMonthly, Month: month(3), Amount: &d, Scope: finance.BudgetScopeForward,
	})
	require.ErrorIs(t, err, finance.ErrNotFound)
}

func TestIntegration_CategoryTotals(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	acc, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "Main"})
	require.NoError(t, err)
	mustTx(t, repo, txtype.Expense, 10, acc.ID, utc(2026, 3, 1, 0, 0, 0))
	mustTx(t, repo, txtype.Expense, 20, acc.ID, utc(2026, 3, 31, 23, 0, 0))
	mustTx(t, repo, txtype.Income, 5, acc.ID, utc(2026, 3, 15, 0, 0, 0))
	mustTx(t, repo, txtype.Expense, 99, acc.ID, utc(2026, 4, 1, 0, 0, 0)) // excluded: end is exclusive

	rows, err := repo.GetCategoryTotals(ctx, month(3), month(4))
	require.NoError(t, err)
	got := map[txtype.Type]string{}
	for _, r := range rows {
		require.Nil(t, r.CategoryID)
		got[r.Type] = r.Amount.StringFixed(0)
	}
	require.Equal(t, map[txtype.Type]string{txtype.Expense: "30", txtype.Income: "5"}, got)
}

func TestIntegration_Budgets_Yearly(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	cat, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Property tax", Type: txtype.Expense})
	require.NoError(t, err)
	yearly := finance.BudgetPeriodYearly
	y := func(year, m int) time.Time { return time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.UTC) }

	// Set from any month of the year; it applies to the whole year and carries on.
	setPeriodBudget(t, repo, yearly, cat.ID, y(2026, 6), "450", finance.BudgetScopeForward)
	require.Equal(t, "450.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2026, 1)))
	require.Equal(t, "450.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2028, 12)))
	require.Equal(t, "", periodBudgetAt(t, repo, yearly, cat.ID, y(2025, 12)))

	// Once affects one year; the next goes back.
	setPeriodBudget(t, repo, yearly, cat.ID, y(2027, 3), "500", finance.BudgetScopeOnce)
	require.Equal(t, "500.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2027, 11)))
	require.Equal(t, "450.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2028, 1)))

	// A monthly budget on the same category is a separate series.
	setBudget(t, repo, cat.ID, y(2026, 3), "40", finance.BudgetScopeForward)
	require.Equal(t, "40.00", budgetAt(t, repo, cat.ID, y(2026, 3)))
	require.Equal(t, "450.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2026, 3)))
	setBudget(t, repo, cat.ID, y(2026, 3), "", finance.BudgetScopeForward)
	require.Equal(t, "450.00", periodBudgetAt(t, repo, yearly, cat.ID, y(2026, 3)))

	// Removing the yearly budget forward ends it.
	setPeriodBudget(t, repo, yearly, cat.ID, y(2026, 1), "", finance.BudgetScopeForward)
	require.Equal(t, "", periodBudgetAt(t, repo, yearly, cat.ID, y(2027, 5)))
}

func TestIntegration_ListCategoryTransactions(t *testing.T) {
	ctx := context.Background()
	repo := newFinRepo(t)
	acc, err := repo.CreateAccount(ctx, finance.CreateAccountRequest{Name: "Main"})
	require.NoError(t, err)
	food, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Food", Type: txtype.Expense})
	require.NoError(t, err)
	rent, err := repo.CreateCategory(ctx, finance.CreateCategoryRequest{Name: "Rent", Type: txtype.Expense})
	require.NoError(t, err)

	add := func(cat int32, amount int64, at time.Time) {
		_, err := repo.CreateTransaction(ctx, finance.CreateTransactionRequest{
			Type: txtype.Expense, Amount: decimal.NewFromInt(amount), AccountID: acc.ID, CategoryID: &cat, OccurredAt: &at,
		})
		require.NoError(t, err)
	}
	add(food.ID, 10, utc(2026, 3, 2, 9, 0, 0))
	add(food.ID, 20, utc(2026, 3, 20, 9, 0, 0))
	add(food.ID, 99, utc(2026, 4, 1, 0, 0, 0))  // next month: excluded
	add(rent.ID, 800, utc(2026, 3, 1, 9, 0, 0)) // other category: excluded

	out, err := repo.ListCategoryTransactions(ctx, []int32{food.ID}, txtype.Expense, month(3), month(4))
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, "20", out[0].Amount.String()) // newest first
	require.Equal(t, "Main", out[0].AccountName)
	require.Equal(t, "Food", *out[0].CategoryName)
}
