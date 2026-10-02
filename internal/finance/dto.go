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

// BudgetPeriod is what a budget is measured against: a calendar month or a calendar year.
type BudgetPeriod string

const (
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodYearly  BudgetPeriod = "yearly"
)

func (p BudgetPeriod) Valid() bool {
	return p == BudgetPeriodMonthly || p == BudgetPeriodYearly
}

// Start is the first day of the period containing t, in t's location.
func (p BudgetPeriod) Start(t time.Time) time.Time {
	if p == BudgetPeriodYearly {
		return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// Next is the start of the period after the one starting at start.
func (p BudgetPeriod) Next(start time.Time) time.Time {
	if p == BudgetPeriodYearly {
		return start.AddDate(1, 0, 0)
	}
	return start.AddDate(0, 1, 0)
}

// Label formats a period start the way the API reports it: YYYY-MM or YYYY.
func (p BudgetPeriod) Label(start time.Time) string {
	if p == BudgetPeriodYearly {
		return start.Format("2006")
	}
	return start.Format("2006-01")
}

// BudgetScope says how far a budget change reaches: forward from its period, or that period only.
type BudgetScope string

const (
	BudgetScopeForward BudgetScope = "forward"
	BudgetScopeOnce    BudgetScope = "once"
	BudgetScopeMonth   BudgetScope = "month"
)

func (s BudgetScope) Valid() bool {
	return s == BudgetScopeForward || s == BudgetScopeOnce || s == BudgetScopeMonth
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

// SetBudgetRequest sets (Amount non-nil) or removes (Amount nil) a category's budget for the
// period containing Month.
type SetBudgetRequest struct {
	CategoryID int32
	Period     BudgetPeriod
	Month      time.Time
	Amount     *decimal.Decimal
	Scope      BudgetScope
}

// EffectiveBudget is the budget in effect for a category and period, and the start of its row.
type EffectiveBudget struct {
	CategoryID int32
	Period     BudgetPeriod
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
	// MonthProgress is the elapsed share of the month: 1 for past months, 0 for future ones.
	MonthProgress float64      `json:"month_progress"`
	Expense       BudgetTotals `json:"expense"`
	Income        BudgetTotals `json:"income"`
	Items         []BudgetItem `json:"items"`
	// PlannedBalance is budgeted income − budgeted expenses, plus a twelfth of the yearly net.
	PlannedBalance decimal.Decimal `json:"planned_balance"`
	Yearly         BudgetYear      `json:"yearly"`
	// Averages is the last 3 complete months' average per category, to suggest monthly budgets.
	Averages []BudgetAmount `json:"averages"`
	// PreviousYear is the previous calendar year's total per category, to suggest yearly budgets.
	PreviousYear []BudgetAmount `json:"previous_year"`
}

// BudgetYear is the yearly budgets of the viewed month's year against that year's actuals.
type BudgetYear struct {
	Year         string  `json:"year"`
	YearProgress float64 `json:"year_progress"`
	// Totals of the yearly budgets only; Unbudgeted is always 0 here.
	Expense BudgetTotals `json:"expense"`
	Income  BudgetTotals `json:"income"`
	Items   []BudgetItem `json:"items"`
}

type BudgetTotals struct {
	// Budgeted sums the outermost budgeted categories only, so nesting does not double count.
	Budgeted decimal.Decimal `json:"budgeted"`
	Actual   decimal.Decimal `json:"actual"`
	// Unbudgeted is the part of Actual in categories no budget covers (incl. uncategorized).
	Unbudgeted decimal.Decimal `json:"unbudgeted"`
	// Overspent is what went over the budgets (expenses only). A nested budget's excess is counted
	// once: each budget contributes the larger of its own excess and its children's.
	Overspent decimal.Decimal `json:"overspent"`
}

type BudgetItem struct {
	CategoryID int32           `json:"category_id"`
	Period     BudgetPeriod    `json:"period"`
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
