package finance

import (
	"context"
	"errors"
	"time"

	"gv-api/internal/database/financedb"
	"gv-api/internal/finance/txtype"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrAccountInUse    = errors.New("account has transactions")
	ErrCategoryInUse   = errors.New("category is referenced")
	ErrInvalidInput    = errors.New("invalid input")
	ErrCategoryMismatch = errors.New("category type does not match transaction type")
)

type Repository interface {
	GetAccount(ctx context.Context, id int32) (Account, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, req CreateAccountRequest) (Account, error)
	UpdateAccount(ctx context.Context, req UpdateAccountRequest) (Account, error)
	DeleteAccount(ctx context.Context, id int32) error

	GetCategory(ctx context.Context, id int32) (Category, error)
	GetCategoryType(ctx context.Context, id int32) (txtype.Type, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error)
	UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (Category, error)
	DeleteCategory(ctx context.Context, id int32) error

	GetTransaction(ctx context.Context, id int32) (Transaction, error)
	ListTransactions(ctx context.Context) ([]Transaction, error)
	ListTransactionsByAccount(ctx context.Context, accountID int32) ([]Transaction, error)
	CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error)
	UpdateTransaction(ctx context.Context, req UpdateTransactionRequest) (Transaction, error)
	DeleteTransaction(ctx context.Context, id int32) error

	GetAccountsTotal(ctx context.Context) (decimal.Decimal, error)
	GetMonthlyTotals(ctx context.Context, since time.Time) (decimal.Decimal, decimal.Decimal, error)
	ListRecentTransactions(ctx context.Context, since time.Time) ([]OverviewTransaction, error)
}

type PostgresRepository struct {
	q *financedb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: financedb.New(pool)}
}

func accountToDTO(a financedb.Account) Account {
	return Account{
		ID:        a.ID,
		Name:      a.Name,
		Total:     a.Total,
		CreatedAt: a.CreatedAt.Time,
	}
}

func categoryToDTO(c financedb.Category) Category {
	return Category{
		ID:        c.ID,
		Name:      c.Name,
		ParentID:  c.ParentID,
		Type:      c.Type,
		CreatedAt: c.CreatedAt.Time,
	}
}

func transactionToDTO(t financedb.Transaction) Transaction {
	return Transaction{
		ID:          t.ID,
		Type:        t.Type,
		Amount:      t.Amount,
		AccountID:   t.AccountID,
		ToAccountID: t.ToAccountID,
		CategoryID:  t.CategoryID,
		Description: t.Description,
		OccurredAt:  t.OccurredAt.Time,
		CreatedAt:   t.CreatedAt.Time,
	}
}

// --- Accounts ---

func (r *PostgresRepository) GetAccount(ctx context.Context, id int32) (Account, error) {
	row, err := r.q.GetAccount(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrNotFound
		}
		return Account{}, err
	}
	return accountToDTO(row), nil
}

func (r *PostgresRepository) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := r.q.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(rows))
	for i, row := range rows {
		out[i] = accountToDTO(row)
	}
	return out, nil
}

func (r *PostgresRepository) CreateAccount(ctx context.Context, req CreateAccountRequest) (Account, error) {
	row, err := r.q.CreateAccount(ctx, req.Name)
	if err != nil {
		return Account{}, err
	}
	return accountToDTO(row), nil
}

func (r *PostgresRepository) UpdateAccount(ctx context.Context, req UpdateAccountRequest) (Account, error) {
	row, err := r.q.UpdateAccount(ctx, financedb.UpdateAccountParams{
		ID:   req.ID,
		Name: req.Name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrNotFound
		}
		return Account{}, err
	}
	return accountToDTO(row), nil
}

func (r *PostgresRepository) DeleteAccount(ctx context.Context, id int32) error {
	if err := r.q.DeleteAccount(ctx, id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrAccountInUse
		}
		return err
	}
	return nil
}

// --- Categories ---

func (r *PostgresRepository) GetCategory(ctx context.Context, id int32) (Category, error) {
	row, err := r.q.GetCategory(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Category{}, ErrNotFound
		}
		return Category{}, err
	}
	return categoryToDTO(row), nil
}

func (r *PostgresRepository) GetCategoryType(ctx context.Context, id int32) (txtype.Type, error) {
	t, err := r.q.GetCategoryType(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return t, nil
}

func (r *PostgresRepository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.q.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = categoryToDTO(row)
	}
	return out, nil
}

func (r *PostgresRepository) CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error) {
	row, err := r.q.CreateCategory(ctx, financedb.CreateCategoryParams{
		Name:     req.Name,
		ParentID: req.ParentID,
		Type:     req.Type,
	})
	if err != nil {
		return Category{}, mapCategoryError(err)
	}
	return categoryToDTO(row), nil
}

