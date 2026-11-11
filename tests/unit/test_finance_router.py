"""The finance router over the real service and a recording fake repository."""

import json
from datetime import UTC, date, datetime
from decimal import Decimal
from types import SimpleNamespace
from typing import Any, cast
from zoneinfo import ZoneInfo

import httpx
import pytest

from gv_api import finance
from gv_api.auth import AuthService
from gv_api.core.app import create_app
from gv_api.finance import FinanceService
from gv_api.finance.errors import (
    AccountInUseError,
    CategoryInUseError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import Account, Category, Transaction
from gv_api.finance.service import Repository

from .test_finance_service import BUDGET_CATS, BUDGETS, Repo, Seq
from .test_tasks_router import AUTH, CFG, FAIL, FULL, ping
from .test_tasks_service import FakeRepo

NOW = datetime(2026, 5, 18, 10, tzinfo=UTC)
AT = datetime(2026, 5, 10, 12, tzinfo=UTC)
ACCOUNT = Account.model_construct(id=1, name="Main", total=Decimal("12.50"), created_at=AT)
CATEGORY = Category.model_construct(
    id=2, name="Food", type="expense", parent_id=None, created_at=AT
)
TX = Transaction.model_construct(
    id=3,
    type="income",
    amount=Decimal("100.00"),
    account_id=1,
    to_account_id=None,
    category_id=2,
    description=None,
    occurred_at=AT,
    created_at=AT,
)
NEW_TX = {"type": "income", "amount": "50.00", "account_id": 1, "category_id": 2}


async def call(repo: FakeRepo, method: str, path: str, body: Any = None) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[finance.router])
    svc = FinanceService(cast(Repository, repo), ZoneInfo("Europe/Madrid"), lambda: NOW)
    app.state.container = SimpleNamespace(services={AuthService: AUTH, FinanceService: svc})
    content = body if isinstance(body, str) or body is None else json.dumps(body)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.request(method, path, content=content, headers=FULL)
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


def err(status: int, message: str) -> tuple[int, dict[str, str]]:
    return status, {"error": message}


# Accounts


async def test_accounts() -> None:
    assert await call(FakeRepo(list_accounts=[ACCOUNT]), "GET", "/finance/accounts") == (
        200,
        [{"id": 1, "name": "Main", "total": "12.5", "created_at": "2026-05-10T12:00:00Z"}],
    )
    assert (await call(FakeRepo(get_account=ACCOUNT), "GET", "/finance/accounts/1"))[0] == 200
    repo = FakeRepo(create_account=ACCOUNT)
    assert (await call(repo, "POST", "/finance/accounts", {"name": " Main "}))[0] == 201
    assert repo.calls == [("create_account", "Main")]
    repo = FakeRepo(update_account=ACCOUNT)
    assert (await call(repo, "PUT", "/finance/accounts/1", {"name": "Main"}))[0] == 200
    assert await call(FakeRepo(), "DELETE", "/finance/accounts/1") == (204, "")


@pytest.mark.parametrize(
    ("repo", "method", "path", "body", "expected"),
    [
        (
            FakeRepo(list_accounts=FAIL),
            "GET",
            "/finance/accounts",
            None,
            err(500, "Failed to list accounts"),
        ),
        (
            FakeRepo(get_account=NotFoundError()),
            "GET",
            "/finance/accounts/1",
            None,
            err(404, "account not found"),
        ),
        (FakeRepo(), "GET", "/finance/accounts/x", None, err(400, "invalid account id")),
        (FakeRepo(), "POST", "/finance/accounts", "{", err(400, "Invalid Body")),
        (FakeRepo(), "POST", "/finance/accounts", {"name": " "}, err(400, "name is required")),
        (
            FakeRepo(),
            "POST",
            "/finance/accounts",
            {"name": "a" * 41},
            err(400, "name must be at most 40 characters"),
        ),
        (
            FakeRepo(update_account=NotFoundError()),
            "PUT",
            "/finance/accounts/1",
            {"name": "x"},
            err(404, "account not found"),
        ),
        (
            FakeRepo(delete_account=AccountInUseError()),
            "DELETE",
            "/finance/accounts/1",
            None,
            err(409, "account has transactions; delete them first"),
        ),
        (
            FakeRepo(delete_account=FAIL),
            "DELETE",
            "/finance/accounts/1",
            None,
            err(500, "Failed to delete account"),
        ),
    ],
)
async def test_account_errors(
    repo: FakeRepo, method: str, path: str, body: Any, expected: tuple[int, Any]
) -> None:
    assert await call(repo, method, path, body) == expected


