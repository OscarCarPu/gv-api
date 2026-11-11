import json
from datetime import UTC, date, datetime, timedelta, timezone
from decimal import Decimal
from zoneinfo import ZoneInfo

import pytest
from pydantic import Field
from pydantic_core import TzInfo

from gv_api.core.encoding import (
    ApiModel,
    Day,
    Money,
    Real,
    Timestamp,
    real_number,
    respond,
)

MADRID = ZoneInfo("Europe/Madrid")


def body(value: object) -> object:
    return json.loads(respond(200, value).body)


@pytest.mark.parametrize(
    ("raw", "want"),
    [("12.50", "12.50"), ("100", "100"), ("0.05", "0.05"), ("-12.30", "-12.30")],
)
def test_amount_is_a_string_with_its_scale(raw: str, want: str) -> None:
    assert body({"v": Decimal(raw)}) == {"v": want}


@pytest.mark.parametrize(
    ("value", "want"),
    [
        (datetime(2026, 11, 9, 9, 0, tzinfo=UTC), "2026-11-09T09:00:00Z"),
        (datetime(2026, 11, 9, 9, 0, 0, 120000, tzinfo=UTC), "2026-11-09T09:00:00.120000Z"),
        (datetime(2026, 11, 9, 9, 0, tzinfo=ZoneInfo("UTC")), "2026-11-09T09:00:00Z"),
        (datetime(2026, 11, 9, 9, 0, tzinfo=TzInfo(0)), "2026-11-09T09:00:00Z"),
        (datetime(2026, 11, 9, 10, 0, tzinfo=MADRID), "2026-11-09T10:00:00+01:00"),
        (
            datetime(2026, 1, 1, 0, 0, tzinfo=timezone(timedelta(hours=-3, minutes=-30))),
            "2026-01-01T00:00:00-03:30",
        ),
    ],
)
def test_instant_is_rfc3339(value: datetime, want: str) -> None:
    assert body({"v": value}) == {"v": want}


class Row(ApiModel):
    id: int
    amount: Money
    maybe_amount: Money | None = None
    at: Timestamp
    day: Day
    tags: list[str] | None = None
    items: list[int] = Field(default_factory=list)


def test_model_shapes() -> None:
    row = Row.model_construct(
        id=1,
        amount=Decimal("12.50"),
        maybe_amount=None,
        at=datetime(2026, 11, 9, 9, 0, tzinfo=UTC),
        day=date(2026, 11, 9),
        tags=None,
        items=[],
    )
    assert body(row) == {
        "id": 1,
        "amount": "12.50",
        "maybe_amount": None,
        "at": "2026-11-09T09:00:00Z",
        "day": "2026-11-09",
        "tags": None,
        "items": [],
    }


def test_api_model_ignores_unknown_fields() -> None:
    class Req(ApiModel):
        name: str = ""

    assert Req.model_validate({"name": "x", "foo": 1}).name == "x"


def test_respond_encodes_nested_values() -> None:
    resp = respond(201, {"total": Decimal("3.10"), "rows": [{"day": date(2026, 1, 2)}], "x": None})
    assert resp.status_code == 201
    assert resp.headers["content-type"] == "application/json"
    assert json.loads(resp.body) == {"total": "3.10", "rows": [{"day": "2026-01-02"}], "x": None}


def test_respond_list_of_models() -> None:
    class Item(ApiModel):
        at: Timestamp

    resp = respond(200, [Item(at=datetime(2026, 1, 1, tzinfo=UTC))])
    assert json.loads(resp.body) == [{"at": "2026-01-01T00:00:00Z"}]


def test_respond_keeps_non_ascii() -> None:
    resp = respond(200, {"name": "Café <b>"})
    assert json.loads(resp.body) == {"name": "Café <b>"}


# A real column comes back as the double nearest the stored float: 7 significant digits.
@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (64.22000122070312, 64.22),
        (0.10000000149011612, 0.1),
        (70.30000305175781, 70.3),
        (5.0, 5),
        (0.0, 0),
        (-2.5, -2.5),
        (1.0000000116860974e-07, 1e-7),
    ],
)
def test_real(value: float, expected: float) -> None:
    class M(ApiModel):
        v: Real

    assert json.loads(respond(200, M(v=value)).body) == {"v": expected}
    assert real_number(value) == expected
