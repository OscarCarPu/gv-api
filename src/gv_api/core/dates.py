"""Date helpers: strict YYYY-MM-DD parsing, month arithmetic and "today" in a zone."""

import re
from calendar import monthrange
from collections.abc import Callable
from datetime import UTC, date, datetime
from zoneinfo import ZoneInfo

Clock = Callable[[], datetime]

_DAY = re.compile(r"(\d{4})-(\d{2})-(\d{2})")


def utcnow() -> datetime:
    return datetime.now(UTC)


def parse_day(raw: str) -> date:
    """Exactly YYYY-MM-DD; ValueError on anything else."""
    m = _DAY.fullmatch(raw)
    if m is None:
        raise ValueError(f"invalid date {raw!r}: want YYYY-MM-DD")
    return date(int(m[1]), int(m[2]), int(m[3]))


def add_months(d: date, months: int) -> date:
    """d shifted by whole months, clamped to the last day of the target month
    (Mar 31 - 1 month = Feb 28)."""
    month0 = d.month - 1 + months
    year, month = d.year + month0 // 12, month0 % 12 + 1
    return d.replace(year=year, month=month, day=min(d.day, monthrange(year, month)[1]))


def today_in(loc: ZoneInfo, now: Clock = utcnow) -> date:
    return now().astimezone(loc).date()
