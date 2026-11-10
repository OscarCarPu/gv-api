import json
from datetime import UTC, date, datetime, timedelta, timezone
from decimal import Decimal
from zoneinfo import ZoneInfo

import pytest
from pydantic import Field

from gv_api.core.encoding import (
    ApiModel,
    Day,
    Float32,
    Money,
    Timestamp,
    go_decimal,
    go_float32,
    go_time,
    respond,
)

MADRID = ZoneInfo("Europe/Madrid")


# Expected values are what Go's encoding/json + shopspring/decimal print for the same input.
@pytest.mark.parametrize(
    ("raw", "want"),
    [
        ("12.50", "12.5"),
        ("100.00", "100"),
        ("100", "100"),
        ("1E+2", "100"),
        ("0.05", "0.05"),
        ("0.00", "0"),
        ("-0.00", "0"),
        ("-12.30", "-12.3"),
        ("1234567.890", "1234567.89"),
        ("7.25", "7.25"),
    ],
)
def test_go_decimal(raw: str, want: str) -> None:
    assert go_decimal(Decimal(raw)) == want


@pytest.mark.parametrize(
    ("value", "want"),
    [
        (datetime(2026, 11, 9, 9, 0, tzinfo=UTC), "2026-11-09T09:00:00Z"),
        (datetime(2026, 11, 9, 9, 0, 0, 120000, tzinfo=UTC), "2026-11-09T09:00:00.12Z"),
        (datetime(2026, 11, 9, 19, 16, 17, 993807, tzinfo=UTC), "2026-11-09T19:16:17.993807Z"),
        (datetime(2026, 11, 9, 9, 0, tzinfo=ZoneInfo("UTC")), "2026-11-09T09:00:00Z"),
        (datetime(2026, 11, 9, 10, 0, tzinfo=MADRID), "2026-11-09T10:00:00+01:00"),
        (datetime(2026, 7, 1, 10, 0, 0, 500000, tzinfo=MADRID), "2026-07-01T10:00:00.5+02:00"),
        (
            datetime(2026, 1, 1, 0, 0, tzinfo=timezone(timedelta(hours=-3, minutes=-30))),
            "2026-01-01T00:00:00-03:30",
        ),
        (
            datetime(2026, 1, 1, 12, 0, tzinfo=ZoneInfo("Europe/London")),
            "2026-01-01T12:00:00+00:00",
        ),
    ],
)
def test_go_time(value: datetime, want: str) -> None:
    assert go_time(value) == want


class Row(ApiModel):
    id: int
    amount: Money
    maybe_amount: Money | None = None
    at: Timestamp
    day: Day
    tags: list[str] | None = None
    items: list[int] = Field(default_factory=list)


def test_model_dump_matches_go_shapes() -> None:
    row = Row.model_construct(
        id=1,
        amount=Decimal("12.50"),
        maybe_amount=None,
        at=datetime(2026, 11, 9, 9, 0, tzinfo=UTC),
        day=date(2026, 11, 9),
        tags=None,
        items=[],
    )
    assert row.model_dump(mode="json") == {
        "id": 1,
        "amount": "12.5",
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
    assert json.loads(resp.body) == {"total": "3.1", "rows": [{"day": "2026-01-02"}], "x": None}
    assert resp.body.endswith(b"\n")


def test_respond_list_of_models() -> None:
    class Item(ApiModel):
        at: Timestamp

    resp = respond(200, [Item(at=datetime(2026, 1, 1, tzinfo=UTC))])
    assert json.loads(resp.body) == [{"at": "2026-01-01T00:00:00Z"}]


def test_respond_keeps_non_ascii() -> None:
    resp = respond(200, {"name": "Café <b>"})
    assert json.loads(resp.body) == {"name": "Café <b>"}


# Values from Go: json.Marshal(float32(x)).
@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (64.22000122070312, "64.22"),
        (0.10000000149011612, "0.1"),
        (5.0, "5"),
        (0.0, "0"),
        (-2.5, "-2.5"),
        (16777216.0, "16777216"),
        (1.0000000116860974e-07, "1e-7"),
        (3.4028234663852886e38, "3.4028235e+38"),
    ],
)
def test_float32(value: float, expected: str) -> None:
    class M(ApiModel):
        v: Float32

    assert json.loads(respond(200, M(v=value)).body) == {"v": json.loads(expected)}
    assert go_float32(value) == json.loads(expected)
