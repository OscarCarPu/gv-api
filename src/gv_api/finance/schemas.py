from dataclasses import dataclass
from datetime import date
from decimal import Decimal

from gv_api.core.encoding import ApiModel, Money, Timestamp
from gv_api.core.http import Body, Int32, InTime

TX_TYPES = ("income", "expense", "transfer")
ZERO = Decimal(0)

# Responses


class Account(ApiModel):
    id: int
    name: str
    total: Money
    created_at: Timestamp


class Category(ApiModel):
    id: int
    name: str
    parent_id: int | None = None
    type: str
    created_at: Timestamp


class Transaction(ApiModel):
    id: int
    type: str
    amount: Money
    account_id: int
    to_account_id: int | None = None
    category_id: int | None = None
    description: str | None = None
    occurred_at: Timestamp
    created_at: Timestamp


class OverviewMonth(ApiModel):
    income: Money
    expense: Money
    balance: Money


class OverviewTransaction(ApiModel):
    id: int
    type: str
    amount: Money
    account_name: str
    to_account_name: str | None = None
    category_name: str | None = None
    description: str | None = None
    occurred_at: Timestamp


class Overview(ApiModel):
    accounts_total: Money
    month: OverviewMonth
    previous_month: OverviewMonth
    recent_transactions: list[OverviewTransaction] = []


class NetWorthPoint(ApiModel):
    date: str
    total: Money


class CategoryStat(ApiModel):
    category_id: int | None = None
    name: str
    amount: Money
    share: float
    tx_count: int


class MonthlyStat(ApiModel):
    month: str
    income: Money
    expense: Money
    balance: Money


class EstimationPoint(ApiModel):
    date: str
    total: Money
    estimated: bool


class EstimationResult(ApiModel):
    points: list[EstimationPoint] = []
    rate: Money  # monthly compound rate in percent (mode rate), else 0
    saving: Money  # average monthly change (mode saving), else 0


class BudgetTotals(ApiModel):
    # The outermost budgeted categories only, so nesting does not count twice.
    budgeted: Money = ZERO
    actual: Money = ZERO
    # The part of actual no budget covers, uncategorized included.
    unbudgeted: Money = ZERO
    # What went over the budgets (expenses): a nested budget's excess counts once.
    overspent: Money = ZERO


class BudgetItem(ApiModel):
    category_id: int
    period: str
    name: str
    parent_id: int | None = None
    type: str
    depth: int
    budget: Money
    actual: Money
    remaining: Money
    progress: float
    status: str
    since: str


class BudgetAmount(ApiModel):
    category_id: int
    amount: Money


class BudgetYear(ApiModel):
    year: str
    year_progress: float
    expense: BudgetTotals
    income: BudgetTotals
    items: list[BudgetItem] = []


class BudgetMonth(ApiModel):
    month: str
    month_progress: float  # elapsed share of the month: 1 when past, 0 when future
    expense: BudgetTotals
    income: BudgetTotals
    items: list[BudgetItem] = []
    # Budgeted income minus budgeted expenses, plus a twelfth of the yearly net.
    planned_balance: Money
    yearly: BudgetYear
    # The last 3 complete months' average per category, to suggest monthly budgets.
    averages: list[BudgetAmount] = []
    # The previous calendar year's total per category, to suggest yearly budgets.
    previous_year: list[BudgetAmount] = []


@dataclass
class EffectiveBudget:
    category_id: int
    period: str
    since: date
    amount: Decimal


@dataclass
class CategoryTotal:
    category_id: int | None  # None: uncategorized
    type: str
    amount: Decimal


# Requests


class AccountRequest(Body):
    name: str = ""


class CategoryRequest(Body):
    name: str = ""
    parent_id: Int32 | None = None
    type: str = ""


class TransactionRequest(Body):
    type: str = ""
    amount: Decimal = ZERO
    account_id: Int32 = 0
    to_account_id: Int32 | None = None
    category_id: Int32 | None = None
    description: str | None = None
    occurred_at: InTime | None = None  # optional on create (now), required on update


class BudgetRequest(Body):
    month: str = ""
    amount: Decimal = ZERO
    scope: str = ""
    period: str = ""
