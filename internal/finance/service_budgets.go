package finance

import (
	"context"
	"time"

	"gv-api/internal/finance/txtype"

	"github.com/shopspring/decimal"
)

// budgetWarningShare is the share of an expense budget from which it is flagged as warning.
const budgetWarningShare = 0.8

// budgetAverageMonths is how many complete months before the viewed one feed the suggested
// average shown when setting a budget.
const budgetAverageMonths = 3

// SetBudget sets or removes a category's budget. Only income and expense categories can be
// budgeted.
func (s *Service) SetBudget(ctx context.Context, req SetBudgetRequest) error {
	t, err := s.repo.GetCategoryType(ctx, req.CategoryID)
	if err != nil {
		return err
	}
	if t == txtype.Transfer {
		return ErrBudgetTransfer
	}
	return s.repo.SetBudget(ctx, req)
}

// GetBudgetMonth compares every budget in effect at a month with what actually came in and
// went out that month. Actuals roll up the category tree: a budget on a parent covers the
// transactions of all its descendants of the same type.
func (s *Service) GetBudgetMonth(ctx context.Context, month time.Time) (BudgetMonth, error) {
	now := time.Now().In(s.loc)
	if month.IsZero() {
		month = now
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.loc)
	end := start.AddDate(0, 1, 0)

	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return BudgetMonth{}, err
	}
	budgets, err := s.repo.ListEffectiveBudgets(ctx, start)
	if err != nil {
		return BudgetMonth{}, err
	}
	totals, err := s.repo.GetCategoryTotals(ctx, start, end)
	if err != nil {
		return BudgetMonth{}, err
	}
	past, err := s.repo.GetCategoryTotals(ctx, start.AddDate(0, -budgetAverageMonths, 0), start)
	if err != nil {
		return BudgetMonth{}, err
	}

	tree := newCategoryTree(cats)
	actual := tree.rollUp(totals)

	budgetByCat := make(map[int32]EffectiveBudget, len(budgets))
	for _, b := range budgets {
		if c, ok := tree.byID[b.CategoryID]; ok && c.Type != txtype.Transfer {
			budgetByCat[b.CategoryID] = b
		}
	}

	out := BudgetMonth{
		Month:         start.Format("2006-01"),
		MonthProgress: monthProgress(now, start, end),
		Items:         []BudgetItem{},
		Averages:      []BudgetAmount{},
	}
	byType := map[txtype.Type]*BudgetTotals{txtype.Income: &out.Income, txtype.Expense: &out.Expense}
	for _, t := range totals {
		if bt, ok := byType[t.Type]; ok {
			bt.Actual = bt.Actual.Add(t.Amount)
		}
	}
	covered := map[txtype.Type]decimal.Decimal{}

	// Depth-first in category order, counting the budgeted ancestors of the same type so a
	// budgeted child nests under its budgeted parent and is not counted twice in the totals.
	// Each call returns the expense overspend of its subtree: a budget's overspend is the larger
	// of its own excess and the overspend of the budgets nested in it, so a euro spent over a
	// child budget is counted once however many budgets it is over.
	var walk func(id int32, depth map[txtype.Type]int) decimal.Decimal
	walk = func(id int32, depth map[txtype.Type]int) decimal.Decimal {
		c := tree.byID[id]
		next := depth
		b, budgeted := budgetByCat[id]
		if budgeted {
			d := depth[c.Type]
			out.Items = append(out.Items, budgetItem(c, b, actual[id], d))
			if d == 0 {
				bt := byType[c.Type]
				bt.Budgeted = bt.Budgeted.Add(b.Amount)
				covered[c.Type] = covered[c.Type].Add(actual[id])
			}
			next = make(map[txtype.Type]int, len(depth)+1)
			for k, v := range depth {
				next[k] = v
			}
			next[c.Type] = d + 1
		}
		over := decimal.Zero
		for _, child := range tree.children[id] {
			over = over.Add(walk(child, next))
		}
		if budgeted && c.Type == txtype.Expense {
			over = decimal.Max(over, actual[id].Sub(b.Amount))
		}
		return over
	}
	for _, root := range tree.roots {
		out.Expense.Overspent = out.Expense.Overspent.Add(walk(root, map[txtype.Type]int{}))
	}
	for t, bt := range byType {
		bt.Unbudgeted = bt.Actual.Sub(covered[t])
	}

	months := decimal.NewFromInt(budgetAverageMonths)
	pastActual := tree.rollUp(past)
	for _, c := range cats {
		if c.Type == txtype.Transfer {
			continue
		}
		if sum := pastActual[c.ID]; sum.IsPositive() {
			out.Averages = append(out.Averages, BudgetAmount{CategoryID: c.ID, Amount: sum.Div(months).Round(2)})
		}
	}
	return out, nil
}

