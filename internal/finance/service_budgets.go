package finance

import (
	"context"
	"time"

	"gv-api/internal/finance/txtype"

	"github.com/shopspring/decimal"
)

// budgetWarningShare is the share of an expense budget from which it is flagged as warning.
const budgetWarningShare = 0.8

// budgetAverageMonths is how many complete months feed the suggested monthly budget.
const budgetAverageMonths = 3

// SetBudget sets or removes a category's budget. Only income and expense categories qualify.
func (s *Service) SetBudget(ctx context.Context, req SetBudgetRequest) error {
	t, err := s.repo.GetCategoryType(ctx, req.CategoryID)
	if err != nil {
		return err
	}
	if t == txtype.Transfer {
		return ErrBudgetTransfer
	}
	if req.Period == "" {
		req.Period = BudgetPeriodMonthly
	}
	return s.repo.SetBudget(ctx, req)
}

// GetBudgetMonth compares every budget in effect at a month with the actuals: monthly budgets
// against the month, yearly ones against its calendar year.
//
// Actuals roll up the category tree by type, except subtrees budgeted with the other period.
func (s *Service) GetBudgetMonth(ctx context.Context, month time.Time) (BudgetMonth, error) {
	now := time.Now().In(s.loc)
	if month.IsZero() {
		month = now
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.loc)
	end := start.AddDate(0, 1, 0)
	yearStart := time.Date(month.Year(), time.January, 1, 0, 0, 0, 0, s.loc)
	yearEnd := yearStart.AddDate(1, 0, 0)

	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return BudgetMonth{}, err
	}
	budgets, err := s.repo.ListEffectiveBudgets(ctx, start)
	if err != nil {
		return BudgetMonth{}, err
	}
	monthTotals, err := s.repo.GetCategoryTotals(ctx, start, end)
	if err != nil {
		return BudgetMonth{}, err
	}
	yearTotals, err := s.repo.GetCategoryTotals(ctx, yearStart, yearEnd)
	if err != nil {
		return BudgetMonth{}, err
	}
	pastMonths, err := s.repo.GetCategoryTotals(ctx, start.AddDate(0, -budgetAverageMonths, 0), start)
	if err != nil {
		return BudgetMonth{}, err
	}
	prevYear, err := s.repo.GetCategoryTotals(ctx, yearStart.AddDate(-1, 0, 0), yearStart)
	if err != nil {
		return BudgetMonth{}, err
	}

	tree := newCategoryTree(cats)
	byPeriod := map[BudgetPeriod]map[int32]EffectiveBudget{
		BudgetPeriodMonthly: {},
		BudgetPeriodYearly:  {},
	}
	for _, b := range budgets {
		if c, ok := tree.byID[b.CategoryID]; ok && c.Type != txtype.Transfer {
			if set, ok := byPeriod[b.Period]; ok {
				set[b.CategoryID] = b
			}
		}
	}
	monthly, yearly := byPeriod[BudgetPeriodMonthly], byPeriod[BudgetPeriodYearly]

	m := tree.summarize(BudgetPeriodMonthly, monthly, yearly, monthTotals)
	y := tree.summarize(BudgetPeriodYearly, yearly, monthly, yearTotals)

	out := BudgetMonth{
		Month:         start.Format("2006-01"),
		MonthProgress: periodProgress(now, start, end),
		Expense:       m.totals[txtype.Expense],
		Income:        m.totals[txtype.Income],
		Items:         m.items,
		Yearly: BudgetYear{
			Year:         yearStart.Format("2006"),
			YearProgress: periodProgress(now, yearStart, yearEnd),
			Expense:      y.totals[txtype.Expense],
			Income:       y.totals[txtype.Income],
			Items:        y.items,
		},
		Averages:     []BudgetAmount{},
		PreviousYear: []BudgetAmount{},
	}

	// The monthly view reports everything the month took; unbudgeted is what neither period covers.
	out.Expense.Actual, out.Income.Actual = decimal.Zero, decimal.Zero
	covered := tree.covered(monthly, yearly)
	for _, t := range monthTotals {
		bt := totalsFor(&out, t.Type)
		if bt == nil {
			continue
		}
		bt.Actual = bt.Actual.Add(t.Amount)
		if t.CategoryID == nil || !covered[*t.CategoryID] {
			bt.Unbudgeted = bt.Unbudgeted.Add(t.Amount)
		}
	}

	twelve := decimal.NewFromInt(12)
	out.PlannedBalance = out.Income.Budgeted.Sub(out.Expense.Budgeted).
		Add(out.Yearly.Income.Budgeted.Sub(out.Yearly.Expense.Budgeted).Div(twelve)).
		Round(2)

	months := decimal.NewFromInt(budgetAverageMonths)
	pastActual := tree.rollUp(pastMonths, nil)
	prevYearActual := tree.rollUp(prevYear, nil)
	for _, c := range cats {
		if c.Type == txtype.Transfer {
			continue
		}
		if sum := pastActual[c.ID]; sum.IsPositive() {
			out.Averages = append(out.Averages, BudgetAmount{CategoryID: c.ID, Amount: sum.Div(months).Round(2)})
		}
		if sum := prevYearActual[c.ID]; sum.IsPositive() {
			out.PreviousYear = append(out.PreviousYear, BudgetAmount{CategoryID: c.ID, Amount: sum})
		}
	}
	return out, nil
}

