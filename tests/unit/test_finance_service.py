"""FinanceService over a recording fake repository: transactions, overview, stats, estimation
and budgets."""

from datetime import UTC, date, datetime, timedelta
from decimal import Decimal
from typing import Any, cast
from zoneinfo import ZoneInfo

import pytest

from gv_api.finance import FinanceService, budgets
from gv_api.finance.errors import (
    BudgetTransferError,
    CategoryMismatchError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import (
    Category,
    CategoryTotal,
    EffectiveBudget,
    NetWorthPoint,
    TransactionRequest,
)
from gv_api.finance.service import Repository

from .test_tasks_service import FakeRepo

MADRID = ZoneInfo("Europe/Madrid")
NOW = datetime(2026, 5, 18, 10, tzinfo=UTC)


class Seq:
    """Successive return values for a method called several times."""

    def __init__(self, *values: Any) -> None:
        self.values = list(values)


class Repo(FakeRepo):
    def __getattr__(self, name: str) -> Any:
        method = super().__getattr__(name)
        value = self.returns.get(name)
        if not isinstance(value, Seq):
            return method

        async def next_value(*args: Any) -> Any:
            self.calls.append((name, *args))
            return value.values.pop(0)

        return next_value


def svc(repo: FakeRepo, now: datetime = NOW, loc: ZoneInfo = MADRID) -> FinanceService:
    return FinanceService(cast(Repository, repo), loc, lambda: now)


def d(s: str | int) -> Decimal:
    return Decimal(s)


def madrid(y: int, m: int, day: int = 1) -> datetime:
    return datetime(y, m, day, tzinfo=MADRID)


def tx_req(**kw: Any) -> TransactionRequest:
    return TransactionRequest.model_validate(
        {"type": "income", "amount": "100", "account_id": 1} | kw
    )


# Delegation


async def test_accounts_and_categories_delegate() -> None:
    repo = FakeRepo()
    s = svc(repo)
    await s.get_account(1)
    await s.list_accounts()
    await s.create_account("A")
    await s.update_account(1, "B")
    await s.delete_account(1)
    await s.get_category(2)
    await s.list_categories()
    await s.create_category("C", None, "expense")
    await s.update_category(2, "D", 1, "income")
    await s.delete_category(2)
    await s.get_transaction(3)
    await s.list_transactions(1, None, "income", None, None)
    await s.delete_transaction(3)
    assert repo.calls == [
        ("get_account", 1),
        ("list_accounts",),
        ("create_account", "A"),
        ("update_account", 1, "B"),
        ("delete_account", 1),
        ("get_category", 2),
        ("list_categories",),
        ("create_category", "C", None, "expense"),
        ("update_category", 2, "D", 1, "income"),
        ("delete_category", 2),
        ("get_transaction", 3),
        ("list_transactions", 1, None, "income", None, None),
        ("delete_transaction", 3),
    ]


# Transactions


async def test_transaction_category_must_match_its_type() -> None:
    with pytest.raises(CategoryMismatchError):
        await svc(FakeRepo(get_category_type="expense")).create_transaction(tx_req(category_id=5))
    with pytest.raises(CategoryMismatchError):
        await svc(FakeRepo(get_category_type="income")).update_transaction(
            1, tx_req(type="expense", category_id=7)
        )


async def test_unknown_category_is_invalid_input() -> None:
    repo = FakeRepo(get_category_type=NotFoundError())
    with pytest.raises(InvalidInputError):
        await svc(repo).create_transaction(tx_req(category_id=5))


async def test_transaction_without_category_and_values() -> None:
    repo = FakeRepo(create_transaction="created", update_transaction="updated")
    assert await svc(repo).create_transaction(tx_req(description="x")) == "created"
    assert repo.calls[0][1] == {
        "type": "income",
        "amount": d(100),
        "account_id": 1,
        "to_account_id": None,
        "category_id": None,
        "description": "x",
        "occurred_at": None,
    }
    repo = FakeRepo(get_category_type="income", update_transaction="updated")
    assert await svc(repo).update_transaction(4, tx_req(category_id=2)) == "updated"
    assert repo.calls[1][:2] == ("update_transaction", 4)


# Overview and stats


async def test_overview() -> None:
    repo = Repo(
        accounts_total=d(5000),
        month_totals=Seq((d(1000), d(400)), (d(800), d(300))),
        recent_transactions=[],
    )
    got = await svc(repo).get_overview()
    assert repo.calls[1] == ("month_totals", madrid(2026, 5), madrid(2026, 6))
    assert repo.calls[2] == ("month_totals", madrid(2026, 4), madrid(2026, 5))
    assert repo.calls[3] == ("recent_transactions", NOW - timedelta(days=30))
    assert got.model_dump(mode="json") == {
        "accounts_total": "5000",
        "month": {"income": "1000", "expense": "400", "balance": "600"},
        "previous_month": {"income": "800", "expense": "300", "balance": "500"},
        "recent_transactions": [],
    }


async def test_net_worth_defaults_from_one_day_before_the_earliest() -> None:
    earliest = datetime(2025, 6, 15, tzinfo=UTC)
    repo = FakeRepo(earliest_transaction=earliest, net_worth_series=[])
    await svc(repo).net_worth(None, None, "")
    _, g, start, end = repo.calls[1]
    assert (g, start, end) == ("day", earliest.astimezone(MADRID) - timedelta(days=1), NOW)


@pytest.mark.parametrize(
    ("granularity", "back"),
    [("week", timedelta(days=7)), ("day", timedelta(days=1))],
)
async def test_net_worth_steps_back_one_period(granularity: str, back: timedelta) -> None:
    earliest = datetime(2025, 6, 15, tzinfo=UTC)
    repo = FakeRepo(earliest_transaction=earliest, net_worth_series=[])
    await svc(repo).net_worth(None, NOW, granularity)
    assert repo.calls[1][2] == earliest.astimezone(MADRID) - back


async def test_net_worth_without_transactions_starts_six_months_back() -> None:
    repo = FakeRepo(earliest_transaction=None, net_worth_series=[])
    await svc(repo).net_worth(None, None, "month")
    assert repo.calls[1][2] == NOW.astimezone(MADRID).replace(month=11, year=2025)


async def test_given_range_is_kept() -> None:
    start, end = datetime(2026, 1, 1, tzinfo=UTC), datetime(2026, 2, 1, tzinfo=UTC)
    repo = FakeRepo(category_stats=[], monthly_stats=[])
    await svc(repo).category_stats("expense", start, end, 3)
    await svc(repo).monthly_stats(start, end, None, 4)
    assert repo.calls == [
        ("category_stats", "expense", start, end, 3),
        ("monthly_stats", start, end, None, 4),
    ]


async def test_monthly_stats_default_steps_back_a_clamped_month() -> None:
    repo = FakeRepo(earliest_transaction=datetime(2026, 3, 31, 10, tzinfo=UTC), monthly_stats=[])
    await svc(repo).monthly_stats(None, None, None, None)
    assert repo.calls[1][1] == datetime(2026, 2, 28, 12, tzinfo=MADRID)


# Estimation


def point(day: date, total: str) -> NetWorthPoint:
    return NetWorthPoint(date=day.isoformat(), total=d(total))


async def test_estimation_saving_mode() -> None:
    actual = [point(date(2026, 3, 1), "10000"), point(date(2026, 4, 1), "10500")]
    repo = FakeRepo(earliest_transaction=datetime(2026, 3, 6, tzinfo=UTC), net_worth_series=actual)
    got = await svc(repo).estimation(date(2026, 3, 1), date(2026, 6, 1), "saving")
    assert repo.calls[1] == (
        "net_worth_series",
        "month",
        madrid(2026, 3),
        madrid(2026, 5) - timedelta(microseconds=1),
    )
    out = got.model_dump(mode="json")
    assert out["saving"] == "500"
    assert out["rate"] == "0"
    assert [(p["date"], p["total"], p["estimated"]) for p in out["points"]] == [
        ("2026-03-01", "10000", False),
        ("2026-04-01", "10500", False),
        ("2026-05-01", "11000", True),
        ("2026-06-01", "11500", True),
    ]


async def test_estimation_rate_mode() -> None:
    actual = [
        point(date(2026, 2, 1), "1000"),
        point(date(2026, 3, 1), "1100"),
        point(date(2026, 4, 1), "1210"),
    ]
    repo = FakeRepo(earliest_transaction=None, net_worth_series=actual)
    got = await svc(repo).estimation(date(2026, 2, 1), date(2026, 5, 1), "rate")
    assert got.rate == Decimal("10.0000")
    assert [(p.date, p.total) for p in got.points[3:]] == [("2026-05-01", Decimal("1331.00"))]


async def test_estimation_starts_at_the_earliest_month_and_skips_negative_rates() -> None:
    actual = [point(date(2026, 3, 1), "-10"), point(date(2026, 4, 1), "20")]
    repo = FakeRepo(earliest_transaction=datetime(2026, 3, 9, tzinfo=UTC), net_worth_series=actual)
    got = await svc(repo).estimation(date(2025, 1, 1), date(2026, 5, 1), "rate")
    assert repo.calls[1][2] == madrid(2026, 3)
    assert got.rate == 0
    assert got.points[-1].total == Decimal("20.00")


async def test_estimation_in_the_future_has_no_actuals() -> None:
    repo = FakeRepo(earliest_transaction=None)
    got = await svc(repo).estimation(date(2026, 7, 1), date(2026, 9, 1), "rate")
    assert repo.names() == ["earliest_transaction"]
    assert [(p.date, p.total, p.estimated) for p in got.points] == [
        ("2026-07-01", 0, True),
        ("2026-08-01", 0, True),
        ("2026-09-01", 0, True),
    ]


# Budgets


def cat(id: int, name: str, type: str, parent: int | None = None) -> Category:
    return Category.model_construct(id=id, name=name, type=type, parent_id=parent, created_at=NOW)


def total(category: int | None, type: str, amount: str) -> CategoryTotal:
    return CategoryTotal(category, type, d(amount))


def monthly(category: int, since: date, amount: str) -> EffectiveBudget:
    return EffectiveBudget(category, "monthly", since, d(amount))


def yearly(category: int, since: date, amount: str) -> EffectiveBudget:
    return EffectiveBudget(category, "yearly", since, d(amount))


MARCH = date(2026, 3, 1)
JAN = date(2026, 1, 1)
CATEGORIES = [
    cat(1, "Salary", "income"),
    cat(2, "Food", "expense"),
    cat(3, "Eating out", "expense", 2),
    cat(5, "Groceries", "expense", 2),
    cat(4, "Coffee", "expense", 3),
    cat(6, "Transport", "expense"),
    cat(7, "Savings", "transfer"),
]


def budget_repo(
    cats: list[Category],
    effective: list[EffectiveBudget],
    month_rows: list[CategoryTotal],
    year_rows: list[CategoryTotal] | None = None,
    past_rows: list[CategoryTotal] | None = None,
    prev_year_rows: list[CategoryTotal] | None = None,
) -> Repo:
    return Repo(
        list_categories=cats,
        effective_budgets=effective,
        category_totals=Seq(month_rows, year_rows or [], past_rows or [], prev_year_rows or []),
    )


async def test_budget_month() -> None:
    repo = budget_repo(
        CATEGORIES,
        [
            monthly(1, JAN, "2000"),
            monthly(2, JAN, "500"),
            monthly(3, JAN, "100"),
            monthly(4, MARCH, "20"),
            monthly(7, JAN, "50"),  # a transfer: ignored
        ],
        [
            total(1, "income", "2100"),
            total(3, "expense", "90"),
            total(4, "expense", "35"),
            total(5, "expense", "200"),
            total(6, "expense", "40"),
            total(None, "expense", "10"),
        ],
        past_rows=[total(5, "expense", "300"), total(6, "expense", "90")],
        prev_year_rows=[total(6, "expense", "700")],
    )
    out = await svc(repo).budget_month(MARCH)
    assert repo.calls[1] == ("effective_budgets", MARCH)
    assert [c[1:] for c in repo.calls[2:]] == [
        (madrid(2026, 3), madrid(2026, 4)),
        (madrid(2026, 1), madrid(2027, 1)),
        (madrid(2025, 12), madrid(2026, 3)),
        (madrid(2025, 1), madrid(2026, 1)),
    ]
    assert (out.month, out.month_progress) == ("2026-03", 1.0)
    assert [(i.name, i.depth, i.actual, i.status, i.since) for i in out.items] == [
        ("Salary", 0, 2100, "met", "2026-01"),
        ("Food", 0, 325, "ok", "2026-01"),
        ("Eating out", 1, 125, "over", "2026-01"),
        ("Coffee", 2, 35, "over", "2026-03"),
    ]
    assert out.items[1].remaining == 175
    e, i = out.expense, out.income
    assert (e.budgeted, e.actual, e.unbudgeted, e.overspent) == (500, 375, 50, 25)
    assert (i.budgeted, i.actual, i.unbudgeted, i.overspent) == (2000, 2100, 0, 0)
    assert {a.category_id: a.amount for a in out.averages} == {2: 100, 5: 100, 6: 30}
    assert [(p.category_id, p.amount) for p in out.previous_year] == [(6, 700)]
    assert out.planned_balance == 1500
    assert (out.yearly.year, out.yearly.items) == ("2026", [])


async def test_budget_month_with_yearly_budgets() -> None:
    cats = [
        cat(1, "Transport", "expense"),
        cat(2, "Car maintenance", "expense", 1),
        cat(3, "Fuel", "expense", 1),
        cat(4, "Property tax", "expense"),
        cat(5, "Bonus", "income"),
    ]
    repo = budget_repo(
        cats,
        [
            monthly(1, JAN, "80"),
            yearly(2, JAN, "500"),
            yearly(4, JAN, "450"),
            yearly(5, JAN, "1200"),
        ],
        [total(2, "expense", "380"), total(3, "expense", "60"), total(4, "expense", "452")],
        [total(2, "expense", "425"), total(3, "expense", "170"), total(4, "expense", "452")],
    )
    out = await svc(repo).budget_month(MARCH)
    [fuel_budget] = out.items
    assert (fuel_budget.actual, fuel_budget.period, fuel_budget.status) == (60, "monthly", "ok")
    assert (out.expense.actual, out.expense.unbudgeted, out.expense.overspent) == (892, 0, 0)

    y = out.yearly
    assert [(i.name, i.depth, i.actual, i.status, i.since) for i in y.items] == [
        ("Car maintenance", 0, 425, "warning", "2026"),
        ("Property tax", 0, 452, "over", "2026"),
        ("Bonus", 0, 0, "pending", "2026"),
    ]
    assert (y.expense.budgeted, y.expense.actual, y.expense.overspent) == (950, 877, 2)
    assert y.income.budgeted == 1200
    assert out.planned_balance == Decimal("-59.17")


async def test_budget_status_thresholds() -> None:
    repo = budget_repo(
        [cat(1, "Warn", "expense"), cat(2, "Zero", "expense"), cat(3, "Pending", "income")],
        [monthly(1, MARCH, "100"), monthly(2, MARCH, "0"), monthly(3, MARCH, "100")],
        [total(1, "expense", "80"), total(2, "expense", "5"), total(3, "income", "40")],
    )
    out = await svc(repo).budget_month(MARCH)
    warn, zero, pending = out.items
    assert (warn.status, warn.progress) == ("warning", pytest.approx(0.8))
    assert (zero.status, zero.progress) == ("over", 1.0)
    assert pending.status == "pending"
    assert out.expense.overspent == 5


async def test_budget_month_defaults_to_now_and_tracks_progress() -> None:
    repo = budget_repo([], [], [])
    out = await svc(repo).budget_month(None)
    assert out.month == "2026-05"
    assert 0 < out.month_progress < 1
    assert 0 < out.yearly.year_progress < 1
    assert budgets.progress(NOW, NOW + timedelta(days=1), NOW + timedelta(days=2)) == 0


async def test_set_budget() -> None:
    with pytest.raises(BudgetTransferError):
        await svc(FakeRepo(get_category_type="transfer")).set_budget(7, "", MARCH, None, "once")
    with pytest.raises(NotFoundError):
        await svc(FakeRepo(get_category_type=NotFoundError())).set_budget(
            9, "", MARCH, None, "once"
        )
    repo = FakeRepo(get_category_type="expense")
    await svc(repo).set_budget(2, "", date(2026, 12, 1), d(10), "forward")
    await svc(repo).set_budget(2, "yearly", MARCH, None, "once")
    assert repo.calls[1] == (
        "set_budget",
        2,
        "monthly",
        date(2026, 12, 1),
        date(2027, 1, 1),
        d(10),
        "forward",
    )
    assert repo.calls[3] == ("set_budget", 2, "yearly", JAN, date(2027, 1, 1), None, "once")


BUDGET_CATS = [
    cat(1, "Transport", "expense"),
    cat(2, "Car maintenance", "expense", 1),
    cat(3, "Fuel", "expense", 1),
    cat(4, "Refund", "income", 1),
    cat(7, "Savings", "transfer"),
]
BUDGETS = [monthly(1, JAN, "80"), yearly(2, JAN, "500")]


async def test_budget_transactions_leave_out_the_other_period_and_type() -> None:
    repo = FakeRepo(
        list_categories=BUDGET_CATS, effective_budgets=BUDGETS, category_transactions=[]
    )
    await svc(repo).budget_transactions(1, "monthly", MARCH)
    assert repo.calls[-1] == (
        "category_transactions",
        [1, 3],
        "expense",
        madrid(2026, 3),
        madrid(2026, 4),
    )


async def test_yearly_budget_transactions_cover_the_year() -> None:
    repo = FakeRepo(
        list_categories=BUDGET_CATS, effective_budgets=BUDGETS, category_transactions=[]
    )
    await svc(repo).budget_transactions(2, "yearly", None)
    assert repo.calls[1] == ("effective_budgets", date(2026, 5, 1))
    assert repo.calls[-1][1:] == ([2], "expense", madrid(2026, 1), madrid(2027, 1))


async def test_budget_transactions_errors() -> None:
    with pytest.raises(NotFoundError):
        await svc(FakeRepo(list_categories=BUDGET_CATS)).budget_transactions(99, "monthly", None)
    with pytest.raises(BudgetTransferError):
        await svc(FakeRepo(list_categories=BUDGET_CATS)).budget_transactions(7, "monthly", None)


def test_tree_helpers() -> None:
    tree = budgets.CategoryTree([*BUDGET_CATS, cat(8, "Orphan", "expense", 99)])
    assert tree.roots == [1, 7, 8]
    assert tree.covered({2: BUDGETS[1]}) == {2}
    assert tree.subtree(1, lambda _: False) == [1, 2, 3]
    assert budgets.next_period("monthly", date(2026, 12, 1)) == date(2027, 1, 1)
    assert budgets.period_label("yearly", JAN) == "2026"