# Categories


async def test_categories() -> None:
    status, body = await call(FakeRepo(list_categories=[CATEGORY]), "GET", "/finance/categories")
    assert (status, body[0]["type"], body[0]["parent_id"]) == (200, "expense", None)
    assert (await call(FakeRepo(get_category=CATEGORY), "GET", "/finance/categories/2"))[0] == 200
    repo = FakeRepo(create_category=CATEGORY)
    new = {"name": " Food ", "type": "expense", "parent_id": 5}
    assert (await call(repo, "POST", "/finance/categories", new))[0] == 201
    assert repo.calls == [("create_category", "Food", 5, "expense")]
    repo = FakeRepo(update_category=CATEGORY)
    assert (await call(repo, "PUT", "/finance/categories/2", new))[0] == 200
    assert await call(FakeRepo(), "DELETE", "/finance/categories/2") == (204, "")


FOOD = {"name": "Food", "type": "expense"}


@pytest.mark.parametrize(
    ("repo", "method", "path", "body", "expected"),
    [
        (
            FakeRepo(get_category=NotFoundError()),
            "GET",
            "/finance/categories/2",
            None,
            err(404, "category not found"),
        ),
        (
            FakeRepo(),
            "POST",
            "/finance/categories",
            FOOD | {"type": "x"},
            err(400, "type must be income, expense, or transfer"),
        ),
        (
            FakeRepo(),
            "PUT",
            "/finance/categories/5",
            FOOD | {"parent_id": 5},
            err(400, "parent_id must not equal id"),
        ),
        (
            FakeRepo(create_category=InvalidInputError()),
            "POST",
            "/finance/categories",
            FOOD,
            err(400, "parent_id is invalid"),
        ),
        (
            FakeRepo(create_category=FAIL),
            "POST",
            "/finance/categories",
            FOOD,
            err(500, "Failed to create category"),
        ),
        (
            FakeRepo(update_category=NotFoundError()),
            "PUT",
            "/finance/categories/2",
            FOOD,
            err(404, "category not found"),
        ),
        (
            FakeRepo(update_category=InvalidInputError()),
            "PUT",
            "/finance/categories/2",
            FOOD,
            err(400, "parent_id is invalid"),
        ),
        (
            FakeRepo(update_category=FAIL),
            "PUT",
            "/finance/categories/2",
            FOOD,
            err(500, "Failed to update category"),
        ),
        (
            FakeRepo(delete_category=CategoryInUseError()),
            "DELETE",
            "/finance/categories/2",
            None,
            err(409, "category is referenced by transactions or other categories"),
        ),
        (
            FakeRepo(delete_category=FAIL),
            "DELETE",
            "/finance/categories/2",
            None,
            err(500, "Failed to delete category"),
        ),
    ],
)
async def test_category_errors(
    repo: FakeRepo, method: str, path: str, body: Any, expected: tuple[int, Any]
) -> None:
    assert await call(repo, method, path, body) == expected


# Transactions


