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

// budgetAt returns the budget in effect for a category at a month, "" when there is none.
func budgetAt(t *testing.T, repo *finance.PostgresRepository, cat int32, m time.Time) string {
	t.Helper()
	list, err := repo.ListEffectiveBudgets(context.Background(), m)
	require.NoError(t, err)
	for _, b := range list {
		if b.CategoryID == cat {
			return b.Amount.StringFixed(2)
		}
	}
	return ""
}

func setBudget(t *testing.T, repo *finance.PostgresRepository, cat int32, m time.Time, amount string, scope finance.BudgetScope) {
	t.Helper()
	var a *decimal.Decimal
	if amount != "" {
		d := decimal.RequireFromString(amount)
		a = &d
	}
	require.NoError(t, repo.SetBudget(context.Background(), finance.SetBudgetRequest{
		CategoryID: cat, Month: m, Amount: a, Scope: scope,
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
	setBudget(t, repo, cat.ID, month(6), "300", finance.BudgetScopeMonth)
	require.Equal(t, "300.00", budgetAt(t, repo, cat.ID, month(6)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(7)))

	// A later forward change replaces everything after it.
	setBudget(t, repo, cat.ID, month(5), "150", finance.BudgetScopeForward)
	require.Equal(t, "150.00", budgetAt(t, repo, cat.ID, month(6)))
	require.Equal(t, "150.00", budgetAt(t, repo, cat.ID, month(9)))
	require.Equal(t, "100.00", budgetAt(t, repo, cat.ID, month(4)))

	// Removing for one month skips it only.
	setBudget(t, repo, cat.ID, month(8), "", finance.BudgetScopeMonth)
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
	setBudget(t, repo, cat.ID, month(6), "300", finance.BudgetScopeMonth)
	require.Equal(t, 3, count()) // 03:100, 06:300, 07:100

	// Undoing the exception leaves a single row again.
	setBudget(t, repo, cat.ID, month(6), "100", finance.BudgetScopeMonth)
	require.Equal(t, 1, count())

	// Removing a budget that never started leaves nothing behind.
	setBudget(t, repo, cat.ID, month(3), "", finance.BudgetScopeForward)
	require.Equal(t, 0, count())
	setBudget(t, repo, cat.ID, month(5), "", finance.BudgetScopeMonth)
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
		CategoryID: 9999, Month: month(3), Amount: &d, Scope: finance.BudgetScopeForward,
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
