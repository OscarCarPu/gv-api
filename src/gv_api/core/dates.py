"""Go's date handling where Python's differs: strict parsing and AddDate's normalization."""

import re
from collections.abc import Callable
from datetime import UTC, date, datetime, timedelta
from zoneinfo import ZoneInfo

Clock = Callable[[], datetime]

_DAY = re.compile(r"(\d{4})-(\d{2})-(\d{2})")


def utcnow() -> datetime:
    return datetime.now(UTC)


def parse_day(raw: str) -> date:
    """time.Parse("2006-01-02", raw); ValueError on anything else."""
    m = _DAY.fullmatch(raw)
    if m is None:
        raise ValueError(f'parsing time "{raw}" as "2006-01-02": cannot parse')
    return date(int(m[1]), int(m[2]), int(m[3]))


def add_date(d: date, years: int = 0, months: int = 0, days: int = 0) -> date:
    """time.AddDate: an overflowing day rolls into the next month (Mar 31 - 1 month = Mar 3)."""
    month0 = d.month - 1 + months
    year = d.year + years + month0 // 12
    first = date(year, month0 % 12 + 1, 1)
    return first + timedelta(days=d.day - 1 + days)


def today_in(loc: ZoneInfo, now: Clock = utcnow) -> date:
    return now().astimezone(loc).date()
