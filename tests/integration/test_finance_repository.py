"""FinanceRepository against the test database."""

from datetime import UTC, date, datetime
from decimal import Decimal

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.finance import FinanceRepository
from gv_api.finance.errors import (
    AccountInUseError,
    CategoryInUseError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import Transaction


@pytest.fixture
def repo(db: AsyncEngine) -> FinanceRepository:
    return FinanceRepository(db)


def utc(y: int, m: int, d: int, h: int = 0, mi: int = 0, s: int = 0) -> datetime:
    return datetime(y, m, d, h, mi, s, tzinfo=UTC)


async def tx(
    repo: FinanceRepository,
    type: str,
    amount: int,
    account: int,
    at: datetime,
    category: int | None = None,
) -> Transaction:
    return await repo.create_transaction(
        {
            "type": type,
            "amount": Decimal(amount),
            "account_id": account,
            "to_account_id": None,
            "category_id": category,
            "description": None,
            "occurred_at": at,
        }
    )


async def test_accounts_crud(repo: FinanceRepository) -> None:
    created = await repo.create_account("Checking")
    got = await repo.get_account(created.id)
    assert (got.name, got.total) == ("Checking", Decimal(0))
    assert (await repo.update_account(created.id, "Renamed")).name == "Renamed"
    assert [a.name for a in await repo.list_accounts()] == ["Renamed"]
    await repo.delete_account(created.id)
    with pytest.raises(NotFoundError):
        await repo.get_account(created.id)
    with pytest.raises(NotFoundError):
        await repo.update_account(created.id, "X")


async def test_account_with_transactions_cannot_be_deleted(repo: FinanceRepository) -> None:
    acc = await repo.create_account("Busy")
    await tx(repo, "income", 100, acc.id, utc(2026, 5, 10, 12))
    assert (await repo.get_account(acc.id)).total == Decimal(100)
    with pytest.raises(AccountInUseError):
        await repo.delete_account(acc.id)


async def test_categories_crud_and_in_use(repo: FinanceRepository) -> None:
    parent = await repo.create_category("Food", None, "expense")
    child = await repo.create_category("Groceries", parent.id, "expense")
    assert child.parent_id == parent.id
    with pytest.raises(CategoryInUseError):
        await repo.delete_category(parent.id)

    acc = await repo.create_account("A")
    await tx(repo, "expense", 5, acc.id, utc(2026, 5, 10, 12), child.id)
    with pytest.raises(CategoryInUseError):
        await repo.delete_category(child.id)

    with pytest.raises(InvalidInputError):
        await repo.create_category("Orphan", 99999, "expense")
    with pytest.raises(InvalidInputError):
        await repo.update_category(child.id, "Self", child.id, "expense")
    with pytest.raises(NotFoundError):
        await repo.update_category(99999, "X", None, "income")
    with pytest.raises(NotFoundError):
        await repo.get_category(99999)
    assert await repo.get_category_type(child.id) == "expense"
    with pytest.raises(NotFoundError):
        await repo.get_category_type(99999)

    salary = await repo.create_category("Salary", None, "income")
    # Ordered by type (income first) and name.
    assert [c.id for c in await repo.list_categories()] == [salary.id, parent.id, child.id]


async def test_transaction_with_invalid_refs(repo: FinanceRepository) -> None:
    with pytest.raises(InvalidInputError):
        await tx(repo, "income", 10, 99999, utc(2026, 5, 10, 12))
    with pytest.raises(NotFoundError):
        await repo.delete_transaction(99999)
    with pytest.raises(NotFoundError):
        await repo.get_transaction(99999)


async def test_transaction_update_moves_totals(repo: FinanceRepository) -> None:
    a = await repo.create_account("A")
    b = await repo.create_account("B")
    t = await tx(repo, "expense", 30, a.id, utc(2026, 5, 10))
    values = {
        "type": "transfer",
        "amount": Decimal("12.50"),
        "account_id": a.id,
        "to_account_id": b.id,
        "category_id": None,
        "description": "move",
        "occurred_at": utc(2026, 5, 11),
    }
    updated = await repo.update_transaction(t.id, values)
    assert (updated.type, updated.description) == ("transfer", "move")
    assert (await repo.get_account(a.id)).total == Decimal("-12.50")
    assert (await repo.get_account(b.id)).total == Decimal("12.50")
    with pytest.raises(NotFoundError):
        await repo.update_transaction(99999, values)
    await repo.delete_transaction(t.id)
    assert (await repo.get_account(b.id)).total == Decimal(0)


async def test_create_without_a_time_uses_now(repo: FinanceRepository) -> None:
    acc = await repo.create_account("A")
    created = await repo.create_transaction(
        {"type": "income", "amount": Decimal(1), "account_id": acc.id, "occurred_at": None}
    )
    assert created.occurred_at == created.created_at


async def test_list_transactions_filters(repo: FinanceRepository) -> None:
    a = await repo.create_account("A")
    b = await repo.create_account("B")
    food = await repo.create_category("Food", None, "expense")
    day_start = await tx(repo, "expense", 1, a.id, utc(2026, 5, 10))
    day_end = await tx(repo, "expense", 2, a.id, utc(2026, 5, 10, 23, 59))
    before = await tx(repo, "expense", 3, a.id, utc(2026, 5, 9, 12))
    after = await tx(repo, "income", 4, a.id, utc(2026, 5, 11, 12))
    in_b = await tx(repo, "expense", 5, b.id, utc(2026, 5, 10, 12))
    with_cat = await tx(repo, "expense", 6, a.id, utc(2026, 5, 10, 13), food.id)
    day = (utc(2026, 5, 10), utc(2026, 5, 10, 23, 59, 59))

    async def ids(**kw: object) -> list[int]:
        args = {"account_id": None, "category_id": None, "type": None, "start": None, "end": None}
        return [t.id for t in await repo.list_transactions(**(args | kw))]  # type: ignore[arg-type]

    assert await ids(start=day[0], end=day[1]) == [day_end.id, with_cat.id, in_b.id, day_start.id]
    assert await ids(start=utc(2026, 5, 11)) == [after.id]
    assert await ids(end=utc(2026, 5, 9, 23, 59, 59)) == [before.id]
    assert await ids(account_id=b.id, start=day[0], end=day[1]) == [in_b.id]
    assert await ids(category_id=food.id, start=day[0], end=day[1]) == [with_cat.id]
    assert await ids(type="income") == [after.id]


async def test_stats(repo: FinanceRepository) -> None:
    acc = await repo.create_account("Main")
    food = await repo.create_category("Food", None, "expense")
    await tx(repo, "income", 100, acc.id, utc(2026, 4, 5, 10))
    await tx(repo, "expense", 40, acc.id, utc(2026, 4, 20, 10), food.id)
    await tx(repo, "income", 50, acc.id, utc(2026, 5, 5, 10))
    await tx(repo, "expense", 20, acc.id, utc(2026, 5, 20, 10))
    span = (utc(2026, 4, 1), utc(2026, 5, 31, 23, 59, 59))

    stats = await repo.monthly_stats(*span, None, None)
    assert [(s.month, s.income, s.expense, s.balance) for s in stats] == [
        ("2026-04", 100, 40, 60),
        ("2026-05", 50, 20, 30),
    ]
    assert len(await repo.monthly_stats(*span, None, food.id)) == 1

    points = await repo.net_worth_series("month", *span)
    assert [(p.date, p.total) for p in points] == [("2026-04-01", 60), ("2026-05-01", 90)]

    by_cat = await repo.category_stats("expense", *span, acc.id)
    assert [(s.category_id, s.name, s.amount, s.tx_count, s.share) for s in by_cat] == [
        (food.id, "Food", 40, 1, 40 / 60),
        (None, "Sin categoría", 20, 1, 20 / 60),
    ]

    assert await repo.accounts_total() == Decimal(90)
    assert await repo.month_totals(utc(2026, 4, 1), utc(2026, 5, 1)) == (100, 40)
    assert await repo.earliest_transaction() == utc(2026, 4, 5, 10)
    recent = await repo.recent_transactions(utc(2026, 5, 1))
    assert [(r.amount, r.account_name, r.category_name) for r in recent] == [
        (20, "Main", None),
        (50, "Main", None),
    ]


# Budgets


def month(m: int, y: int = 2026) -> date:
    return date(y, m, 1)


def next_month(d: date) -> date:
    return date(d.year + d.month // 12, d.month % 12 + 1, 1)


async def budget_at(repo: FinanceRepository, cat: int, at: date, period: str = "monthly") -> str:
    for b in await repo.effective_budgets(at):
        if b.category_id == cat and b.period == period:
            return f"{b.amount:.2f}"
    return ""


async def set_budget(
    repo: FinanceRepository, cat: int, at: date, amount: str | None, scope: str
) -> None:
    value = None if amount is None else Decimal(amount)
    await repo.set_budget(cat, "monthly", at, next_month(at), value, scope)


async def set_yearly(
    repo: FinanceRepository, cat: int, year: int, amount: str | None, scope: str
) -> None:
    value = None if amount is None else Decimal(amount)
    await repo.set_budget(cat, "yearly", date(year, 1, 1), date(year + 1, 1, 1), value, scope)


async def test_budget_scopes(repo: FinanceRepository) -> None:
    cat = (await repo.create_category("Food", None, "expense")).id
    await set_budget(repo, cat, month(3), "100", "forward")
    assert await budget_at(repo, cat, month(2)) == ""
    assert await budget_at(repo, cat, month(3)) == "100.00"
    assert await budget_at(repo, cat, month(12)) == "100.00"

    await set_budget(repo, cat, month(6), "300", "once")
    assert await budget_at(repo, cat, month(6)) == "300.00"
    assert await budget_at(repo, cat, month(7)) == "100.00"

    await set_budget(repo, cat, month(5), "150", "forward")
    assert await budget_at(repo, cat, month(6)) == "150.00"
    assert await budget_at(repo, cat, month(9)) == "150.00"
    assert await budget_at(repo, cat, month(4)) == "100.00"

    await set_budget(repo, cat, month(8), None, "once")
    assert await budget_at(repo, cat, month(8)) == ""
    assert await budget_at(repo, cat, month(9)) == "150.00"

    await set_budget(repo, cat, month(10), "0", "forward")
    assert await budget_at(repo, cat, month(11)) == "0.00"

    await set_budget(repo, cat, month(4), None, "forward")
    assert await budget_at(repo, cat, month(3)) == "100.00"
    assert await budget_at(repo, cat, month(4)) == ""
    assert await budget_at(repo, cat, month(12)) == ""


async def test_budget_rows_collapse(repo: FinanceRepository, db: AsyncEngine) -> None:
    cat = (await repo.create_category("Food", None, "expense")).id

    async def count() -> int:
        async with db.connect() as conn:
            q = text("SELECT count(*) FROM budgets WHERE category_id = :c")
            return int((await conn.execute(q, {"c": cat})).scalar_one())

    await set_budget(repo, cat, month(3), "100", "forward")
    await set_budget(repo, cat, month(6), "300", "once")
    assert await count() == 3  # 03:100, 06:300, 07:100
    await set_budget(repo, cat, month(6), "100", "month")
    assert await count() == 1
    await set_budget(repo, cat, month(3), None, "forward")
    assert await count() == 0
    await set_budget(repo, cat, month(5), None, "once")
    assert await count() == 0


async def test_budget_cascades_and_unknown_category(repo: FinanceRepository) -> None:
    cat = (await repo.create_category("Food", None, "expense")).id
    await set_budget(repo, cat, month(3), "100", "forward")
    await repo.delete_category(cat)
    assert await budget_at(repo, cat, month(3)) == ""
    with pytest.raises(NotFoundError):
        await set_budget(repo, 9999, month(3), "10", "forward")


async def test_yearly_budgets(repo: FinanceRepository) -> None:
    cat = (await repo.create_category("Property tax", None, "expense")).id
    await set_yearly(repo, cat, 2026, "450", "forward")
    assert await budget_at(repo, cat, month(1), "yearly") == "450.00"
    assert await budget_at(repo, cat, month(12, 2028), "yearly") == "450.00"
    assert await budget_at(repo, cat, month(12, 2025), "yearly") == ""

    await set_yearly(repo, cat, 2027, "500", "once")
    assert await budget_at(repo, cat, month(11, 2027), "yearly") == "500.00"
    assert await budget_at(repo, cat, month(1, 2028), "yearly") == "450.00"

    await set_budget(repo, cat, month(3), "40", "forward")
    assert await budget_at(repo, cat, month(3)) == "40.00"
    assert await budget_at(repo, cat, month(3), "yearly") == "450.00"
    await set_budget(repo, cat, month(3), None, "forward")
    assert await budget_at(repo, cat, month(3), "yearly") == "450.00"

    await set_yearly(repo, cat, 2026, None, "forward")
    assert await budget_at(repo, cat, month(5, 2027), "yearly") == ""


async def test_category_totals_and_transactions(repo: FinanceRepository) -> None:
    acc = await repo.create_account("Main")
    food = await repo.create_category("Food", None, "expense")
    rent = await repo.create_category("Rent", None, "expense")
    await tx(repo, "expense", 10, acc.id, utc(2026, 3, 1))
    await tx(repo, "expense", 20, acc.id, utc(2026, 3, 31, 23))
    await tx(repo, "income", 5, acc.id, utc(2026, 3, 15))
    await tx(repo, "expense", 99, acc.id, utc(2026, 4, 1))  # the end is exclusive
    totals = await repo.category_totals(utc(2026, 3, 1), utc(2026, 4, 1))
    assert {(t.category_id, t.type, t.amount) for t in totals} == {
        (None, "expense", 30),
        (None, "income", 5),
    }

    await tx(repo, "expense", 10, acc.id, utc(2026, 3, 2, 9), food.id)
    await tx(repo, "expense", 20, acc.id, utc(2026, 3, 20, 9), food.id)
    await tx(repo, "expense", 99, acc.id, utc(2026, 4, 1), food.id)
    await tx(repo, "expense", 800, acc.id, utc(2026, 3, 1, 9), rent.id)
    out = await repo.category_transactions([food.id], "expense", utc(2026, 3, 1), utc(2026, 4, 1))
    assert [(t.amount, t.account_name, t.category_name) for t in out] == [
        (20, "Main", "Food"),
        (10, "Main", "Food"),
    ]
