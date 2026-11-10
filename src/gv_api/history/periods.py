"""Period maths shared by the history endpoints (habits, time entries)."""

from datetime import date, timedelta
from zoneinfo import ZoneInfo

from gv_api.core.dates import Clock, add_months, parse_day, today_in, utcnow

_TRUNC = {"daily": "day", "weekly": "week", "monthly": "month"}


def valid_frequency(frequency: str) -> str:
    """The date_trunc unit for an API frequency."""
    try:
        return _TRUNC[frequency]
    except KeyError:
        raise ValueError(f"invalid frequency: {frequency}") from None


def default_start_date(today: date, frequency: str) -> date:
    if frequency == "weekly":
        return today - timedelta(weeks=12)
    if frequency == "monthly":
        return add_months(today, -12)
    return add_months(today, -1)


def period_start(d: date, frequency: str) -> date:
    if frequency == "weekly":
        return d - timedelta(days=d.weekday())
    if frequency == "monthly":
        return d.replace(day=1)
    return d


def next_period_start(start: date, frequency: str) -> date:
    if frequency == "weekly":
        return start + timedelta(weeks=1)
    if frequency == "monthly":
        return add_months(start, 1)
    return start + timedelta(days=1)


def period_ceil(d: date, frequency: str) -> date:
    floor = period_start(d, frequency)
    return d if floor == d else next_period_start(floor, frequency)


def parse_date_range(
    loc: ZoneInfo, frequency: str, start_at: str, end_at: str, now: Clock = utcnow
) -> tuple[date, date]:
    """The [start, end] range snapped outwards to period boundaries, with per-frequency defaults."""
    today = today_in(loc, now)
    end = today
    if end_at:
        try:
            end = parse_day(end_at)
        except ValueError:
            raise ValueError("invalid end_at date") from None
    start = default_start_date(today, frequency)
    if start_at:
        try:
            start = parse_day(start_at)
        except ValueError:
            raise ValueError("invalid start_at date") from None
    return period_start(start, frequency), period_ceil(end, frequency)