// GetBudgetTransactions lists the transactions a budget counts in the period containing month,
// matching what GetBudgetMonth rolls up.
func (s *Service) GetBudgetTransactions(ctx context.Context, categoryID int32, period BudgetPeriod, month time.Time) ([]OverviewTransaction, error) {
	if month.IsZero() {
		month = time.Now().In(s.loc)
	}
	start := period.Start(time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.loc))
	end := period.Next(start)

	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	tree := newCategoryTree(cats)
	c, ok := tree.byID[categoryID]
	if !ok {
		return nil, ErrNotFound
	}
	if c.Type == txtype.Transfer {
		return nil, ErrBudgetTransfer
	}
	budgets, err := s.repo.ListEffectiveBudgets(ctx, time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.loc))
	if err != nil {
		return nil, err
	}
	own, other := map[int32]EffectiveBudget{}, map[int32]EffectiveBudget{}
	for _, b := range budgets {
		if b.Period == period {
			own[b.CategoryID] = b
		} else {
			other[b.CategoryID] = b
		}
	}
	ids := tree.subtree(categoryID, otherPeriodStop(own, other))
	return s.repo.ListCategoryTransactions(ctx, ids, c.Type, start, end)
}

// totalsFor is the monthly totals bucket of a transaction type (nil for transfers).
func totalsFor(out *BudgetMonth, t txtype.Type) *BudgetTotals {
	switch t {
	case txtype.Income:
		return &out.Income
	case txtype.Expense:
		return &out.Expense
	}
	return nil
}

// periodSummary is one period's budget rows and per-type totals.
type periodSummary struct {
	items  []BudgetItem
	totals map[txtype.Type]BudgetTotals
}

