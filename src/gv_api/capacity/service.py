from datetime import date, datetime, time, timedelta
from decimal import Decimal
from typing import Protocol
from zoneinfo import ZoneInfo

from gv_api.capacity.schemas import DayFreeBusy
from gv_api.core.dates import Clock, utcnow


class BusyProvider(Protocol):
    async def busy_hours_by_date(self, first: date, end: date) -> dict[date, Decimal]: ...


class CapacityService:
    def __init__(
        self, daily_hours: float, plan: BusyProvider, loc: ZoneInfo, now: Clock = utcnow
    ) -> None:
        self._daily = Decimal(str(daily_hours))
        self._plan = plan
        self._loc = loc
        self._now = now

    async def free_busy_range(self, start: date, end: date) -> list[DayFreeBusy]:
        """One entry per day in [start, end). An empty or reversed range has no days: a past
        due date is an ordinary input. Today's free hours shrink as the day goes."""
        if end <= start:
            return []
        busy = await self._plan.busy_hours_by_date(start, end)

        now = self._now().astimezone(self._loc)
        today = now.date()
        tomorrow = datetime.combine(today + timedelta(days=1), time(), self._loc)
        left_today = Decimal(str((tomorrow - now).total_seconds() / 3600))

        days = []
        day = start
        while day < end:
            b = busy.get(day, Decimal(0))
            free = self._daily - b
            if day == today:
                free = min(free, left_today)
            days.append(
                DayFreeBusy(
                    date=day,
                    capacity_hours=self._daily,
                    busy_hours=b,
                    free_hours=max(free, Decimal(0)),
                )
            )
            day += timedelta(days=1)
        return days
