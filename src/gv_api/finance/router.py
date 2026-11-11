import re
from collections.abc import Awaitable
from datetime import UTC, date, datetime, time, timedelta
from decimal import Decimal
from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.core.dates import parse_day, parse_instant
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import ApiError, error, internal_error, parse_body, parse_id, query
from gv_api.finance.errors import (
    AccountInUseError,
    BudgetTransferError,
    CategoryInUseError,
    CategoryMismatchError,
    InvalidInputError,
    NotFoundError,
)
from gv_api.finance.schemas import (
    TX_TYPES,
    AccountRequest,
    BudgetRequest,
    CategoryRequest,
    TransactionRequest,
)
from gv_api.finance.service import FinanceService, cents

router = full_router()
Svc = Annotated[FinanceService, Depends(service(FinanceService))]

MAX_NAME = 40
MAX_BUDGET = Decimal("9999999999999.99")  # the largest NUMERIC(15,2)
BAD_TYPE = "type must be income, expense, or transfer"
GRANULARITIES = ("day", "week", "month")
SCOPES = ("forward", "once", "month")

_MONTH = re.compile(r"(\d{4})-(\d{2})")
_YEAR = re.compile(r"\d{4}")


async def _answer(
    call: Awaitable[object],
    failure: str,
    not_found: str | None = None,
    status: int = 200,
) -> Response:
    try:
        result = await call
    except NotFoundError as err:
        if not_found is None:
            return internal_error(err, failure)
        return error(404, not_found)
    except Exception as err:
        return internal_error(err, failure)
    if result is None:
        return Response(status_code=204)
    return respond(status, result)


# Query parsing. Each raises ValueError, which the handler answers with its own message.


def _instant(raw: str) -> datetime | None:
    return parse_instant(raw) if raw else None


def _instant_end(raw: str) -> datetime | None:
    """Like _instant, but a plain date means the end of that day."""
    if not raw:
        return None
    try:
        day = parse_day(raw)
    except ValueError:
        return parse_instant(raw)
    return datetime.combine(day, time(), UTC) + timedelta(days=1, microseconds=-1)


def _optional_id(raw: str) -> int | None:
    if not raw:
        return None
    try:
        return parse_id(raw, "")
    except ApiError:
        raise ValueError(raw) from None


def _month(raw: str) -> date:
    """YYYY-MM or YYYY-MM-DD."""
    if m := _MONTH.fullmatch(raw):
        return date(int(m[1]), int(m[2]), 1)
    return parse_day(raw)


def _budget_month(raw: str, required: bool, period: str = "monthly") -> date | None:
    """YYYY-MM, or a bare YYYY for yearly budgets. Empty is the current month."""
    if not raw:
        if required:
            raise ApiError(400, "month is required (YYYY-MM)")
        return None
    try:
        if period == "yearly" and _YEAR.fullmatch(raw):
            return date(int(raw), 1, 1)
        if m := _MONTH.fullmatch(raw):
            return date(int(m[1]), int(m[2]), 1)
    except ValueError:
        pass
    raise ApiError(400, "month must be YYYY-MM")


def _period(raw: str) -> str:
    if raw in ("", "monthly"):
        return "monthly"
    if raw == "yearly":
        return raw
    raise ApiError(400, "period must be monthly or yearly")


def _scope(raw: str) -> str:
    if not raw:
        return "forward"
    if raw not in SCOPES:
        raise ApiError(400, "scope must be forward or once")
    return raw


# Validation


def _name(raw: str) -> str:
    name = raw.strip()
    if not name:
        raise ApiError(400, "name is required")
    if len(name) > MAX_NAME:
        raise ApiError(400, "name must be at most 40 characters")
    return name


def _check_category(req: CategoryRequest, self_id: int = 0) -> str:
    name = _name(req.name)
    if req.type not in TX_TYPES:
        raise ApiError(400, BAD_TYPE)
    if req.parent_id is not None and self_id and req.parent_id == self_id:
        raise ApiError(400, "parent_id must not equal id")
    return name


def _check_transaction(req: TransactionRequest) -> None:
    if req.type not in TX_TYPES:
        raise ApiError(400, BAD_TYPE)
    if req.type == "transfer":
        if req.to_account_id is None:
            raise ApiError(400, "to_account_id is required for transfer")
        if req.to_account_id == req.account_id:
            raise ApiError(400, "to_account_id must differ from account_id")
    elif req.to_account_id is not None:
        raise ApiError(400, f"to_account_id must be omitted for {req.type}")
    if req.account_id <= 0:
        raise ApiError(400, "account_id is required")
    if req.amount <= 0:
        raise ApiError(400, "amount must be greater than 0")
    if req.category_id is None:
        raise ApiError(400, "category_id is required")


