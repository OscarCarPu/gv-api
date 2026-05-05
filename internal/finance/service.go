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
	last30 := now.Add(-30 * 24 * time.Hour)

	accountsTotal, err := s.repo.GetAccountsTotal(ctx)
	if err != nil {
		return Overview{}, err
	}
	income, expense, err := s.repo.GetMonthlyTotals(ctx, monthStart)
	if err != nil {
		return Overview{}, err
	}
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
		Recent: recent,
	}, nil
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
