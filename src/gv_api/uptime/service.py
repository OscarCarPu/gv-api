import asyncio
import logging
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Protocol

from gv_api.core.dates import Clock, utcnow
from gv_api.uptime.errors import InvalidRangeError
from gv_api.uptime.schemas import Overview, Range, RangeUptime, Window, WindowsReport

log = logging.getLogger("gv_api")

BEAT_EVERY = timedelta(seconds=30)
GAP = timedelta(seconds=90)  # a silence longer than this opens a down range

DEFAULT_LOOKBACK = timedelta(days=30)
DEFAULT_WINDOW_LIMIT = 1000
MAX_WINDOW_LIMIT = 5000

# The overview's ranges and how far back each looks (None: since tracking began).
LOOKBACKS: list[tuple[str, timedelta | None]] = [
    ("month", timedelta(days=30)),
    ("3 months", timedelta(days=90)),
    ("year", timedelta(days=365)),
    ("all", None),
]


class Repository(Protocol):
    async def tracking_start(self) -> datetime: ...
    async def ranges(self, start: datetime, end: datetime) -> list[Range]: ...
    async def beat(self, now: datetime, gap: timedelta) -> None: ...


@dataclass
class Totals:
    up: float = 0.0
    down: float = 0.0
    failover: float = 0.0
    outages: int = 0

    def percent(self) -> float | None:
        """Up (failover included) over the covered time, with 2 decimals."""
        covered = self.up + self.down + self.failover
        if covered <= 0:
            return None
        return round(10000 * (self.up + self.failover) / covered) / 100


def clip(r: Range, start: datetime, end: datetime) -> tuple[datetime, datetime]:
    return max(r.start, start), min(r.end, end)


def totals(ranges: list[Range], start: datetime, end: datetime) -> Totals:
    t = Totals()
    for r in ranges:
        lo, hi = clip(r, start, end)
        seconds = (hi - lo).total_seconds()
        if seconds <= 0:
            continue
        if r.kind == "up":
            t.up += seconds
        elif r.kind == "down":
            t.down += seconds
            t.outages += 1
        elif r.kind == "failover":
            t.failover += seconds
    return t


class UptimeService:
    def __init__(self, repo: Repository, now: Clock = utcnow) -> None:
        self._repo = repo
        self._now = now

    async def overview(self) -> Overview:
        since = await self._repo.tracking_start()
        now = self._now()
        ranges = await self._repo.ranges(since, now)
        up_since = since
        for r in ranges:
            if r.kind != "up":
                up_since = r.end
        out = []
        for name, back in LOOKBACKS:
            start = since if back is None else max(since, now - back)
            t = totals(ranges, start, now)
            out.append(
                RangeUptime(
                    range=name,
                    uptime=t.percent(),
                    up_seconds=t.up,
                    down_seconds=t.down,
                    failover_seconds=t.failover,
                    range_start=start,
                    range_end=now,
                )
            )
        return Overview(up_since=up_since, ranges=out)

    async def windows(
        self, start: datetime | None, end: datetime | None, limit: int | None
    ) -> WindowsReport:
        """The ranges over [start, end) newest first, at most limit, and their totals."""
        since = await self._repo.tracking_start()
        now = self._now()
        end = now if end is None or end > now else end
        start = max(start or end - DEFAULT_LOOKBACK, since)
        if start >= end:
            raise InvalidRangeError
        limit = min(limit or DEFAULT_WINDOW_LIMIT, MAX_WINDOW_LIMIT)

        ranges = await self._repo.ranges(start, end)
        t = totals(ranges, start, end)
        report = WindowsReport(
            from_=start,
            to=end,
            uptime=t.percent(),
            up_seconds=t.up,
            down_seconds=t.down,
            failover_seconds=t.failover,
            outages=t.outages,
        )
        for r in reversed(ranges):
            lo, hi = clip(r, start, end)
            if hi <= lo:
                continue
            if len(report.windows) == limit:
                report.truncated = True
                break
            report.windows.append(
                Window(
                    kind=r.kind,
                    start_time=r.start,
                    end_time=r.end,
                    seconds=(hi - lo).total_seconds(),
                )
            )
        return report


Gate = Callable[[], Awaitable[bool]]


class Heartbeat:
    """Records a beat right away and then every `every`, while the gate (if any) allows it."""

    def __init__(
        self,
        repo: Repository,
        every: timedelta = BEAT_EVERY,
        gap: timedelta = GAP,
        gate: Gate | None = None,
        now: Clock = utcnow,
    ) -> None:
        self._repo = repo
        self._every = every
        self._gap = gap
        self._now = now
        self.gate = gate

    async def run(self) -> None:
        while True:
            await self.beat()
            await asyncio.sleep(self._every.total_seconds())

    async def beat(self) -> None:
        if self.gate is not None and not await self.gate():
            return
        try:
            await self._repo.beat(self._now(), self._gap)
        except Exception as err:
            log.error("uptime: heartbeat failed", extra={"error": str(err)})