async def test_transactions() -> None:
    status, body = await call(FakeRepo(list_transactions=[TX]), "GET", "/finance/transactions")
    assert (status, body[0]["amount"]) == (200, "100")
    repo = FakeRepo(list_transactions=[])
    q = "?account_id=1&category_id=2&type=expense&from=2026-05-01&to=2026-05-31"
    assert (await call(repo, "GET", "/finance/transactions" + q))[0] == 200
    assert repo.calls[0][1:] == (
        1,
        2,
        "expense",
        datetime(2026, 5, 1, tzinfo=UTC),
        datetime(2026, 5, 31, 23, 59, 59, 999999, tzinfo=UTC),
    )
    repo = FakeRepo(list_transactions=[])
    q = "?from=2026-05-01T10:00:00Z&to=2026-05-02T10:00:00Z"
    await call(repo, "GET", "/finance/transactions" + q)
    assert repo.calls[0][4:] == (
        datetime(2026, 5, 1, 10, tzinfo=UTC),
        datetime(2026, 5, 2, 10, tzinfo=UTC),
    )
    assert (await call(FakeRepo(get_transaction=TX), "GET", "/finance/transactions/3"))[0] == 200
    repo = FakeRepo(get_category_type="income", create_transaction=TX)
    assert (await call(repo, "POST", "/finance/transactions", NEW_TX))[0] == 201
    repo = FakeRepo(get_category_type="income", update_transaction=TX)
    body = NEW_TX | {"occurred_at": "2026-05-10T12:00:00Z"}
    assert (await call(repo, "PUT", "/finance/transactions/3", body))[0] == 200
    assert await call(FakeRepo(), "DELETE", "/finance/transactions/3") == (204, "")


@pytest.mark.parametrize(
    ("query", "message"),
    [
        ("account_id=abc", "invalid account_id"),
        ("account_id=0", "invalid account_id"),
        ("category_id=-1", "invalid category_id"),
        ("type=refund", "type must be income, expense, or transfer"),
        ("from=yesterday", "invalid from"),
        ("to=2026-13-01", "invalid to"),
    ],
)
async def test_list_transactions_rejects(query: str, message: str) -> None:
    assert await call(FakeRepo(), "GET", "/finance/transactions?" + query) == err(400, message)


@pytest.mark.parametrize(
    ("body", "message"),
    [
        (NEW_TX | {"type": "invalid"}, "type must be income, expense, or transfer"),
        (NEW_TX | {"type": "transfer"}, "to_account_id is required for transfer"),
        (
            NEW_TX | {"type": "transfer", "to_account_id": 1},
            "to_account_id must differ from account_id",
        ),
        (NEW_TX | {"to_account_id": 2}, "to_account_id must be omitted for income"),
        (NEW_TX | {"account_id": 0}, "account_id is required"),
        (NEW_TX | {"amount": "0"}, "amount must be greater than 0"),
        (NEW_TX | {"category_id": None}, "category_id is required"),
        (NEW_TX | {"amount": "ten"}, "Invalid Body"),
    ],
)
async def test_transaction_validation(body: dict[str, Any], message: str) -> None:
    assert await call(FakeRepo(), "POST", "/finance/transactions", body) == err(400, message)


@pytest.mark.parametrize(
    ("repo", "method", "path", "body", "expected"),
    [
        (
            FakeRepo(get_transaction=NotFoundError()),
            "GET",
            "/finance/transactions/3",
            None,
            err(404, "transaction not found"),
        ),
        (FakeRepo(), "PUT", "/finance/transactions/3", NEW_TX, err(400, "occurred_at is required")),
        (
            FakeRepo(get_category_type="expense"),
            "POST",
            "/finance/transactions",
            NEW_TX,
            err(400, "category type does not match transaction type"),
        ),
        (
            FakeRepo(get_category_type=NotFoundError()),
            "POST",
            "/finance/transactions",
            NEW_TX,
            err(400, "referenced account or category does not exist"),
        ),
        (
            FakeRepo(get_category_type="income", create_transaction=FAIL),
            "POST",
            "/finance/transactions",
            NEW_TX,
            err(500, "Failed to create transaction"),
        ),
        (
            FakeRepo(get_category_type="income", update_transaction=NotFoundError()),
            "PUT",
            "/finance/transactions/3",
            NEW_TX | {"occurred_at": "2026-05-10T12:00:00Z"},
            err(404, "transaction not found"),
        ),
        (
            FakeRepo(delete_transaction=NotFoundError()),
            "DELETE",
            "/finance/transactions/3",
            None,
            err(404, "transaction not found"),
        ),
    ],
)
async def test_transaction_errors(
    repo: FakeRepo, method: str, path: str, body: Any, expected: tuple[int, Any]
) -> None:
    assert await call(repo, method, path, body) == expected


# Overview and stats


