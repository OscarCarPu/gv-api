package finance

import (
	"context"
	"math"
	"time"

	"gv-api/internal/finance/txtype"

	"github.com/shopspring/decimal"
)

type Service struct {
	repo Repository
	loc  *time.Location
}

func NewService(repo Repository, loc *time.Location) *Service {
	return &Service{repo: repo, loc: loc}
}

// --- Accounts ---

func (s *Service) GetAccount(ctx context.Context, id int32) (Account, error) {
	return s.repo.GetAccount(ctx, id)
}

func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	return s.repo.ListAccounts(ctx)
}

func (s *Service) CreateAccount(ctx context.Context, req CreateAccountRequest) (Account, error) {
	return s.repo.CreateAccount(ctx, req)
}

func (s *Service) UpdateAccount(ctx context.Context, req UpdateAccountRequest) (Account, error) {
	return s.repo.UpdateAccount(ctx, req)
}

func (s *Service) DeleteAccount(ctx context.Context, id int32) error {
	return s.repo.DeleteAccount(ctx, id)
}

// --- Categories ---

func (s *Service) GetCategory(ctx context.Context, id int32) (Category, error) {
	return s.repo.GetCategory(ctx, id)
}

func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	return s.repo.ListCategories(ctx)
}

func (s *Service) CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error) {
	return s.repo.CreateCategory(ctx, req)
}

func (s *Service) UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (Category, error) {
	return s.repo.UpdateCategory(ctx, req)
}

func (s *Service) DeleteCategory(ctx context.Context, id int32) error {
	return s.repo.DeleteCategory(ctx, id)
}

// --- Transactions ---

func (s *Service) GetTransaction(ctx context.Context, id int32) (Transaction, error) {
	return s.repo.GetTransaction(ctx, id)
}

func (s *Service) ListTransactions(ctx context.Context) ([]Transaction, error) {
	return s.repo.ListTransactions(ctx)
}

func (s *Service) ListTransactionsByAccount(ctx context.Context, accountID int32) ([]Transaction, error) {
	return s.repo.ListTransactionsByAccount(ctx, accountID)
}

func (s *Service) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error) {
	if err := s.assertCategoryMatchesType(ctx, req.CategoryID, req.Type); err != nil {
		return Transaction{}, err
	}
	return s.repo.CreateTransaction(ctx, req)
}

func (s *Service) UpdateTransaction(ctx context.Context, req UpdateTransactionRequest) (Transaction, error) {
	if err := s.assertCategoryMatchesType(ctx, req.CategoryID, req.Type); err != nil {
		return Transaction{}, err
	}
	return s.repo.UpdateTransaction(ctx, req)
}

func (s *Service) DeleteTransaction(ctx context.Context, id int32) error {
	return s.repo.DeleteTransaction(ctx, id)
}

// GetOverview returns the cross-feature summary used by the /finance/overview
// endpoint: total balance across all accounts, this-month income/expense/balance
// in the configured timezone, and the last 30 days of transactions joined with
// account/category names for display.
func (s *Service) GetOverview(ctx context.Context) (Overview, error) {
	now := time.Now().In(s.loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.loc)
	prevMonthStart := monthStart.AddDate(0, -1, 0)
	last30 := now.Add(-30 * 24 * time.Hour)

	accountsTotal, err := s.repo.GetAccountsTotal(ctx)
	if err != nil {
		return Overview{}, err
	}
	income, expense, err := s.repo.GetMonthlyTotals(ctx, monthStart)
	if err != nil {
		return Overview{}, err
	}
	// GetMonthlyTotals only takes a lower bound, so summing from prevMonthStart
	// gives prev-month + current-month combined; subtract current to isolate prev.
	prevPlusCurIncome, prevPlusCurExpense, err := s.repo.GetMonthlyTotals(ctx, prevMonthStart)
	if err != nil {
		return Overview{}, err
	}
	prevIncome := prevPlusCurIncome.Sub(income)
	prevExpense := prevPlusCurExpense.Sub(expense)
	recent, err := s.repo.ListRecentTransactions(ctx, last30)
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		AccountsTotal: accountsTotal,
		Month: OverviewMonth{
			Income:  income,
			Expense: expense,
			Balance: income.Sub(expense),
		},
		PreviousMonth: OverviewMonth{
			Income:  prevIncome,
			Expense: prevExpense,
			Balance: prevIncome.Sub(prevExpense),
		},
		Recent: recent,
	}, nil
}

// --- Stats ---

func (s *Service) GetNetWorthSeries(ctx context.Context, q NetWorthQuery) ([]NetWorthPoint, error) {
	from, to, g, err := s.normalizeStatsRange(ctx, q.From, q.To, q.Granularity)
	if err != nil {
		return nil, err
	}
	q.From, q.To, q.Granularity = from, to, g
	return s.repo.GetNetWorthSeries(ctx, q)
}

func (s *Service) GetCategoryStats(ctx context.Context, q CategoryStatsQuery) ([]CategoryStat, error) {
	from, to, _, err := s.normalizeStatsRange(ctx, q.From, q.To, GranularityDay)
	if err != nil {
		return nil, err
	}
	q.From, q.To = from, to
	return s.repo.GetCategoryStats(ctx, q)
}