func budgetItem(c Category, b EffectiveBudget, actual decimal.Decimal, depth int) BudgetItem {
	item := BudgetItem{
		CategoryID: c.ID,
		Name:       c.Name,
		ParentID:   c.ParentID,
		Type:       c.Type,
		Depth:      depth,
		Budget:     b.Amount,
		Actual:     actual,
		Remaining:  b.Amount.Sub(actual),
		Since:      b.Since.Format("2006-01"),
	}
	switch {
	case b.Amount.IsPositive():
		item.Progress, _ = actual.Div(b.Amount).Float64()
	case actual.IsPositive():
		item.Progress = 1
	}
	if c.Type == txtype.Income {
		item.Status = BudgetStatusPending
		if actual.GreaterThanOrEqual(b.Amount) {
			item.Status = BudgetStatusMet
		}
		return item
	}
	switch {
	case actual.GreaterThan(b.Amount):
		item.Status = BudgetStatusOver
	case item.Progress >= budgetWarningShare:
		item.Status = BudgetStatusWarning
	default:
		item.Status = BudgetStatusOK
	}
	return item
}

// monthProgress is the elapsed share of [start, end) at now, clamped to [0, 1].
func monthProgress(now, start, end time.Time) float64 {
	switch {
	case !now.After(start):
		return 0
	case !now.Before(end):
		return 1
	}
	return float64(now.Sub(start)) / float64(end.Sub(start))
}

// categoryTree indexes the flat category list by id and parent, keeping the list's order.
type categoryTree struct {
	byID     map[int32]Category
	children map[int32][]int32
	roots    []int32
}

func newCategoryTree(cats []Category) categoryTree {
	t := categoryTree{
		byID:     make(map[int32]Category, len(cats)),
		children: map[int32][]int32{},
	}
	for _, c := range cats {
		t.byID[c.ID] = c
	}
	for _, c := range cats {
		if c.ParentID != nil {
			if _, ok := t.byID[*c.ParentID]; ok {
				t.children[*c.ParentID] = append(t.children[*c.ParentID], c.ID)
				continue
			}
		}
		t.roots = append(t.roots, c.ID)
	}
	return t
}

// rollUp sums the totals of each category and all its descendants of the same type.
// Uncategorized totals are ignored (no category to attribute them to).
func (t categoryTree) rollUp(totals []CategoryTotal) map[int32]decimal.Decimal {
	direct := map[int32]decimal.Decimal{}
	for _, row := range totals {
		if row.CategoryID != nil {
			direct[*row.CategoryID] = direct[*row.CategoryID].Add(row.Amount)
		}
	}
	out := make(map[int32]decimal.Decimal, len(t.byID))
	var sum func(id int32, typ txtype.Type, seen map[int32]bool) decimal.Decimal
	sum = func(id int32, typ txtype.Type, seen map[int32]bool) decimal.Decimal {
		if seen[id] {
			return decimal.Zero
		}
		seen[id] = true
		total := decimal.Zero
		if t.byID[id].Type == typ {
			total = direct[id]
		}
		for _, child := range t.children[id] {
			total = total.Add(sum(child, typ, seen))
		}
		return total
	}
	for id, c := range t.byID {
		out[id] = sum(id, c.Type, map[int32]bool{})
	}
	return out
}