async def test_overview_and_stats() -> None:
    repo = Repo(accounts_total=Decimal(0), month_totals=Seq((0, 0), (0, 0)), recent_transactions=[])
    assert (await call(repo, "GET", "/finance/overview"))[0] == 200
    assert await call(FakeRepo(accounts_total=FAIL), "GET", "/finance/overview") == err(
        500, "Failed to get overview"
    )

    repo = FakeRepo(net_worth_series=[])
    q = "?from=2026-01-01&to=2026-03-31&granularity=week"
    assert await call(repo, "GET", "/finance/stats/networth" + q) == (200, [])
    assert repo.calls[0][1] == "week"

    repo = FakeRepo(category_stats=[])
    q = "?type=expense&from=2026-01-01&to=2026-03-31&account_id=2"
    assert (await call(repo, "GET", "/finance/stats/by-category" + q))[0] == 200
    repo = FakeRepo(monthly_stats=[])
    q = "?from=2026-01-01&to=2026-03-31&account_id=2&category_id=3"
    assert (await call(repo, "GET", "/finance/stats/monthly" + q))[0] == 200
    assert repo.calls[0][3:] == (2, 3)

    repo = FakeRepo(earliest_transaction=None)
    q = "?start_month=2026-07&end_month=2026-08-15&mode=saving"
    status, body = await call(repo, "GET", "/finance/stats/estimation" + q)
    assert (status, [p["date"] for p in body["points"]]) == (200, ["2026-07-01", "2026-08-01"])


@pytest.mark.parametrize(
    ("path", "message"),
    [
        ("/finance/stats/networth?from=x", "invalid from"),
        ("/finance/stats/networth?to=x", "invalid to"),
        ("/finance/stats/networth?granularity=year", "granularity must be day, week, or month"),
        ("/finance/stats/by-category", "type must be income, expense, or transfer"),
        ("/finance/stats/by-category?type=income&from=x", "invalid from"),
        ("/finance/stats/by-category?type=income&to=x", "invalid to"),
        ("/finance/stats/by-category?type=income&account_id=x", "invalid account_id"),
        ("/finance/stats/monthly?from=x", "invalid from"),
        ("/finance/stats/monthly?to=x", "invalid to"),
        ("/finance/stats/monthly?account_id=x", "invalid account_id"),
        ("/finance/stats/monthly?category_id=x", "invalid category_id"),
        (
            "/finance/stats/estimation?end_month=2026-05&mode=rate",
            "start_month is required (YYYY-MM)",
        ),
        (
            "/finance/stats/estimation?start_month=2026-05&mode=rate",
            "end_month is required (YYYY-MM)",
        ),
        (
            "/finance/stats/estimation?start_month=2026-05&end_month=2026-04&mode=rate",
            "end_month must be on or after start_month",
        ),
        (
            "/finance/stats/estimation?start_month=2026-05&end_month=2026-06&mode=x",
            "mode must be rate or saving",
        ),
    ],
)
async def test_stats_reject(path: str, message: str) -> None:
    assert await call(FakeRepo(), "GET", path) == err(400, message)


async def test_stats_failures() -> None:
    for path, message in [
        ("/finance/stats/networth?from=2026-01-01", "Failed to compute net worth"),
        (
            "/finance/stats/by-category?type=income&from=2026-01-01",
            "Failed to compute category stats",
        ),
        ("/finance/stats/monthly?from=2026-01-01", "Failed to compute monthly stats"),
    ]:
        repo = FakeRepo(net_worth_series=FAIL, category_stats=FAIL, monthly_stats=FAIL)
        assert await call(repo, "GET", path) == err(500, message)
    q = "?start_month=2026-05&end_month=2026-06&mode=rate"
    assert await call(
        FakeRepo(earliest_transaction=FAIL), "GET", "/finance/stats/estimation" + q
    ) == err(500, "Failed to compute estimation")


# Budgets


def budget_repo() -> Repo:
    return Repo(list_categories=[], effective_budgets=[], category_totals=Seq([], [], [], []))


