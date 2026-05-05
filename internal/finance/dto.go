package finance

import (
	"time"

	"gv-api/internal/finance/txtype"

	"github.com/shopspring/decimal"
)

type Account struct {
	ID        int32           `json:"id"`
	Name      string          `json:"name"`
	Total     decimal.Decimal `json:"total"`
	CreatedAt time.Time       `json:"created_at"`
}

type CreateAccountRequest struct {
	Name string `json:"name"`
}

type UpdateAccountRequest struct {
	ID   int32  `json:"-"`
	Name string `json:"name"`
}

type Category struct {
	ID        int32       `json:"id"`
	Name      string      `json:"name"`
	ParentID  *int32      `json:"parent_id"`
	Type      txtype.Type `json:"type"`
	CreatedAt time.Time   `json:"created_at"`
}

type CreateCategoryRequest struct {
	Name     string      `json:"name"`
	ParentID *int32      `json:"parent_id"`
	Type     txtype.Type `json:"type"`
}

type UpdateCategoryRequest struct {
	ID       int32       `json:"-"`
	Name     string      `json:"name"`
	ParentID *int32      `json:"parent_id"`
	Type     txtype.Type `json:"type"`
}

type Transaction struct {
	ID          int32           `json:"id"`
	Type        txtype.Type     `json:"type"`
	Amount      decimal.Decimal `json:"amount"`
	AccountID   int32           `json:"account_id"`
	ToAccountID *int32          `json:"to_account_id"`
	CategoryID  *int32          `json:"category_id"`
	Description *string         `json:"description"`
	OccurredAt  time.Time       `json:"occurred_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

type CreateTransactionRequest struct {
	Type        txtype.Type     `json:"type"`
	Amount      decimal.Decimal `json:"amount"`
	AccountID   int32           `json:"account_id"`
	ToAccountID *int32          `json:"to_account_id"`
	CategoryID  *int32          `json:"category_id"`
	Description *string         `json:"description"`
	OccurredAt  *time.Time      `json:"occurred_at"`
}

type UpdateTransactionRequest struct {
	ID          int32           `json:"-"`
	Type        txtype.Type     `json:"type"`
	Amount      decimal.Decimal `json:"amount"`
	AccountID   int32           `json:"account_id"`
	ToAccountID *int32          `json:"to_account_id"`
	CategoryID  *int32          `json:"category_id"`
	Description *string         `json:"description"`
	OccurredAt  time.Time       `json:"occurred_at"`
}

type Overview struct {
	AccountsTotal decimal.Decimal       `json:"accounts_total"`
	Month         OverviewMonth         `json:"month"`
	Recent        []OverviewTransaction `json:"recent_transactions"`
}

type OverviewMonth struct {
	Income  decimal.Decimal `json:"income"`
	Expense decimal.Decimal `json:"expense"`
	Balance decimal.Decimal `json:"balance"`
}

type OverviewTransaction struct {
	ID            int32           `json:"id"`
	Type          txtype.Type     `json:"type"`
	Amount        decimal.Decimal `json:"amount"`
	AccountName   string          `json:"account_name"`
	ToAccountName *string         `json:"to_account_name"`
	CategoryName  *string         `json:"category_name"`
	Description   *string         `json:"description"`
	OccurredAt    time.Time       `json:"occurred_at"`
}