async def _transaction(call: Awaitable[object], op: str, status: int = 200) -> Response:
    try:
        return respond(status, await call)
    except NotFoundError:
        return error(404, "transaction not found")
    except CategoryMismatchError:
        return error(400, "category type does not match transaction type")
    except InvalidInputError:
        return error(400, "referenced account or category does not exist")
    except Exception as err:
        return internal_error(err, f"Failed to {op} transaction")


# Accounts


@router.get("/finance/accounts")
async def list_accounts(svc: Svc) -> Response:
    return await _answer(svc.list_accounts(), "Failed to list accounts")


@router.get("/finance/accounts/{id}")
async def get_account(id: str, svc: Svc) -> Response:
    account_id = parse_id(id, "account")
    return await _answer(svc.get_account(account_id), "Failed to get account", "account not found")


@router.post("/finance/accounts")
async def create_account(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, AccountRequest, "Invalid Body")
    return await _answer(
        svc.create_account(_name(req.name)), "Failed to create account", status=201
    )


@router.put("/finance/accounts/{id}")
async def update_account(id: str, request: Request, svc: Svc) -> Response:
    account_id = parse_id(id, "account")
    req = await parse_body(request, AccountRequest, "Invalid Body")
    return await _answer(
        svc.update_account(account_id, _name(req.name)),
        "Failed to update account",
        "account not found",
    )


@router.delete("/finance/accounts/{id}")
async def delete_account(id: str, svc: Svc) -> Response:
    account_id = parse_id(id, "account")
    try:
        await svc.delete_account(account_id)
    except AccountInUseError:
        return error(409, "account has transactions; delete them first")
    except Exception as err:
        return internal_error(err, "Failed to delete account")
    return Response(status_code=204)


@router.get("/finance/overview")
async def get_overview(svc: Svc) -> Response:
    return await _answer(svc.get_overview(), "Failed to get overview")


# Categories


@router.get("/finance/categories")
async def list_categories(svc: Svc) -> Response:
    return await _answer(svc.list_categories(), "Failed to list categories")


