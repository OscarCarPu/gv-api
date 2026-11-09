import json
import logging
from datetime import datetime
from decimal import Decimal

import pytest
from starlette.requests import Request

from gv_api.core.http import (
    ApiError,
    Body,
    Int32,
    InTime,
    error,
    error_code,
    internal_error,
    parse_body,
    parse_id,
    query,
)


def make_request(body: bytes = b"", qs: str = "") -> Request:
    async def receive() -> dict[str, object]:
        return {"type": "http.request", "body": body, "more_body": False}

    scope = {
        "type": "http",
        "method": "POST",
        "path": "/",
        "headers": [],
        "query_string": qs.encode(),
    }
    return Request(scope, receive)


class LogReq(Body):
    habit_id: Int32 = 0
    date: str = ""
    value: float = 0.0


class TxReq(Body):
    amount: Decimal = Decimal(0)
    occurred_at: InTime | None = None
    description: str | None = None


async def test_unknown_fields_ignored() -> None:
    req = await parse_body(make_request(b'{"habit_id": 1, "foo": 1}'), LogReq, "Invalid Body")
    assert req.habit_id == 1


async def test_missing_and_null_fields_take_zero_values() -> None:
    req = await parse_body(make_request(b'{"habit_id": null}'), LogReq, "Invalid Body")
    assert (req.habit_id, req.date, req.value) == (0, "", 0.0)


async def test_null_body_is_all_zero_values() -> None:
    req = await parse_body(make_request(b"null"), LogReq, "Invalid Body")
    assert req.habit_id == 0


async def test_trailing_data_after_first_value_is_ignored() -> None:
    req = await parse_body(make_request(b' {"habit_id": 2} garbage'), LogReq, "Invalid Body")
    assert req.habit_id == 2


@pytest.mark.parametrize(
    "body",
    [b"", b"{", b"[]", b'"x"', b'{"habit_id": "abc"}', b'{"habit_id": 4294967296}', b"\xff"],
)
async def test_bad_bodies_give_the_handler_message(body: bytes) -> None:
    with pytest.raises(ApiError) as err:
        await parse_body(make_request(body), LogReq, "Invalid Body")
    assert (err.value.status, err.value.message) == (400, "Invalid Body")


async def test_decimal_from_number_or_string_and_aware_times() -> None:
    a = await parse_body(make_request(b'{"amount": 12.5}'), TxReq, "x")
    b = await parse_body(make_request(b'{"amount": "12.50"}'), TxReq, "x")
    assert a.amount == b.amount == Decimal("12.5")
    c = await parse_body(make_request(b'{"occurred_at": "2026-01-01T10:00:00+01:00"}'), TxReq, "x")
    assert isinstance(c.occurred_at, datetime)
    assert c.occurred_at.utcoffset() is not None


@pytest.mark.parametrize("raw", ['"2026-01-01T10:00:00"', '"2026-01-01"', '"yesterday"'])
async def test_times_without_zone_are_rejected_like_go(raw: str) -> None:
    with pytest.raises(ApiError):
        await parse_body(make_request(b'{"occurred_at": ' + raw.encode() + b"}"), TxReq, "x")


async def test_number_into_string_field_is_rejected() -> None:
    with pytest.raises(ApiError):
        await parse_body(make_request(b'{"date": 5}'), LogReq, "Invalid Body")


@pytest.mark.parametrize(
    ("raw", "want"), [("1", 1), ("+5", 5), ("05", 5), ("2147483647", 2147483647)]
)
def test_parse_id_valid(raw: str, want: int) -> None:
    assert parse_id(raw, "habit") == want


@pytest.mark.parametrize(
    "raw", ["abc", "0", "-1", "4294967296", "2147483648", " 5", "5_0", "", "1.0"]
)
def test_parse_id_invalid(raw: str) -> None:
    with pytest.raises(ApiError) as err:
        parse_id(raw, "habit")
    assert (err.value.status, err.value.message) == (400, "invalid habit id")


def test_query_first_value_or_empty() -> None:
    req = make_request(qs="date=2026-01-01&date=2026-02-02&empty=")
    assert query(req, "date") == "2026-01-01"
    assert query(req, "empty") == ""
    assert query(req, "missing") == ""


def test_error_bodies() -> None:
    assert json.loads(error(400, "name is required").body) == {"error": "name is required"}
    body = json.loads(error_code(503, "merge_pending", "wait").body)
    assert body == {"error": "wait", "code": "merge_pending"}


def test_internal_error_logs_cause_but_returns_message(caplog: pytest.LogCaptureFixture) -> None:
    with caplog.at_level(logging.ERROR):
        resp = internal_error(RuntimeError("db down"), "Failed to get daily habits")
    assert resp.status_code == 500
    assert json.loads(resp.body) == {"error": "Failed to get daily habits"}
    assert caplog.records[0].getMessage() == "Failed to get daily habits"
    assert caplog.records[0].__dict__["error"] == "db down"