async def test_get_budgets() -> None:
    status, body = await call(budget_repo(), "GET", "/finance/budgets")
    assert (status, body["month"]) == (200, "2026-05")
    status, body = await call(budget_repo(), "GET", "/finance/budgets?month=2026-03")
    assert (body["month"], body["month_progress"]) == ("2026-03", 1.0)
    assert body["expense"] == {"budgeted": "0", "actual": "0", "unbudgeted": "0", "overspent": "0"}
    assert await call(FakeRepo(), "GET", "/finance/budgets?month=2026-3") == err(
        400, "month must be YYYY-MM"
    )
    assert await call(FakeRepo(list_categories=FAIL), "GET", "/finance/budgets") == err(
        500, "Failed to get budgets"
    )


@pytest.mark.parametrize(
    ("body", "message"),
    [
        ({"amount": "10"}, "month is required (YYYY-MM)"),
        ({"month": "2026-9", "amount": "10"}, "month must be YYYY-MM"),
        ({"month": "2026-09", "amount": "-1"}, "amount must be between 0 and 9999999999999.99"),
        (
            {"month": "2026-09", "amount": "10000000000000"},
            "amount must be between 0 and 9999999999999.99",
        ),
        ({"month": "2026-09", "amount": "10", "scope": "all"}, "scope must be forward or once"),
        (
            {"month": "2026-09", "amount": "10", "period": "weekly"},
            "period must be monthly or yearly",
        ),
        ({"month": "2026", "amount": "10"}, "month must be YYYY-MM"),
        ("{", "Invalid Body"),
    ],
)
async def test_set_budget_validation(body: Any, message: str) -> None:
    assert await call(FakeRepo(), "PUT", "/finance/budgets/3", body) == err(400, message)


async def test_set_budget_defaults_to_forward_and_rounds() -> None:
    repo = FakeRepo(get_category_type="expense")
    body = {"month": "2026-09", "amount": "12.345"}
    assert await call(repo, "PUT", "/finance/budgets/3", body) == (204, "")
    assert repo.calls[1] == (
        "set_budget",
        3,
        "monthly",
        date(2026, 9, 1),
        date(2026, 10, 1),
        Decimal("12.35"),
        "forward",
    )


@pytest.mark.parametrize("month", ["2026", "2026-06"])
async def test_set_yearly_budget(month: str) -> None:
    repo = FakeRepo(get_category_type="expense")
    body = {"month": month, "amount": "450", "period": "yearly", "scope": "once"}
    assert await call(repo, "PUT", "/finance/budgets/3", body) == (204, "")
    assert repo.calls[1][2:5] == ("yearly", date(2026, 1, 1), date(2027, 1, 1))


async def test_set_budget_errors() -> None:
    body = {"month": "2026-09", "amount": "10"}
    assert await call(
        FakeRepo(get_category_type=NotFoundError()), "PUT", "/finance/budgets/3", body
    ) == err(404, "category not found")
    assert await call(
        FakeRepo(get_category_type="transfer"), "PUT", "/finance/budgets/3", body
    ) == err(400, "transfer categories cannot be budgeted")
    repo = FakeRepo(get_category_type="expense", set_budget=FAIL)
    assert await call(repo, "PUT", "/finance/budgets/3", body) == err(500, "Failed to save budget")


async def test_delete_budget() -> None:
    repo = FakeRepo(get_category_type="expense")
    path = "/finance/budgets/3?month=2026&period=yearly&scope=once"
    assert await call(repo, "DELETE", path) == (204, "")
    assert repo.calls[1][2:] == ("yearly", date(2026, 1, 1), date(2027, 1, 1), None, "once")
    assert await call(FakeRepo(), "DELETE", "/finance/budgets/3") == err(
        400, "month is required (YYYY-MM)"
    )


async def test_budget_transactions() -> None:
    repo = FakeRepo(
        list_categories=BUDGET_CATS, effective_budgets=BUDGETS, category_transactions=[]
    )
    path = "/finance/budgets/1/transactions?month=2026-03&period=monthly"
    assert await call(repo, "GET", path) == (200, [])
    for path in [
        "/finance/budgets/1/transactions?period=weekly",
        "/finance/budgets/1/transactions?month=03-2026",
        "/finance/budgets/abc/transactions",
    ]:
        assert (await call(FakeRepo(), "GET", path))[0] == 400, path
    repo = FakeRepo(list_categories=BUDGET_CATS)
    assert await call(repo, "GET", "/finance/budgets/99/transactions") == err(
        404, "category not found"
    )
