"""From internal/history/history_test.go, plus Go's date parsing and AddDate."""

from datetime import UTC, date, datetime, timedelta
from zoneinfo import ZoneInfo

import pytest

from gv_api import history
from gv_api.core.dates import add_date, parse_day, today_in

UTC_ZONE = ZoneInfo("UTC")


def d(iso: str) -> date:
    return date.fromisoformat(iso)


@pytest.mark.parametrize(
    ("day", "frequency", "expected"),
    [
        ("2026-03-17", "daily", "2026-03-17"),
        ("2026-03-19", "weekly", "2026-03-16"),
        ("2026-03-16", "weekly", "2026-03-16"),
        ("2026-03-22", "weekly", "2026-03-16"),
        ("2026-03-17", "monthly", "2026-03-01"),
        ("2026-03-01", "monthly", "2026-03-01"),
    ],
)
def test_period_start(day: str, frequency: str, expected: str) -> None:
    assert history.period_start(d(day), frequency) == d(expected)


@pytest.mark.parametrize(
    ("day", "frequency", "expected"),
    [
        ("2026-03-16", "weekly", "2026-03-16"),
        ("2026-03-17", "daily", "2026-03-17"),
        ("2026-03-01", "monthly", "2026-03-01"),
        ("2026-03-18", "weekly", "2026-03-23"),
        ("2026-03-15", "monthly", "2026-04-01"),
        ("2026-12-15", "monthly", "2027-01-01"),
    ],
)
def test_period_ceil(day: str, frequency: str, expected: str) -> None:
    assert history.period_ceil(d(day), frequency) == d(expected)


@pytest.mark.parametrize(
    ("day", "frequency", "expected"),
    [
        ("2026-03-17", "daily", "2026-03-18"),
        ("2026-03-16", "weekly", "2026-03-23"),
        ("2026-03-01", "monthly", "2026-04-01"),
    ],
)
def test_next_period_start(day: str, frequency: str, expected: str) -> None:
    assert history.next_period_start(d(day), frequency) == d(expected)


@pytest.mark.parametrize(
    ("frequency", "expected"),
    [("daily", "2026-02-17"), ("weekly", "2025-12-23"), ("monthly", "2025-03-17")],
)
def test_default_start_date(frequency: str, expected: str) -> None:
    assert history.default_start_date(d("2026-03-17"), frequency) == d(expected)


def test_default_start_date_rolls_over_like_go() -> None:
    # time.AddDate(0, -1, 0) on Mar 31 is "Feb 31", which Go normalizes to Mar 3.
    assert history.default_start_date(d("2026-03-31"), "daily") == d("2026-03-03")
    assert history.default_start_date(d("2028-02-29"), "monthly") == d("2027-03-01")


@pytest.mark.parametrize(
    ("frequency", "trunc"), [("daily", "day"), ("weekly", "week"), ("monthly", "month")]
)
def test_valid_frequency(frequency: str, trunc: str) -> None:
    assert history.valid_frequency(frequency) == trunc


def test_invalid_frequency() -> None:
    with pytest.raises(ValueError, match="invalid frequency: yearly"):
        history.valid_frequency("yearly")


def fixed(iso: str) -> datetime:
    return datetime.fromisoformat(iso)


def test_parse_date_range_explicit() -> None:
    assert history.parse_date_range(UTC_ZONE, "daily", "2026-03-10", "2026-03-17") == (
        d("2026-03-10"),
        d("2026-03-17"),
    )


def test_parse_date_range_defaults() -> None:
    def now() -> datetime:
        return fixed("2026-03-17T12:00:00+00:00")

    assert history.parse_date_range(UTC_ZONE, "daily", "", "", now) == (
        d("2026-02-17"),
        d("2026-03-17"),
    )


def test_parse_date_range_today_is_in_the_zone() -> None:
    def now() -> datetime:
        return fixed("2026-03-17T23:30:00+00:00")

    _, end = history.parse_date_range(ZoneInfo("Europe/Madrid"), "daily", "", "", now)
    assert end == d("2026-03-18")


@pytest.mark.parametrize(
    ("start_at", "end_at", "message"),
    [("bad", "", "invalid start_at date"), ("", "bad", "invalid end_at date")],
)
def test_parse_date_range_invalid(start_at: str, end_at: str, message: str) -> None:
    with pytest.raises(ValueError, match=message):
        history.parse_date_range(UTC_ZONE, "daily", start_at, end_at)


def test_parse_date_range_snaps_to_periods() -> None:
    assert history.parse_date_range(UTC_ZONE, "weekly", "2026-03-18", "2026-03-18") == (
        d("2026-03-16"),
        d("2026-03-23"),
    )


@pytest.mark.parametrize("raw", ["2026-3-17", "20260317", "2026-03-17T00:00:00", "", "2026-02-30"])
def test_parse_day_is_strict(raw: str) -> None:
    with pytest.raises(ValueError):
        parse_day(raw)


def test_parse_day() -> None:
    assert parse_day("2026-03-17") == d("2026-03-17")


@pytest.mark.parametrize(
    ("args", "expected"),
    [
        ((0, 1, 0), "2026-05-01"),  # "Apr 31"
        ((0, -1, 0), "2026-03-03"),
        ((0, 0, 1), "2026-04-01"),
        ((1, 0, 0), "2027-03-31"),
        ((0, -15, 0), "2024-12-31"),
    ],
)
def test_add_date(args: tuple[int, int, int], expected: str) -> None:
    assert add_date(d("2026-03-31"), *args) == d(expected)


def test_today_in() -> None:
    now = datetime(2026, 3, 17, 23, 30, tzinfo=UTC)
    assert today_in(ZoneInfo("Europe/Madrid"), lambda: now) == d("2026-03-18")
    assert today_in(UTC_ZONE, lambda: now) == d("2026-03-17")
    assert today_in(UTC_ZONE) - datetime.now(UTC).date() <= timedelta(days=1)
