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

// --- Budgets ---

// BudgetScope says how far a budget change reaches: from the month on (every later month
// carries it), or that month only (the previous value comes back the month after).
type BudgetScope string

const (
	BudgetScopeForward BudgetScope = "forward"
	BudgetScopeMonth   BudgetScope = "month"
)

func (s BudgetScope) Valid() bool {
	return s == BudgetScopeForward || s == BudgetScopeMonth
}

// BudgetStatus is how a budgeted category is doing: ok / warning / over for expenses,
// pending / met for income.
type BudgetStatus string

const (
	BudgetStatusOK      BudgetStatus = "ok"
	BudgetStatusWarning BudgetStatus = "warning"
	BudgetStatusOver    BudgetStatus = "over"
	BudgetStatusPending BudgetStatus = "pending"
	BudgetStatusMet     BudgetStatus = "met"
)

// SetBudgetRequest sets (Amount non-nil) or removes (Amount nil) a category's budget at
// Month, which is always the first day of the month.
type SetBudgetRequest struct {
	CategoryID int32
	Month      time.Time
	Amount     *decimal.Decimal
	Scope      BudgetScope
}

// EffectiveBudget is the budget in effect for a category at some month and the month that
// row started.
type EffectiveBudget struct {
	CategoryID int32
	Since      time.Time
	Amount     decimal.Decimal
}

// CategoryTotal is the income or expense summed for one category (nil = uncategorized).
type CategoryTotal struct {
	CategoryID *int32
	Type       txtype.Type
	Amount     decimal.Decimal
}

type BudgetMonth struct {
	Month string `json:"month"`
	// MonthProgress is the share of the month already elapsed: 1 for past months, 0 for
	// future ones, and in between for the current one.
	MonthProgress float64        `json:"month_progress"`
	Expense       BudgetTotals   `json:"expense"`
	Income        BudgetTotals   `json:"income"`
	Items         []BudgetItem   `json:"items"`
	Averages      []BudgetAmount `json:"averages"`
}

type BudgetTotals struct {
	// Budgeted sums the outermost budgeted categories only, so a parent and its child being
	// both budgeted does not count twice.
	Budgeted decimal.Decimal `json:"budgeted"`
	Actual   decimal.Decimal `json:"actual"`
	// Unbudgeted is the part of Actual in categories no budget covers (incl. uncategorized).
	Unbudgeted decimal.Decimal `json:"unbudgeted"`
	// Overspent is what went over the budgets (expenses only; always 0 for income). Nested
	// budgets don't double count: a budget contributes the larger of its own excess and the
	// excess of the budgets inside it.
	Overspent decimal.Decimal `json:"overspent"`
}

type BudgetItem struct {
	CategoryID int32           `json:"category_id"`
	Name       string          `json:"name"`
	ParentID   *int32          `json:"parent_id"`
	Type       txtype.Type     `json:"type"`
	Depth      int             `json:"depth"`
	Budget     decimal.Decimal `json:"budget"`
	Actual     decimal.Decimal `json:"actual"`
	Remaining  decimal.Decimal `json:"remaining"`
	Progress   float64         `json:"progress"`
	Status     BudgetStatus    `json:"status"`
	Since      string          `json:"since"`
}

type BudgetAmount struct {
	CategoryID int32           `json:"category_id"`
	Amount     decimal.Decimal `json:"amount"`
}
