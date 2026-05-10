package finance

import (
	"context"
	"time"

	"gv-api/internal/finance/txtype"
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

func (s *Service) GetMonthlyStats(ctx context.Context, q MonthlyStatsQuery) ([]MonthlyStat, error) {
	from, to, _, err := s.normalizeStatsRange(ctx, q.From, q.To, GranularityMonth)
	if err != nil {
		return nil, err
	}
	q.From, q.To = from, to
	return s.repo.GetMonthlyStats(ctx, q)
}

// normalizeStatsRange fills missing to with now and missing from with the
// earliest transaction date (or now - 6 months if there are no transactions),
// and validates granularity (defaults to day).
func (s *Service) normalizeStatsRange(ctx context.Context, from, to time.Time, g StatsGranularity) (time.Time, time.Time, StatsGranularity, error) {
	now := time.Now().In(s.loc)
	if to.IsZero() {
		to = now
	}
	if from.IsZero() {
		earliest, ok, err := s.repo.GetEarliestTransactionDate(ctx)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
		if ok {
			from = earliest.In(s.loc)
		} else {
			from = now.AddDate(0, -6, 0)
		}
	}
	if !g.Valid() {
		g = GranularityDay
	}
	return from, to, g, nil
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