// GetEstimation returns a monthly series from start_month through end_month.
// Actual points cover start_month..lastCompletedMonth (end of previous month
// relative to now). The projection factor is derived from those actuals:
// - rate: compound monthly rate r such that last = first * (1+r)^n
// - saving: average monthly delta (last - first) / n
// Estimated points are then projected forward from the last actual total to
// end_month using that factor.
func (s *Service) GetEstimation(ctx context.Context, q EstimationQuery) (EstimationResult, error) {
	now := time.Now().In(s.loc)
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.loc)
	lastCompletedEnd := currentMonthStart.Add(-time.Nanosecond)

	startMonth := time.Date(q.StartMonth.Year(), q.StartMonth.Month(), 1, 0, 0, 0, 0, s.loc)
	endMonth := time.Date(q.EndMonth.Year(), q.EndMonth.Month(), 1, 0, 0, 0, 0, s.loc)

	// Clamp startMonth to the month of the earliest transaction so the actuals
	// series doesn't include flat pre-data buckets that would otherwise pin
	// firstTotal to the current accounts.total and yield a near-zero rate.
	// Mirrors the "missing from → earliest tx date" rule in normalizeStatsRange,
	// applied as a lower bound rather than a default.
	earliest, hasTx, err := s.repo.GetEarliestTransactionDate(ctx)
	if err != nil {
		return EstimationResult{}, err
	}
	if hasTx {
		e := earliest.In(s.loc)
		earliestMonth := time.Date(e.Year(), e.Month(), 1, 0, 0, 0, 0, s.loc)
		if startMonth.Before(earliestMonth) {
			startMonth = earliestMonth
		}
	}

	actualTo := lastCompletedEnd
	if startMonth.After(currentMonthStart) {
		actualTo = startMonth.Add(-time.Nanosecond)
	}

	points := []EstimationPoint{}
	var firstTotal, lastTotal decimal.Decimal
	haveLast := false

	if !startMonth.After(lastCompletedEnd) {
		actualPts, err := s.repo.GetNetWorthSeries(ctx, NetWorthQuery{
			From:        startMonth,
			To:          actualTo,
			Granularity: GranularityMonth,
		})
		if err != nil {
			return EstimationResult{}, err
		}
		for i, p := range actualPts {
			points = append(points, EstimationPoint{Date: p.Date, Total: p.Total, Estimated: false})
			if i == 0 {
				firstTotal = p.Total
			}
			lastTotal = p.Total
			haveLast = true
		}
	}

	// Derive projection factor from the actuals. n = number of monthly steps.
	n := len(points) - 1
	var rate, saving decimal.Decimal
	if n > 0 {
		switch q.Mode {
		case EstimationModeRate:
			first, _ := firstTotal.Float64()
			last, _ := lastTotal.Float64()
			if first > 0 && last > 0 {
				r := math.Pow(last/first, 1.0/float64(n)) - 1
				rate = decimal.NewFromFloat(r * 100).Round(4)
			}
		case EstimationModeSaving:
			diff := lastTotal.Sub(firstTotal)
			saving = diff.Div(decimal.NewFromInt(int64(n))).Round(2)
		}
	}

	if !haveLast {
		lastTotal = decimal.Zero
	}

	projStart := currentMonthStart
	if startMonth.After(projStart) {
		projStart = startMonth
	}
	hundred := decimal.NewFromInt(100)
	rateFactor := decimal.NewFromInt(1).Add(rate.Div(hundred))
	for m := projStart; !m.After(endMonth); m = m.AddDate(0, 1, 0) {
		switch q.Mode {
		case EstimationModeRate:
			lastTotal = lastTotal.Mul(rateFactor)
		case EstimationModeSaving:
			lastTotal = lastTotal.Add(saving)
		}
		points = append(points, EstimationPoint{
			Date:      m.Format("2006-01-02"),
			Total:     lastTotal.Round(2),
			Estimated: true,
		})
	}

	return EstimationResult{Points: points, Rate: rate, Saving: saving}, nil
}

func (s *Service) GetMonthlyStats(ctx context.Context, q MonthlyStatsQuery) ([]MonthlyStat, error) {
	from, to, _, err := s.normalizeStatsRange(ctx, q.From, q.To, GranularityMonth)
	if err != nil {
		return nil, err
	}
	q.From, q.To = from, to
	return s.repo.GetMonthlyStats(ctx, q)
}

// normalizeStatsRange fills missing to with now and missing from with one
// granularity period before the earliest transaction date (so the net-worth
// baseline before the first tx is visible), or now - 6 months if there are no
// transactions. Validates granularity (defaults to day).
func (s *Service) normalizeStatsRange(ctx context.Context, from, to time.Time, g StatsGranularity) (time.Time, time.Time, StatsGranularity, error) {
	now := time.Now().In(s.loc)
	if to.IsZero() {
		to = now
	}
	if !g.Valid() {
		g = GranularityDay
	}
	if from.IsZero() {
		earliest, ok, err := s.repo.GetEarliestTransactionDate(ctx)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
		if ok {
			from = shiftBackOnePeriod(earliest.In(s.loc), g)
		} else {
			from = now.AddDate(0, -6, 0)
		}
	}
	return from, to, g, nil
}

func shiftBackOnePeriod(t time.Time, g StatsGranularity) time.Time {
	switch g {
	case GranularityWeek:
		return t.AddDate(0, 0, -7)
	case GranularityMonth:
		return t.AddDate(0, -1, 0)
	default:
		return t.AddDate(0, 0, -1)
	}
}

// assertCategoryMatchesType returns ErrCategoryMismatch if the category's
// type doesn't match the transaction type, ErrInvalidInput if the category
// doesn't exist, or nil if categoryID is nil (categories are optional).
func (s *Service) assertCategoryMatchesType(ctx context.Context, categoryID *int32, t txtype.Type) error {
	if categoryID == nil {
		return nil
	}
	catType, err := s.repo.GetCategoryType(ctx, *categoryID)
	if err != nil {
		if err == ErrNotFound {
			return ErrInvalidInput
		}
		return err
	}
	if catType != t {
		return ErrCategoryMismatch
	}
	return nil
}
