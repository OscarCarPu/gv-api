from datetime import date, datetime, time, timedelta
from decimal import ROUND_HALF_UP, Decimal
from typing import Any, Protocol
from zoneinfo import ZoneInfo

from gv_api.core.dates import Clock, add_months, utcnow
from gv_api.finance import budgets
from gv_api.finance.errors import (
    BudgetTransferError,
    CategoryMismatchError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import (
    ZERO,
    Account,
    BudgetAmount,
    BudgetMonth,
    BudgetYear,
    Category,
    CategoryStat,
    CategoryTotal,
    EffectiveBudget,
    EstimationPoint,
    EstimationResult,
    MonthlyStat,
    NetWorthPoint,
    Overview,
    OverviewMonth,
    OverviewTransaction,
    Transaction,
    TransactionRequest,
)

CENT = Decimal("0.01")
MICROSECOND = timedelta(microseconds=1)


class Repository(Protocol):
    async def get_account(self, id: int) -> Account: ...
    async def list_accounts(self) -> list[Account]: ...
    async def create_account(self, name: str) -> Account: ...
    async def update_account(self, id: int, name: str) -> Account: ...
    async def delete_account(self, id: int) -> None: ...
    async def get_category(self, id: int) -> Category: ...
    async def get_category_type(self, id: int) -> str: ...
    async def list_categories(self) -> list[Category]: ...
    async def create_category(self, name: str, parent_id: int | None, type: str) -> Category: ...
    async def update_category(
        self, id: int, name: str, parent_id: int | None, type: str
    ) -> Category: ...
    async def delete_category(self, id: int) -> None: ...
    async def get_transaction(self, id: int) -> Transaction: ...
    async def list_transactions(
        self,
        account_id: int | None,
        category_id: int | None,
        type: str | None,
        start: datetime | None,
        end: datetime | None,
    ) -> list[Transaction]: ...
    async def create_transaction(self, values: dict[str, Any]) -> Transaction: ...
    async def update_transaction(self, id: int, values: dict[str, Any]) -> Transaction: ...
    async def delete_transaction(self, id: int) -> None: ...
    async def accounts_total(self) -> Decimal: ...
    async def month_totals(self, start: datetime, end: datetime) -> tuple[Decimal, Decimal]: ...
    async def recent_transactions(self, since: datetime) -> list[OverviewTransaction]: ...
    async def earliest_transaction(self) -> datetime | None: ...
    async def net_worth_series(
        self, granularity: str, start: datetime, end: datetime
    ) -> list[NetWorthPoint]: ...
    async def category_stats(
        self, type: str, start: datetime, end: datetime, account_id: int | None
    ) -> list[CategoryStat]: ...
    async def monthly_stats(
        self, start: datetime, end: datetime, account_id: int | None, category_id: int | None
    ) -> list[MonthlyStat]: ...
    async def set_budget(
        self,
        category_id: int,
        period: str,
        start: date,
        next: date,
        amount: Decimal | None,
        scope: str,
    ) -> None: ...
    async def effective_budgets(self, month: date) -> list[EffectiveBudget]: ...
    async def category_totals(self, start: datetime, end: datetime) -> list[CategoryTotal]: ...
    async def category_transactions(
        self, category_ids: list[int], type: str, start: datetime, end: datetime
    ) -> list[OverviewTransaction]: ...


def cents(d: Decimal) -> Decimal:
    return d.quantize(CENT, ROUND_HALF_UP)


def _transaction_values(req: TransactionRequest) -> dict[str, Any]:
    return {
        "type": req.type,
        "amount": req.amount,
        "account_id": req.account_id,
        "to_account_id": req.to_account_id,
        "category_id": req.category_id,
        "description": req.description,
        "occurred_at": req.occurred_at,
    }


class FinanceService:
    def __init__(self, repo: Repository, loc: ZoneInfo, now: Clock = utcnow) -> None:
        self._repo = repo
        self._loc = loc
        self._now = now

    def _local_now(self) -> datetime:
        return self._now().astimezone(self._loc)

    def _midnight(self, d: date) -> datetime:
        return datetime.combine(d, time(), self._loc)

    # Accounts and categories

    async def get_account(self, id: int) -> Account:
        return await self._repo.get_account(id)

    async def list_accounts(self) -> list[Account]:
        return await self._repo.list_accounts()

    async def create_account(self, name: str) -> Account:
        return await self._repo.create_account(name)

    async def update_account(self, id: int, name: str) -> Account:
        return await self._repo.update_account(id, name)

    async def delete_account(self, id: int) -> None:
        await self._repo.delete_account(id)

    async def get_category(self, id: int) -> Category:
        return await self._repo.get_category(id)

    async def list_categories(self) -> list[Category]:
        return await self._repo.list_categories()

    async def create_category(self, name: str, parent_id: int | None, type: str) -> Category:
        return await self._repo.create_category(name, parent_id, type)

    async def update_category(
        self, id: int, name: str, parent_id: int | None, type: str
    ) -> Category:
        return await self._repo.update_category(id, name, parent_id, type)

    async def delete_category(self, id: int) -> None:
        await self._repo.delete_category(id)

    # Transactions

    async def get_transaction(self, id: int) -> Transaction:
        return await self._repo.get_transaction(id)

    async def list_transactions(
        self,
        account_id: int | None,
        category_id: int | None,
        type: str | None,
        start: datetime | None,
        end: datetime | None,
    ) -> list[Transaction]:
        return await self._repo.list_transactions(account_id, category_id, type, start, end)

    async def create_transaction(self, req: TransactionRequest) -> Transaction:
        await self._check_category(req.category_id, req.type)
        return await self._repo.create_transaction(_transaction_values(req))

    async def update_transaction(self, id: int, req: TransactionRequest) -> Transaction:
        await self._check_category(req.category_id, req.type)
        return await self._repo.update_transaction(id, _transaction_values(req))

    async def delete_transaction(self, id: int) -> None:
        await self._repo.delete_transaction(id)

    async def _check_category(self, category_id: int | None, type: str) -> None:
        """An optional category must exist and have the transaction's type."""
        if category_id is None:
            return
        try:
            category_type = await self._repo.get_category_type(category_id)
        except NotFoundError as err:
            raise InvalidInputError from err
        if category_type != type:
            raise CategoryMismatchError

    # Overview and stats

    async def get_overview(self) -> Overview:
        """Total balance, this and last month's income, expense and balance, and the last 30
        days of transactions."""
        now = self._local_now()
        month = now.date().replace(day=1)
        start, prev = self._midnight(month), self._midnight(add_months(month, -1))
        end = self._midnight(add_months(month, 1))
        total = await self._repo.accounts_total()
        income, expense = await self._repo.month_totals(start, end)
        prev_income, prev_expense = await self._repo.month_totals(prev, start)
        recent = await self._repo.recent_transactions(self._now() - timedelta(days=30))
        return Overview(
            accounts_total=total,
            month=OverviewMonth(income=income, expense=expense, balance=income - expense),
            previous_month=OverviewMonth(
                income=prev_income, expense=prev_expense, balance=prev_income - prev_expense
            ),
            recent_transactions=recent,
        )

    async def _stats_range(
        self, start: datetime | None, end: datetime | None, granularity: str
    ) -> tuple[datetime, datetime]:
        """end defaults to now, start to one period before the earliest transaction (or six
        months back when there is none)."""
        now = self._local_now()
        if start is None:
            earliest = await self._repo.earliest_transaction()
            if earliest is None:
                start = _shift_months(now, -6)
            else:
                earliest = earliest.astimezone(self._loc)
                if granularity == "month":
                    start = _shift_months(earliest, -1)
                else:
                    start = earliest - timedelta(days=7 if granularity == "week" else 1)
        return start, end or now

    async def net_worth(
        self, start: datetime | None, end: datetime | None, granularity: str
    ) -> list[NetWorthPoint]:
        granularity = granularity or "day"
        first, last = await self._stats_range(start, end, granularity)
        return await self._repo.net_worth_series(granularity, first, last)

    async def category_stats(
        self, type: str, start: datetime | None, end: datetime | None, account_id: int | None
    ) -> list[CategoryStat]:
        first, last = await self._stats_range(start, end, "day")
        return await self._repo.category_stats(type, first, last, account_id)

    async def monthly_stats(
        self,
        start: datetime | None,
        end: datetime | None,
        account_id: int | None,
        category_id: int | None,
    ) -> list[MonthlyStat]:
        first, last = await self._stats_range(start, end, "month")
        return await self._repo.monthly_stats(first, last, account_id, category_id)

    async def estimation(self, start_month: date, end_month: date, mode: str) -> EstimationResult:
        """Actual monthly net worth up to the last complete month, then projected to end_month
        with the compound monthly rate (mode rate) or the average monthly change (saving)."""
        now = self._local_now()
        current = self._midnight(now.date().replace(day=1))
        last_complete_end = current - MICROSECOND
        start = self._midnight(start_month.replace(day=1))
        end = self._midnight(end_month.replace(day=1))

        # From the earliest transaction's month, so flat months before any data do not flatten
        # the rate.
        earliest = await self._repo.earliest_transaction()
        if earliest is not None:
            first = self._midnight(earliest.astimezone(self._loc).date().replace(day=1))
            start = max(start, first)

        points: list[EstimationPoint] = []
        if start <= last_complete_end:
            actual_to = start - MICROSECOND if start > current else last_complete_end
            series = await self._repo.net_worth_series("month", start, actual_to)
            points = [EstimationPoint(date=p.date, total=p.total, estimated=False) for p in series]

        rate = saving = ZERO
        first_total = points[0].total if points else ZERO
        last_total = points[-1].total if points else ZERO
        n = len(points) - 1
        if n > 0 and mode == "rate" and first_total > 0 and last_total > 0:
            r = (float(last_total) / float(first_total)) ** (1 / n) - 1
            rate = Decimal(repr(r * 100)).quantize(Decimal("0.0001"), ROUND_HALF_UP)
        elif n > 0 and mode == "saving":
            saving = cents((last_total - first_total) / n)

        factor = 1 + rate / 100
        month = max(current, start)
        while month <= end:
            last_total = last_total * factor if mode == "rate" else last_total + saving
            points.append(
                EstimationPoint(
                    date=month.date().isoformat(), total=cents(last_total), estimated=True
                )
            )
            month = self._midnight(add_months(month.date(), 1))
        return EstimationResult(points=points, rate=rate, saving=saving)

    # Budgets

    async def set_budget(
        self,
        category_id: int,
        period: str,
        month: date,
        amount: Decimal | None,
        scope: str,
    ) -> None:
        """Set (amount) or remove (None) a category's budget for the period containing month.
        Only income and expense categories qualify."""
        if await self._repo.get_category_type(category_id) == "transfer":
            raise BudgetTransferError
        period = period or "monthly"
        start = budgets.period_start(period, month)
        await self._repo.set_budget(
            category_id, period, start, budgets.next_period(period, start), amount, scope
        )

    async def budget_month(self, month: date | None) -> BudgetMonth:
        """Every budget in effect at a month against the actuals: monthly budgets against the
        month, yearly ones against its calendar year."""
        now = self._local_now()
        first = (month or now.date()).replace(day=1)
        start, end = self._midnight(first), self._midnight(add_months(first, 1))
        year = date(first.year, 1, 1)
        year_start, year_end = self._midnight(year), self._midnight(date(first.year + 1, 1, 1))

        cats = await self._repo.list_categories()
        effective = await self._repo.effective_budgets(first)
        month_totals = await self._repo.category_totals(start, end)
        year_totals = await self._repo.category_totals(year_start, year_end)
        past = await self._repo.category_totals(
            self._midnight(add_months(first, -budgets.AVERAGE_MONTHS)), start
        )
        prev_year = await self._repo.category_totals(
            self._midnight(date(first.year - 1, 1, 1)), year_start
        )

        tree = budgets.CategoryTree(cats)
        by_period: dict[str, budgets.Budgets] = {"monthly": {}, "yearly": {}}
        for b in effective:
            c = tree.by_id.get(b.category_id)
            if c is not None and c.type != "transfer" and b.period in by_period:
                by_period[b.period][b.category_id] = b
        monthly, yearly = by_period["monthly"], by_period["yearly"]
        m = tree.summarize("monthly", monthly, yearly, month_totals)
        y = tree.summarize("yearly", yearly, monthly, year_totals)

        # The month reports everything it took; unbudgeted is what neither period covers.
        totals = m.totals
        for t in totals.values():
            t.actual = ZERO
        covered = tree.covered(monthly, yearly)
        for row in month_totals:
            if (bt := totals.get(row.type)) is not None:
                bt.actual += row.amount
                if row.category_id not in covered:
                    bt.unbudgeted += row.amount

        planned = (
            totals["income"].budgeted
            - totals["expense"].budgeted
            + (y.totals["income"].budgeted - y.totals["expense"].budgeted) / 12
        )
        past_actual = tree.roll_up(past)
        prev_year_actual = tree.roll_up(prev_year)
        averages, previous = [], []
        for c in cats:
            if c.type == "transfer":
                continue
            if (s := past_actual[c.id]) > 0:
                averages.append(
                    BudgetAmount(category_id=c.id, amount=cents(s / budgets.AVERAGE_MONTHS))
                )
            if (s := prev_year_actual[c.id]) > 0:
                previous.append(BudgetAmount(category_id=c.id, amount=s))

        return BudgetMonth(
            month=first.strftime("%Y-%m"),
            month_progress=budgets.progress(now, start, end),
            expense=totals["expense"],
            income=totals["income"],
            items=m.items,
            planned_balance=cents(planned),
            yearly=BudgetYear(
                year=str(first.year),
                year_progress=budgets.progress(now, year_start, year_end),
                expense=y.totals["expense"],
                income=y.totals["income"],
                items=y.items,
            ),
            averages=averages,
            previous_year=previous,
        )

    async def budget_transactions(
        self, category_id: int, period: str, month: date | None
    ) -> list[OverviewTransaction]:
        """What a budget counts in the period containing month, as budget_month rolls it up."""
        first = (month or self._local_now().date()).replace(day=1)
        start = budgets.period_start(period, first)
        end = budgets.next_period(period, start)

        tree = budgets.CategoryTree(await self._repo.list_categories())
        c = tree.by_id.get(category_id)
        if c is None:
            raise NotFoundError
        if c.type == "transfer":
            raise BudgetTransferError
        own: budgets.Budgets = {}
        other: budgets.Budgets = {}
        for b in await self._repo.effective_budgets(first):
            (own if b.period == period else other)[b.category_id] = b
        ids = tree.subtree(category_id, budgets.other_period_stop(own, other))
        return await self._repo.category_transactions(
            ids, c.type, self._midnight(start), self._midnight(end)
        )


def _shift_months(t: datetime, months: int) -> datetime:
    """t moved by whole months, clamped to the end of a shorter month."""
    return datetime.combine(add_months(t.date(), months), t.timetz())
