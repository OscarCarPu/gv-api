from datetime import date, datetime
from decimal import Decimal
from typing import Any

from sqlalchemy import Select, delete, func, or_, select, text, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.finance.errors import (
    AccountInUseError,
    CategoryInUseError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import (
    Account,
    Category,
    CategoryStat,
    CategoryTotal,
    EffectiveBudget,
    MonthlyStat,
    NetWorthPoint,
    OverviewTransaction,
    Transaction,
)
from gv_api.finance.tables import accounts, categories, transactions

FK_VIOLATION = "23503"
CHECK_VIOLATION = "23514"

_ACCOUNT = (accounts.c.id, accounts.c.name, accounts.c.total, accounts.c.created_at)
_CATEGORY = (
    categories.c.id,
    categories.c.name,
    categories.c.parent_id,
    categories.c.type,
    categories.c.created_at,
)
_TRANSACTION = (
    transactions.c.id,
    transactions.c.type,
    transactions.c.amount,
    transactions.c.account_id,
    transactions.c.to_account_id,
    transactions.c.category_id,
    transactions.c.description,
    transactions.c.occurred_at,
    transactions.c.created_at,
)

_to_account = accounts.alias("ta")
_NAMED_TRANSACTIONS = (
    select(
        transactions.c.id,
        transactions.c.type,
        transactions.c.amount,
        accounts.c.name.label("account_name"),
        _to_account.c.name.label("to_account_name"),
        categories.c.name.label("category_name"),
        transactions.c.description,
        transactions.c.occurred_at,
    )
    .select_from(
        transactions.join(accounts, accounts.c.id == transactions.c.account_id)
        .outerjoin(_to_account, _to_account.c.id == transactions.c.to_account_id)
        .outerjoin(categories, categories.c.id == transactions.c.category_id)
    )
    .order_by(transactions.c.occurred_at.desc(), transactions.c.id.desc())
)

_MONTH_TOTALS = text("""
SELECT
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::numeric AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::numeric AS expense
FROM transactions
WHERE occurred_at >= :start AND occurred_at < :end
""")

# Net worth at the end of each period (day, week or month), walking back from the current
# account totals.
_NET_WORTH = text("""
WITH buckets AS (
    SELECT generate_series(
        date_trunc(CAST(:g AS text), CAST(:start AS timestamptz)),
        CAST(:end AS timestamptz),
        ('1 ' || CAST(:g AS text))::interval
    ) AS bucket_start
)
SELECT b.bucket_start AS bucket_at,
    ((SELECT COALESCE(SUM(total), 0) FROM accounts) - COALESCE((
        SELECT SUM(CASE t.type WHEN 'income' THEN t.amount WHEN 'expense' THEN -t.amount ELSE 0 END)
        FROM transactions t
        WHERE t.occurred_at > LEAST(
            b.bucket_start + ('1 ' || CAST(:g AS text))::interval - interval '1 microsecond',
            CAST(:end AS timestamptz)
        )
    ), 0))::numeric AS total
FROM buckets b
ORDER BY b.bucket_start
""")

# Sum and count of one type per category; account_id matches source or destination.
_CATEGORY_STATS = text("""
WITH filtered AS (
    SELECT t.category_id, t.amount FROM transactions t
    WHERE t.type = CAST(:type AS transaction_type)
      AND t.occurred_at >= :start AND t.occurred_at <= :end
      AND (CAST(:account AS int) IS NULL
           OR t.account_id = CAST(:account AS int) OR t.to_account_id = CAST(:account AS int))
),
totals AS (SELECT COALESCE(SUM(amount), 0)::numeric AS total_amount FROM filtered)
SELECT f.category_id,
    COALESCE(c.name, 'Sin categoría') AS name,
    SUM(f.amount)::numeric AS amount,
    COUNT(*) AS tx_count,
    CASE WHEN (SELECT total_amount FROM totals) > 0
         THEN (SUM(f.amount) / (SELECT total_amount FROM totals))::float8
         ELSE 0::float8 END AS share
FROM filtered f LEFT JOIN categories c ON c.id = f.category_id
GROUP BY f.category_id, c.name
ORDER BY SUM(f.amount) DESC, name ASC
""")

_MONTHLY_STATS = text("""
SELECT to_char(date_trunc('month', occurred_at), 'YYYY-MM') AS month,
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'), 0)::numeric AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'), 0)::numeric AS expense
FROM transactions
WHERE occurred_at >= :start AND occurred_at <= :end
  AND type IN ('income', 'expense')
  AND (CAST(:account AS int) IS NULL
       OR account_id = CAST(:account AS int) OR to_account_id = CAST(:account AS int))
  AND (CAST(:category AS int) IS NULL OR category_id = CAST(:category AS int))
GROUP BY date_trunc('month', occurred_at)
ORDER BY date_trunc('month', occurred_at)
""")

_CATEGORY_TOTALS = text("""
SELECT category_id, type::text AS type, SUM(amount)::numeric AS amount
FROM transactions
WHERE occurred_at >= :start AND occurred_at < :end AND type IN ('income', 'expense')
GROUP BY category_id, type
""")

# The budget in effect for every category and period at a month, skipping ended ones. Yearly
# rows sit on January 1st.
_EFFECTIVE_BUDGETS = text("""
SELECT category_id, period, since, amount FROM (
    SELECT DISTINCT ON (category_id, period) category_id, period, month AS since, amount
    FROM budgets WHERE month <= :month
    ORDER BY category_id, period, month DESC
) e
WHERE amount IS NOT NULL
""")

_BUDGET_AT = text("""
SELECT amount FROM budgets
WHERE category_id = :category AND period = :period AND month <= :month
ORDER BY month DESC LIMIT 1
""")

_BUDGET_EXISTS = text("""
SELECT EXISTS (SELECT 1 FROM budgets
               WHERE category_id = :category AND period = :period AND month = :month)
""")

_UPSERT_BUDGET = text("""
INSERT INTO budgets (category_id, period, month, amount)
VALUES (:category, :period, :month, CAST(:amount AS numeric))
ON CONFLICT (category_id, period, month) DO UPDATE SET amount = EXCLUDED.amount
""")

_DELETE_BUDGETS_AFTER = text("""
DELETE FROM budgets WHERE category_id = :category AND period = :period AND month > :month
""")

# Rows that change nothing: one equal to its predecessor, or a leading NULL row.
_COLLAPSE_BUDGETS = text("""
DELETE FROM budgets b
USING (
    SELECT id, amount,
           LAG(amount) OVER (ORDER BY month) AS prev_amount,
           ROW_NUMBER() OVER (ORDER BY month) AS rn
    FROM budgets WHERE category_id = :category AND period = :period
) x
WHERE b.id = x.id
  AND ((x.rn = 1 AND x.amount IS NULL)
       OR (x.rn > 1 AND x.amount IS NOT DISTINCT FROM x.prev_amount))
""")


def _sqlstate(err: IntegrityError) -> str | None:
    return getattr(err.orig, "sqlstate", None)


class FinanceRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def _rows(self, query: Any, params: dict[str, Any] | None = None) -> list[Any]:
        async with self._engine.connect() as conn:
            return list(await conn.execute(query, params or {}))

    async def _row(self, query: Any, params: dict[str, Any] | None = None) -> Any:
        async with self._engine.connect() as conn:
            return (await conn.execute(query, params or {})).one_or_none()

    # Accounts

    async def get_account(self, id: int) -> Account:
        row = await self._row(select(*_ACCOUNT).where(accounts.c.id == id))
        if row is None:
            raise NotFoundError
        return Account.model_construct(**row._mapping)

    async def list_accounts(self) -> list[Account]:
        rows = await self._rows(select(*_ACCOUNT).order_by(accounts.c.name))
        return [Account.model_construct(**r._mapping) for r in rows]

    async def create_account(self, name: str) -> Account:
        row = await self._row(accounts.insert().values(name=name).returning(*_ACCOUNT))
        return Account.model_construct(**row._mapping)

    async def update_account(self, id: int, name: str) -> Account:
        stmt = update(accounts).where(accounts.c.id == id).values(name=name)
        row = await self._row(stmt.returning(*_ACCOUNT))
        if row is None:
            raise NotFoundError
        return Account.model_construct(**row._mapping)

    async def delete_account(self, id: int) -> None:
        try:
            await self._row(delete(accounts).where(accounts.c.id == id).returning(accounts.c.id))
        except IntegrityError as err:
            if _sqlstate(err) == FK_VIOLATION:
                raise AccountInUseError from err
            raise

    # Categories

    async def get_category(self, id: int) -> Category:
        row = await self._row(select(*_CATEGORY).where(categories.c.id == id))
        if row is None:
            raise NotFoundError
        return Category.model_construct(**row._mapping)

    async def get_category_type(self, id: int) -> str:
        row = await self._row(select(categories.c.type).where(categories.c.id == id))
        if row is None:
            raise NotFoundError
        return str(row.type)

    async def list_categories(self) -> list[Category]:
        rows = await self._rows(select(*_CATEGORY).order_by(categories.c.type, categories.c.name))
        return [Category.model_construct(**r._mapping) for r in rows]

    async def create_category(self, name: str, parent_id: int | None, type: str) -> Category:
        stmt = categories.insert().values(name=name, parent_id=parent_id, type=type)
        return await self._write_category(stmt)

    async def update_category(
        self, id: int, name: str, parent_id: int | None, type: str
    ) -> Category:
        stmt = (
            update(categories)
            .where(categories.c.id == id)
            .values(name=name, parent_id=parent_id, type=type)
        )
        return await self._write_category(stmt)

    async def _write_category(self, stmt: Any) -> Category:
        try:
            row = await self._row(stmt.returning(*_CATEGORY))
        except IntegrityError as err:
            # A parent that does not exist, or the category itself.
            if _sqlstate(err) in (FK_VIOLATION, CHECK_VIOLATION):
                raise InvalidInputError from err
            raise
        if row is None:
            raise NotFoundError
        return Category.model_construct(**row._mapping)

    async def delete_category(self, id: int) -> None:
        try:
            await self._row(
                delete(categories).where(categories.c.id == id).returning(categories.c.id)
            )
        except IntegrityError as err:
            if _sqlstate(err) == FK_VIOLATION:
                raise CategoryInUseError from err
            raise

    # Transactions

    async def get_transaction(self, id: int) -> Transaction:
        row = await self._row(select(*_TRANSACTION).where(transactions.c.id == id))
        if row is None:
            raise NotFoundError
        return Transaction.model_construct(**row._mapping)

    async def list_transactions(
        self,
        account_id: int | None,
        category_id: int | None,
        type: str | None,
        start: datetime | None,
        end: datetime | None,
    ) -> list[Transaction]:
        """Newest first. account_id matches the source or the destination of a transfer."""
        t = transactions
        query = select(*_TRANSACTION).order_by(t.c.occurred_at.desc(), t.c.id.desc())
        if account_id is not None:
            query = query.where(or_(t.c.account_id == account_id, t.c.to_account_id == account_id))
        if category_id is not None:
            query = query.where(t.c.category_id == category_id)
        if type is not None:
            query = query.where(t.c.type == type)
        if start is not None:
            query = query.where(t.c.occurred_at >= start)
        if end is not None:
            query = query.where(t.c.occurred_at <= end)
        return [Transaction.model_construct(**r._mapping) for r in await self._rows(query)]

    async def create_transaction(self, values: dict[str, Any]) -> Transaction:
        if values.get("occurred_at") is None:
            values = {k: v for k, v in values.items() if k != "occurred_at"}
        return await self._write_transaction(transactions.insert().values(values))

    async def update_transaction(self, id: int, values: dict[str, Any]) -> Transaction:
        stmt = update(transactions).where(transactions.c.id == id).values(values)
        return await self._write_transaction(stmt)

    async def _write_transaction(self, stmt: Any) -> Transaction:
        try:
            row = await self._row(stmt.returning(*_TRANSACTION))
        except IntegrityError as err:
            if _sqlstate(err) == FK_VIOLATION:
                raise InvalidInputError from err
            raise
        if row is None:
            raise NotFoundError
        return Transaction.model_construct(**row._mapping)

    async def delete_transaction(self, id: int) -> None:
        stmt = delete(transactions).where(transactions.c.id == id).returning(transactions.c.id)
        if await self._row(stmt) is None:
            raise NotFoundError

    # Overview and stats

    async def accounts_total(self) -> Decimal:
        row = await self._row(select(func.coalesce(func.sum(accounts.c.total), 0)))
        return Decimal(row[0])

    async def month_totals(self, start: datetime, end: datetime) -> tuple[Decimal, Decimal]:
        row = await self._row(_MONTH_TOTALS, {"start": start, "end": end})
        return row.income, row.expense

    async def recent_transactions(self, since: datetime) -> list[OverviewTransaction]:
        query = _NAMED_TRANSACTIONS.where(transactions.c.occurred_at >= since)
        return [OverviewTransaction.model_construct(**r._mapping) for r in await self._rows(query)]

    async def earliest_transaction(self) -> datetime | None:
        row = await self._row(select(func.min(transactions.c.occurred_at)))
        value: datetime | None = row[0]
        return value

    async def net_worth_series(
        self, granularity: str, start: datetime, end: datetime
    ) -> list[NetWorthPoint]:
        rows = await self._rows(_NET_WORTH, {"g": granularity, "start": start, "end": end})
        return [NetWorthPoint(date=r.bucket_at.date().isoformat(), total=r.total) for r in rows]

    async def category_stats(
        self, type: str, start: datetime, end: datetime, account_id: int | None
    ) -> list[CategoryStat]:
        params = {"type": type, "start": start, "end": end, "account": account_id}
        return [
            CategoryStat.model_construct(**r._mapping)
            for r in await self._rows(_CATEGORY_STATS, params)
        ]

    async def monthly_stats(
        self, start: datetime, end: datetime, account_id: int | None, category_id: int | None
    ) -> list[MonthlyStat]:
        params = {"start": start, "end": end, "account": account_id, "category": category_id}
        return [
            MonthlyStat.model_construct(
                month=r.month, income=r.income, expense=r.expense, balance=r.income - r.expense
            )
            for r in await self._rows(_MONTHLY_STATS, params)
        ]

    # Budgets

    async def set_budget(
        self,
        category_id: int,
        period: str,
        start: date,
        next: date,
        amount: Decimal | None,
        scope: str,
    ) -> None:
        """Rows mean "from this period on". scope once restores the next period's previous
        value, confining the change to one period; scope forward drops every later row. A None
        amount ends the budget. Rows that change nothing are dropped afterwards."""
        key = {"category": category_id, "period": period}
        async with self._engine.begin() as conn:
            if scope == "forward":
                await conn.execute(_DELETE_BUDGETS_AFTER, key | {"month": start})
            elif not (await conn.execute(_BUDGET_EXISTS, key | {"month": next})).scalar():
                prev = (await conn.execute(_BUDGET_AT, key | {"month": start})).scalar()
                await conn.execute(_UPSERT_BUDGET, key | {"month": next, "amount": prev})
            try:
                await conn.execute(_UPSERT_BUDGET, key | {"month": start, "amount": amount})
            except IntegrityError as err:
                if _sqlstate(err) == FK_VIOLATION:
                    raise NotFoundError from err
                raise
            await conn.execute(_COLLAPSE_BUDGETS, key)

    async def effective_budgets(self, month: date) -> list[EffectiveBudget]:
        rows = await self._rows(_EFFECTIVE_BUDGETS, {"month": month})
        return [EffectiveBudget(r.category_id, r.period, r.since, r.amount) for r in rows]

    async def category_totals(self, start: datetime, end: datetime) -> list[CategoryTotal]:
        rows = await self._rows(_CATEGORY_TOTALS, {"start": start, "end": end})
        return [CategoryTotal(r.category_id, r.type, r.amount) for r in rows]

    async def category_transactions(
        self, category_ids: list[int], type: str, start: datetime, end: datetime
    ) -> list[OverviewTransaction]:
        t = transactions
        query: Select[Any] = _NAMED_TRANSACTIONS.where(
            t.c.category_id.in_(category_ids),
            t.c.type == type,
            t.c.occurred_at >= start,
            t.c.occurred_at < end,
        )
        return [OverviewTransaction.model_construct(**r._mapping) for r in await self._rows(query)]