// summarize builds one period's rows and totals, leaving out subtrees budgeted in other.
func (t categoryTree) summarize(period BudgetPeriod, own, other map[int32]EffectiveBudget, totals []CategoryTotal) periodSummary {
	actual := t.rollUp(totals, otherPeriodStop(own, other))
	out := periodSummary{items: []BudgetItem{}, totals: map[txtype.Type]BudgetTotals{}}

	// Depth-first in category order, nesting budgeted children under budgeted parents. Each call
	// returns its subtree's overspend, so a euro over several nested budgets counts once.
	var walk func(id int32, depth map[txtype.Type]int) decimal.Decimal
	walk = func(id int32, depth map[txtype.Type]int) decimal.Decimal {
		c := t.byID[id]
		next := depth
		b, budgeted := own[id]
		if budgeted {
			d := depth[c.Type]
			out.items = append(out.items, budgetItem(period, c, b, actual[id], d))
			if d == 0 {
				bt := out.totals[c.Type]
				bt.Budgeted = bt.Budgeted.Add(b.Amount)
				bt.Actual = bt.Actual.Add(actual[id])
				out.totals[c.Type] = bt
			}
			next = make(map[txtype.Type]int, len(depth)+1)
			for k, v := range depth {
				next[k] = v
			}
			next[c.Type] = d + 1
		}
		over := decimal.Zero
		for _, child := range t.children[id] {
			over = over.Add(walk(child, next))
		}
		if budgeted && c.Type == txtype.Expense {
			over = decimal.Max(over, actual[id].Sub(b.Amount))
		}
		return over
	}
	overspent := decimal.Zero
	for _, root := range t.roots {
		overspent = overspent.Add(walk(root, map[txtype.Type]int{}))
	}
	exp := out.totals[txtype.Expense]
	exp.Overspent = overspent
	out.totals[txtype.Expense] = exp
	return out
}

// otherPeriodStop stops the roll-up at subtrees owned by a budget of the other period only.
func otherPeriodStop(own, other map[int32]EffectiveBudget) func(id int32) bool {
	return func(id int32) bool {
		_, mine := own[id]
		_, theirs := other[id]
		return theirs && !mine
	}
}

func budgetItem(period BudgetPeriod, c Category, b EffectiveBudget, actual decimal.Decimal, depth int) BudgetItem {
	item := BudgetItem{
		CategoryID: c.ID,
		Period:     period,
		Name:       c.Name,
		ParentID:   c.ParentID,
		Type:       c.Type,
		Depth:      depth,
		Budget:     b.Amount,
		Actual:     actual,
		Remaining:  b.Amount.Sub(actual),
		Since:      period.Label(b.Since),
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

// periodProgress is the elapsed share of [start, end) at now, clamped to [0, 1].
func periodProgress(now, start, end time.Time) float64 {
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

// rollUp sums each category's totals with its same-type descendants, not descending where stop
// returns true. Uncategorized totals are ignored.
func (t categoryTree) rollUp(totals []CategoryTotal, stop func(id int32) bool) map[int32]decimal.Decimal {
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
			if stop != nil && stop(child) {
				continue
			}
			total = total.Add(sum(child, typ, seen))
		}
		return total
	}
	for id, c := range t.byID {
		out[id] = sum(id, c.Type, map[int32]bool{})
	}
	return out
}

// subtree lists a category and its same-type descendants, not descending where stop returns true.
func (t categoryTree) subtree(id int32, stop func(id int32) bool) []int32 {
	typ := t.byID[id].Type
	out := []int32{}
	seen := map[int32]bool{}
	var walk func(id int32)
	walk = func(id int32) {
		if seen[id] {
			return
		}
		seen[id] = true
		if t.byID[id].Type == typ {
			out = append(out, id)
		}
		for _, child := range t.children[id] {
			if !stop(child) {
				walk(child)
			}
		}
	}
	walk(id)
	return out
}

// covered reports, per category, whether it or a same-type ancestor is budgeted in any set.
func (t categoryTree) covered(sets ...map[int32]EffectiveBudget) map[int32]bool {
	out := make(map[int32]bool, len(t.byID))
	var walk func(id int32, inherited map[txtype.Type]bool)
	walk = func(id int32, inherited map[txtype.Type]bool) {
		if _, done := out[id]; done {
			return
		}
		c := t.byID[id]
		here := inherited[c.Type]
		for _, set := range sets {
			if _, ok := set[id]; ok {
				here = true
			}
		}
		out[id] = here
		next := inherited
		if here != inherited[c.Type] {
			next = make(map[txtype.Type]bool, len(inherited)+1)
			for k, v := range inherited {
				next[k] = v
			}
			next[c.Type] = here
		}
		for _, child := range t.children[id] {
			walk(child, next)
		}
	}
	for _, root := range t.roots {
		walk(root, map[txtype.Type]bool{})
	}
	return out
}
