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
	return finance.EffectiveBudget{CategoryID: cat, Period: finance.BudgetPeriodMonthly, Since: since, Amount: dec(amount)}
}

func yearlyBudget(cat int32, since time.Time, amount string) finance.EffectiveBudget {
	return finance.EffectiveBudget{CategoryID: cat, Period: finance.BudgetPeriodYearly, Since: since, Amount: dec(amount)}
}

// expectTotals wires the four GetCategoryTotals calls of GetBudgetMonth for a month:
// the month, its year, the 3 months before and the previous year.
func expectTotals(repo *mocks.MockRepository, month time.Time, monthRows, yearRows, pastRows, prevYearRows []finance.CategoryTotal) {
	year := time.Date(month.Year(), time.January, 1, 0, 0, 0, 0, month.Location())
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(month), matchTime(month.AddDate(0, 1, 0))).Return(monthRows, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(year), matchTime(year.AddDate(1, 0, 0))).Return(yearRows, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(month.AddDate(0, -3, 0)), matchTime(month)).Return(pastRows, nil)
	repo.EXPECT().GetCategoryTotals(mock.Anything, matchTime(year.AddDate(-1, 0, 0)), matchTime(year)).Return(prevYearRows, nil)
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
	expectTotals(
		repo, month,
		[]finance.CategoryTotal{
			total(1, txtype.Income, "2100"),
			total(3, txtype.Expense, "90"),
			total(4, txtype.Expense, "35"),
			total(5, txtype.Expense, "200"),
			total(6, txtype.Expense, "40"),
			{CategoryID: nil, Type: txtype.Expense, Amount: dec("10")},
		},
		nil,
		[]finance.CategoryTotal{
			total(5, txtype.Expense, "300"),
			total(6, txtype.Expense, "90"),
		},
		[]finance.CategoryTotal{total(6, txtype.Expense, "700")},
	)

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

	require.Len(t, out.PreviousYear, 1)
	requireDec(t, "700", out.PreviousYear[0].Amount)
	requireDec(t, "1500", out.PlannedBalance)
	assert.Equal(t, "2026", out.Yearly.Year)
	assert.Empty(t, out.Yearly.Items)
}

// Monthly Transport with a yearly Car maintenance under it, and a yearly property tax.
func TestService_GetBudgetMonth_Yearly(t *testing.T) {
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	repo := mocks.NewMockRepository(t)
	repo.EXPECT().ListCategories(mock.Anything).Return([]finance.Category{
		{ID: 1, Name: "Transport", Type: txtype.Expense},
		{ID: 2, Name: "Car maintenance", ParentID: ptr[int32](1), Type: txtype.Expense},
		{ID: 3, Name: "Fuel", ParentID: ptr[int32](1), Type: txtype.Expense},
		{ID: 4, Name: "Property tax", Type: txtype.Expense},
		{ID: 5, Name: "Bonus", Type: txtype.Income},
	}, nil)
	repo.EXPECT().ListEffectiveBudgets(mock.Anything, matchTime(month)).Return([]finance.EffectiveBudget{
		budget(1, jan, "80"),
		yearlyBudget(2, jan, "500"),
		yearlyBudget(4, jan, "450"),
		yearlyBudget(5, jan, "1200"),
	}, nil)
	expectTotals(
		repo, month,
		[]finance.CategoryTotal{
			total(2, txtype.Expense, "380"), // tyres this month
			total(3, txtype.Expense, "60"),
			total(4, txtype.Expense, "452"),
		},
		[]finance.CategoryTotal{
			total(2, txtype.Expense, "425"),
			total(3, txtype.Expense, "170"),
			total(4, txtype.Expense, "452"),
		},
		nil, nil,
	)

	out, err := newSvc(repo).GetBudgetMonth(context.Background(), month)
	require.NoError(t, err)

	// The monthly Transport budget leaves the yearly-budgeted car maintenance out.
	require.Len(t, out.Items, 1)
	requireDec(t, "60", out.Items[0].Actual)
	assert.Equal(t, finance.BudgetPeriodMonthly, out.Items[0].Period)
	assert.Equal(t, finance.BudgetStatusOK, out.Items[0].Status)

	// The month took 892, all of it covered by some budget.
	requireDec(t, "892", out.Expense.Actual)
	requireDec(t, "0", out.Expense.Unbudgeted)
	requireDec(t, "0", out.Expense.Overspent)

	y := out.Yearly
	assert.Equal(t, "2026", y.Year)
	require.Len(t, y.Items, 3)
	car, tax, bonus := y.Items[0], y.Items[1], y.Items[2]
	assert.Equal(t, "Car maintenance", car.Name)
	assert.Equal(t, 0, car.Depth) // depth only counts budgets of the same period
	requireDec(t, "425", car.Actual)
	assert.Equal(t, finance.BudgetStatusWarning, car.Status)
	assert.Equal(t, "2026", car.Since)
	assert.Equal(t, finance.BudgetStatusOver, tax.Status)
	assert.Equal(t, finance.BudgetStatusPending, bonus.Status)

	requireDec(t, "950", y.Expense.Budgeted)
	requireDec(t, "877", y.Expense.Actual)
	requireDec(t, "2", y.Expense.Overspent)
	requireDec(t, "1200", y.Income.Budgeted)

	// -80 monthly + (1200 - 950) / 12 yearly.
	requireDec(t, "-59.17", out.PlannedBalance)
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
	expectTotals(repo, month, []finance.CategoryTotal{
		total(1, txtype.Expense, "80"),
		total(2, txtype.Expense, "5"),
		total(3, txtype.Income, "40"),
	}, nil, nil, nil)

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
	req := finance.SetBudgetRequest{
		CategoryID: 2, Period: finance.BudgetPeriodYearly, Month: time.Now(), Amount: ptr(dec("10")), Scope: finance.BudgetScopeOnce,
	}
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(2)).Return(txtype.Expense, nil)
	repo.EXPECT().SetBudget(mock.Anything, req).Return(nil)

	require.NoError(t, newSvc(repo).SetBudget(context.Background(), req))
}