func (r *PostgresRepository) UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (Category, error) {
	row, err := r.q.UpdateCategory(ctx, financedb.UpdateCategoryParams{
		ID:       req.ID,
		Name:     req.Name,
		ParentID: req.ParentID,
		Type:     req.Type,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Category{}, ErrNotFound
		}
		return Category{}, mapCategoryError(err)
	}
	return categoryToDTO(row), nil
}

func (r *PostgresRepository) DeleteCategory(ctx context.Context, id int32) error {
	if err := r.q.DeleteCategory(ctx, id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrCategoryInUse
		}
		return err
	}
	return nil
}

// mapCategoryError handles the parent_id self-FK violation (parent doesn't exist)
// and the parent_id != id CHECK violation.
func mapCategoryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503": // FK violation: parent_id references non-existent category
			return ErrInvalidInput
		case "23514": // CHECK violation: parent_id == id
			return ErrInvalidInput
		}
	}
	return err
}

// --- Transactions ---

func (r *PostgresRepository) GetTransaction(ctx context.Context, id int32) (Transaction, error) {
	row, err := r.q.GetTransaction(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transaction{}, ErrNotFound
		}
		return Transaction{}, err
	}
	return transactionToDTO(row), nil
}

func (r *PostgresRepository) ListTransactions(ctx context.Context) ([]Transaction, error) {
	rows, err := r.q.ListTransactions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Transaction, len(rows))
	for i, row := range rows {
		out[i] = transactionToDTO(row)
	}
	return out, nil
}

func (r *PostgresRepository) ListTransactionsByAccount(ctx context.Context, accountID int32) ([]Transaction, error) {
	rows, err := r.q.ListTransactionsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]Transaction, len(rows))
	for i, row := range rows {
		out[i] = transactionToDTO(row)
	}
	return out, nil
}

func (r *PostgresRepository) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error) {
	occurredAt := pgtype.Timestamptz{}
	if req.OccurredAt != nil {
		occurredAt = pgtype.Timestamptz{Time: *req.OccurredAt, Valid: true}
	}
	row, err := r.q.CreateTransaction(ctx, financedb.CreateTransactionParams{
		Type:        req.Type,
		Amount:      req.Amount,
		AccountID:   req.AccountID,
		ToAccountID: req.ToAccountID,
		CategoryID:  req.CategoryID,
		Description: req.Description,
		OccurredAt:  occurredAt,
	})
	if err != nil {
		return Transaction{}, mapTxError(err)
	}
	return transactionToDTO(row), nil
}

func (r *PostgresRepository) UpdateTransaction(ctx context.Context, req UpdateTransactionRequest) (Transaction, error) {
	row, err := r.q.UpdateTransaction(ctx, financedb.UpdateTransactionParams{
		ID:          req.ID,
		Type:        req.Type,
		Amount:      req.Amount,
		AccountID:   req.AccountID,
		ToAccountID: req.ToAccountID,
		CategoryID:  req.CategoryID,
		Description: req.Description,
		OccurredAt:  pgtype.Timestamptz{Time: req.OccurredAt, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Transaction{}, ErrNotFound
		}
		return Transaction{}, mapTxError(err)
	}
	return transactionToDTO(row), nil
}

func (r *PostgresRepository) DeleteTransaction(ctx context.Context, id int32) error {
	return r.q.DeleteTransaction(ctx, id)
}

// --- Overview ---

func (r *PostgresRepository) GetAccountsTotal(ctx context.Context) (decimal.Decimal, error) {
	return r.q.GetAccountsTotal(ctx)
}

func (r *PostgresRepository) GetMonthlyTotals(ctx context.Context, since time.Time) (decimal.Decimal, decimal.Decimal, error) {
	row, err := r.q.GetMonthlyTotals(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return row.Income, row.Expense, nil
}

func (r *PostgresRepository) ListRecentTransactions(ctx context.Context, since time.Time) ([]OverviewTransaction, error) {
	rows, err := r.q.ListRecentTransactions(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]OverviewTransaction, len(rows))
	for i, row := range rows {
		out[i] = OverviewTransaction{
			ID:            row.ID,
			Type:          row.Type,
			Amount:        row.Amount,
			AccountName:   row.AccountName,
			ToAccountName: row.ToAccountName,
			CategoryName:  row.CategoryName,
			Description:   row.Description,
			OccurredAt:    row.OccurredAt.Time,
		}
	}
	return out, nil
}

// mapTxError converts FK violations on transactions.account_id /
// to_account_id / category_id (referenced row doesn't exist) into
// ErrInvalidInput so the handler returns 400 instead of 500.
func mapTxError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrInvalidInput
	}
	return err
}
