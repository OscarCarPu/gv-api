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
	PreviousMonth OverviewMonth         `json:"previous_month"`
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

// --- Stats ---

type StatsGranularity string

const (
	GranularityDay   StatsGranularity = "day"
	GranularityWeek  StatsGranularity = "week"
	GranularityMonth StatsGranularity = "month"
)

func (g StatsGranularity) Valid() bool {
	switch g {
	case GranularityDay, GranularityWeek, GranularityMonth:
		return true
	}
	return false
}

type NetWorthPoint struct {
	Date  string          `json:"date"`
	Total decimal.Decimal `json:"total"`
}

type CategoryStat struct {
	CategoryID *int32          `json:"category_id"`
	Name       string          `json:"name"`
	Amount     decimal.Decimal `json:"amount"`
	Share      float64         `json:"share"`
	TxCount    int64           `json:"tx_count"`
}

type MonthlyStat struct {
	Month   string          `json:"month"`
	Income  decimal.Decimal `json:"income"`
	Expense decimal.Decimal `json:"expense"`
	Balance decimal.Decimal `json:"balance"`
}

type NetWorthQuery struct {
	From        time.Time
	To          time.Time
	Granularity StatsGranularity
}

type ListTransactionsQuery struct {
	AccountID  *int32
	CategoryID *int32
	Type       *txtype.Type
	From       time.Time
	To         time.Time
}

type CategoryStatsQuery struct {
	Type      txtype.Type
	From      time.Time
	To        time.Time
	AccountID *int32
}

type MonthlyStatsQuery struct {
	From       time.Time
	To         time.Time
	AccountID  *int32
	CategoryID *int32
}

type EstimationMode string

const (
	EstimationModeRate   EstimationMode = "rate"
	EstimationModeSaving EstimationMode = "saving"
)

func (m EstimationMode) Valid() bool {
	switch m {
	case EstimationModeRate, EstimationModeSaving:
		return true
	}
	return false
}

type EstimationQuery struct {
	StartMonth time.Time
	EndMonth   time.Time
	Mode       EstimationMode
}

type EstimationPoint struct {
	Date      string          `json:"date"`
	Total     decimal.Decimal `json:"total"`
	Estimated bool            `json:"estimated"`
}

type EstimationResult struct {
	Points []EstimationPoint `json:"points"`
	// Rate is the monthly compound rate (percent) when Mode=rate, otherwise 0.
	Rate decimal.Decimal `json:"rate"`
	// Saving is the average monthly delta (currency) when Mode=saving, otherwise 0.
	Saving decimal.Decimal `json:"saving"`
}