func TestService_SetBudget_DefaultsToMonthly(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().GetCategoryType(mock.Anything, int32(2)).Return(txtype.Expense, nil)
	repo.EXPECT().SetBudget(mock.Anything, mock.MatchedBy(func(req finance.SetBudgetRequest) bool {
		return req.Period == finance.BudgetPeriodMonthly
	})).Return(nil)

	require.NoError(t, newSvc(repo).SetBudget(context.Background(), finance.SetBudgetRequest{CategoryID: 2}))
}

func TestService_GetBudgetTransactions(t *testing.T) {
	month := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cats := []finance.Category{
		{ID: 1, Name: "Transport", Type: txtype.Expense},
		{ID: 2, Name: "Car maintenance", ParentID: ptr[int32](1), Type: txtype.Expense},
		{ID: 3, Name: "Fuel", ParentID: ptr[int32](1), Type: txtype.Expense},
		{ID: 4, Name: "Refund", ParentID: ptr[int32](1), Type: txtype.Income},
	}
	budgets := []finance.EffectiveBudget{budget(1, jan, "80"), yearlyBudget(2, jan, "500")}

	t.Run("monthly leaves out the yearly subtree and other types", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().ListCategories(mock.Anything).Return(cats, nil)
		repo.EXPECT().ListEffectiveBudgets(mock.Anything, matchTime(month)).Return(budgets, nil)
		repo.EXPECT().ListCategoryTransactions(mock.Anything, []int32{1, 3}, txtype.Expense,
			matchTime(month), matchTime(month.AddDate(0, 1, 0))).Return([]finance.OverviewTransaction{{ID: 7}}, nil)

		out, err := newSvc(repo).GetBudgetTransactions(context.Background(), 1, finance.BudgetPeriodMonthly, month)
		require.NoError(t, err)
		require.Len(t, out, 1)
	})

	t.Run("yearly covers the whole year", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().ListCategories(mock.Anything).Return(cats, nil)
		repo.EXPECT().ListEffectiveBudgets(mock.Anything, matchTime(month)).Return(budgets, nil)
		repo.EXPECT().ListCategoryTransactions(mock.Anything, []int32{2}, txtype.Expense,
			matchTime(jan), matchTime(jan.AddDate(1, 0, 0))).Return(nil, nil)

		_, err := newSvc(repo).GetBudgetTransactions(context.Background(), 2, finance.BudgetPeriodYearly, month)
		require.NoError(t, err)
	})

	t.Run("unknown category", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		repo.EXPECT().ListCategories(mock.Anything).Return(cats, nil)
		_, err := newSvc(repo).GetBudgetTransactions(context.Background(), 99, finance.BudgetPeriodMonthly, month)
		assert.ErrorIs(t, err, finance.ErrNotFound)
	})
}
