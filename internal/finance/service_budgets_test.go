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

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func requireDec(t *testing.T, want string, got decimal.Decimal) {
	t.Helper()
	require.True(t, dec(want).Equal(got), "want %s, got %s", want, got)
}

// budgetCategories is a small tree:
//
//	1 Salary (income)
//	2 Food (expense) > 3 Eating out > 4 Coffee
//	2 Food           > 5 Groceries
//	6 Transport (expense)
//	7 Savings (transfer)
func budgetCategories() []finance.Category {
	return []finance.Category{
		{ID: 1, Name: "Salary", Type: txtype.Income},
		{ID: 2, Name: "Food", Type: txtype.Expense},
		{ID: 3, Name: "Eating out", ParentID: ptr[int32](2), Type: txtype.Expense},
		{ID: 5, Name: "Groceries", ParentID: ptr[int32](2), Type: txtype.Expense},
		{ID: 4, Name: "Coffee", ParentID: ptr[int32](3), Type: txtype.Expense},
		{ID: 6, Name: "Transport", Type: txtype.Expense},
		{ID: 7, Name: "Savings", Type: txtype.Transfer},
	}
}

func total(cat int32, typ txtype.Type, amount string) finance.CategoryTotal {
	return finance.CategoryTotal{CategoryID: ptr(cat), Type: typ, Amount: dec(amount)}
}

func budget(cat int32, since time.Time, amount string) finance.EffectiveBudget {
	return finance.EffectiveBudget{CategoryID: cat, Since: since, Amount: dec(amount)}
}

func TestService_GetBudgetMonth(t *testing.T) {
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListCategories(mock.Anything).Return(budgetCategories(), nil)
	repo.EXPECT().ListEffectiveBudgets(mock.Anything, matchTime(month)).Return([]finance.EffectiveBudget{
		budget(1, since, "2000"),
		budget(2, since, "500"),
		budget(3, since, "100"),
		budget(4, month, "20"),
		budget(7, since, "50"), // transfer: ignored
	}, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(month), matchTime(month.AddDate(0, 1, 0))).
		Return([]finance.CategoryTotal{
			total(1, txtype.Income, "2100"),
			total(3, txtype.Expense, "90"),
			total(4, txtype.Expense, "35"),
			total(5, txtype.Expense, "200"),
			total(6, txtype.Expense, "40"),
			{CategoryID: nil, Type: txtype.Expense, Amount: dec("10")},
		}, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(month.AddDate(0, -3, 0)), matchTime(month)).
		Return([]finance.CategoryTotal{
			total(5, txtype.Expense, "300"),
			total(6, txtype.Expense, "90"),
		}, nil)

	out, err := newSvc(repo).GetBudgetMonth(context.Background(), month)
	require.NoError(t, err)

	assert.Equal(t, "2026-03", out.Month)
	assert.Equal(t, 1.0, out.MonthProgress) // a past month

	// Items come depth-first in category order, nested budgets one level deeper.
	require.Len(t, out.Items, 4)
	names := []string{}
	for _, it := range out.Items {
		names = append(names, it.Name)
	}
	assert.Equal(t, []string{"Salary", "Food", "Eating out", "Coffee"}, names)

	salary, food, eating, coffee := out.Items[0], out.Items[1], out.Items[2], out.Items[3]
	assert.Equal(t, finance.BudgetStatusMet, salary.Status)

	// Food rolls up Eating out (90) + Coffee (35) + Groceries (200).
	assert.Equal(t, 0, food.Depth)
	requireDec(t, "325", food.Actual)
	requireDec(t, "175", food.Remaining)
	assert.Equal(t, finance.BudgetStatusOK, food.Status)
	assert.Equal(t, "2026-01", food.Since)

	assert.Equal(t, 1, eating.Depth)
	requireDec(t, "125", eating.Actual)
	assert.Equal(t, finance.BudgetStatusOver, eating.Status)

	assert.Equal(t, 2, coffee.Depth)
	assert.Equal(t, finance.BudgetStatusOver, coffee.Status)
	assert.Equal(t, "2026-03", coffee.Since)

	// Only the outermost budgets count towards the budgeted total.
	requireDec(t, "500", out.Expense.Budgeted)
	requireDec(t, "375", out.Expense.Actual)
	// Transport (40) and the uncategorized 10 sit outside every budget.
	requireDec(t, "50", out.Expense.Unbudgeted)
	// Coffee is 15 over, Eating out 25 over (incl. Coffee's): counted once as 25.
	requireDec(t, "25", out.Expense.Overspent)

	requireDec(t, "2000", out.Income.Budgeted)
	requireDec(t, "2100", out.Income.Actual)
	requireDec(t, "0", out.Income.Unbudgeted)
	requireDec(t, "0", out.Income.Overspent)

	// Averages roll up too and skip categories with nothing spent.
	avg := map[int32]string{}
	for _, a := range out.Averages {
		avg[a.CategoryID] = a.Amount.String()
	}
	assert.Equal(t, map[int32]string{2: "100", 5: "100", 6: "30"}, avg)
}

func TestService_GetBudgetMonth_StatusThresholds(t *testing.T) {
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListCategories(mock.Anything).Return([]finance.Category{
		{ID: 1, Name: "Warn", Type: txtype.Expense},
		{ID: 2, Name: "Zero", Type: txtype.Expense},
		{ID: 3, Name: "Pending", Type: txtype.Income},
	}, nil)
	repo.EXPECT().ListEffectiveBudgets(mock.Anything, mock.Anything).Return([]finance.EffectiveBudget{
		budget(1, month, "100"),
		budget(2, month, "0"),
		budget(3, month, "100"),
	}, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(month), mock.Anything).Return([]finance.CategoryTotal{
		total(1, txtype.Expense, "80"),
		total(2, txtype.Expense, "5"),
		total(3, txtype.Income, "40"),
	}, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, mock.Anything, matchTime(month)).Return(nil, nil)

	out, err := newSvc(repo).GetBudgetMonth(context.Background(), month)
	require.NoError(t, err)
	require.Len(t, out.Items, 3)

	assert.Equal(t, finance.BudgetStatusWarning, out.Items[0].Status)
	assert.InDelta(t, 0.8, out.Items[0].Progress, 1e-9)
	// A zero budget with spending is over, at 100%.
	assert.Equal(t, finance.BudgetStatusOver, out.Items[1].Status)
	assert.Equal(t, 1.0, out.Items[1].Progress)
	assert.Equal(t, finance.BudgetStatusPending, out.Items[2].Status)
	requireDec(t, "5", out.Expense.Overspent)
}

func TestService_SetBudget_TransferRejected(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(7)).Return(txtype.Transfer, nil)

	err := newSvc(repo).SetBudget(context.Background(), finance.SetBudgetRequest{CategoryID: 7})
	assert.ErrorIs(t, err, finance.ErrBudgetTransfer)
}

func TestService_SetBudget_CategoryNotFound(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(9)).Return(txtype.Type(""), finance.ErrNotFound)

	err := newSvc(repo).SetBudget(context.Background(), finance.SetBudgetRequest{CategoryID: 9})
	assert.ErrorIs(t, err, finance.ErrNotFound)
}

func TestService_SetBudget_Delegates(t *testing.T) {
	req := finance.SetBudgetRequest{CategoryID: 2, Month: time.Now(), Amount: ptr(dec("10")), Scope: finance.BudgetScopeMonth}
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(2)).Return(txtype.Expense, nil)
	repo.EXPECT().SetBudget(mock.Anything, req).Return(nil)

	require.NoError(t, newSvc(repo).SetBudget(context.Background(), req))
}