@router.get("/finance/categories/{id}")
async def get_category(id: str, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    return await _answer(
        svc.get_category(category_id), "Failed to get category", "category not found"
    )


@router.post("/finance/categories")
async def create_category(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CategoryRequest, "Invalid Body")
    name = _check_category(req)
    try:
        return respond(201, await svc.create_category(name, req.parent_id, req.type))
    except InvalidInputError:
        return error(400, "parent_id is invalid")
    except Exception as err:
        return internal_error(err, "Failed to create category")


@router.put("/finance/categories/{id}")
async def update_category(id: str, request: Request, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    req = await parse_body(request, CategoryRequest, "Invalid Body")
    name = _check_category(req, category_id)
    try:
        return respond(200, await svc.update_category(category_id, name, req.parent_id, req.type))
    except NotFoundError:
        return error(404, "category not found")
    except InvalidInputError:
        return error(400, "parent_id is invalid")
    except Exception as err:
        return internal_error(err, "Failed to update category")


@router.delete("/finance/categories/{id}")
async def delete_category(id: str, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    try:
        await svc.delete_category(category_id)
    except CategoryInUseError:
        return error(409, "category is referenced by transactions or other categories")
    except Exception as err:
        return internal_error(err, "Failed to delete category")
    return Response(status_code=204)


# Transactions


@router.get("/finance/transactions")
async def list_transactions(request: Request, svc: Svc) -> Response:
    try:
        account_id = _optional_id(query(request, "account_id"))
    except ValueError:
        return error(400, "invalid account_id")
    try:
        category_id = _optional_id(query(request, "category_id"))
    except ValueError:
        return error(400, "invalid category_id")
    type = query(request, "type") or None
    if type is not None and type not in TX_TYPES:
        return error(400, BAD_TYPE)
    try:
        start = _instant(query(request, "from"))
    except ValueError:
        return error(400, "invalid from")
    try:
        end = _instant_end(query(request, "to"))
    except ValueError:
        return error(400, "invalid to")
    return await _answer(
        svc.list_transactions(account_id, category_id, type, start, end),
        "Failed to list transactions",
    )


@router.get("/finance/transactions/{id}")
async def get_transaction(id: str, svc: Svc) -> Response:
    tx_id = parse_id(id, "transaction")
    return await _answer(
        svc.get_transaction(tx_id), "Failed to get transaction", "transaction not found"
    )


@router.post("/finance/transactions")
async def create_transaction(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, TransactionRequest, "Invalid Body")
    _check_transaction(req)
    return await _transaction(svc.create_transaction(req), "create", 201)


@router.put("/finance/transactions/{id}")
async def update_transaction(id: str, request: Request, svc: Svc) -> Response:
    tx_id = parse_id(id, "transaction")
    req = await parse_body(request, TransactionRequest, "Invalid Body")
    _check_transaction(req)
    if req.occurred_at is None:
        return error(400, "occurred_at is required")
    return await _transaction(svc.update_transaction(tx_id, req), "update")


@router.delete("/finance/transactions/{id}")
async def delete_transaction(id: str, svc: Svc) -> Response:
    tx_id = parse_id(id, "transaction")
    return await _answer(
        svc.delete_transaction(tx_id), "Failed to delete transaction", "transaction not found"
    )


# Stats


@router.get("/finance/stats/networth")
async def net_worth(request: Request, svc: Svc) -> Response:
    try:
        start = _instant(query(request, "from"))
    except ValueError:
        return error(400, "invalid from")
    try:
        end = _instant_end(query(request, "to"))
    except ValueError:
        return error(400, "invalid to")
    granularity = query(request, "granularity")
    if granularity and granularity not in GRANULARITIES:
        return error(400, "granularity must be day, week, or month")
    return await _answer(svc.net_worth(start, end, granularity), "Failed to compute net worth")


@router.get("/finance/stats/by-category")
async def category_stats(request: Request, svc: Svc) -> Response:
    type = query(request, "type")
    if type not in TX_TYPES:
        return error(400, BAD_TYPE)
    try:
        start = _instant(query(request, "from"))
    except ValueError:
        return error(400, "invalid from")
    try:
        end = _instant_end(query(request, "to"))
    except ValueError:
        return error(400, "invalid to")
    try:
        account_id = _optional_id(query(request, "account_id"))
    except ValueError:
        return error(400, "invalid account_id")
    return await _answer(
        svc.category_stats(type, start, end, account_id), "Failed to compute category stats"
    )


@router.get("/finance/stats/monthly")
async def monthly_stats(request: Request, svc: Svc) -> Response:
    try:
        start = _instant(query(request, "from"))
    except ValueError:
        return error(400, "invalid from")
    try:
        end = _instant_end(query(request, "to"))
    except ValueError:
        return error(400, "invalid to")
    try:
        account_id = _optional_id(query(request, "account_id"))
    except ValueError:
        return error(400, "invalid account_id")
    try:
        category_id = _optional_id(query(request, "category_id"))
    except ValueError:
        return error(400, "invalid category_id")
    return await _answer(
        svc.monthly_stats(start, end, account_id, category_id), "Failed to compute monthly stats"
    )


@router.get("/finance/stats/estimation")
async def estimation(request: Request, svc: Svc) -> Response:
    try:
        start = _month(query(request, "start_month"))
    except ValueError:
        return error(400, "start_month is required (YYYY-MM)")
    try:
        end = _month(query(request, "end_month"))
    except ValueError:
        return error(400, "end_month is required (YYYY-MM)")
    if end < start:
        return error(400, "end_month must be on or after start_month")
    mode = query(request, "mode")
    if mode not in ("rate", "saving"):
        return error(400, "mode must be rate or saving")
    return await _answer(svc.estimation(start, end, mode), "Failed to compute estimation")


# Budgets


async def _budget(call: Awaitable[object], failure: str) -> Response:
    try:
        result = await call
    except NotFoundError:
        return error(404, "category not found")
    except BudgetTransferError:
        return error(400, "transfer categories cannot be budgeted")
    except Exception as err:
        return internal_error(err, failure)
    if result is None:
        return Response(status_code=204)
    return respond(200, result)


@router.get("/finance/budgets")
async def get_budgets(request: Request, svc: Svc) -> Response:
    month = _budget_month(query(request, "month"), required=False)
    return await _answer(svc.budget_month(month), "Failed to get budgets")


@router.get("/finance/budgets/{id}/transactions")
async def get_budget_transactions(id: str, request: Request, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    period = _period(query(request, "period"))
    month = _budget_month(query(request, "month"), required=False)
    return await _budget(
        svc.budget_transactions(category_id, period, month), "Failed to list budget transactions"
    )


@router.put("/finance/budgets/{id}")
async def set_budget(id: str, request: Request, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    req = await parse_body(request, BudgetRequest, "Invalid Body")
    period = _period(req.period)
    month = _budget_month(req.month, required=True, period=period)
    scope = _scope(req.scope)
    if req.amount < 0 or req.amount > MAX_BUDGET:
        return error(400, "amount must be between 0 and 9999999999999.99")
    assert month is not None  # noqa: S101 - required above
    return await _budget(
        svc.set_budget(category_id, period, month, cents(req.amount), scope),
        "Failed to save budget",
    )


@router.delete("/finance/budgets/{id}")
async def delete_budget(id: str, request: Request, svc: Svc) -> Response:
    category_id = parse_id(id, "category")
    period = _period(query(request, "period"))
    month = _budget_month(query(request, "month"), required=True, period=period)
    scope = _scope(query(request, "scope"))
    assert month is not None  # noqa: S101 - required above
    return await _budget(
        svc.set_budget(category_id, period, month, None, scope), "Failed to save budget"
    )
