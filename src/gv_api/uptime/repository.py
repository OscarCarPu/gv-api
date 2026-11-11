from datetime import datetime, timedelta

from sqlalchemy import insert, select, text, update
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine

from gv_api.uptime.schemas import Range
from gv_api.uptime.tables import outages

_RANGES = text("""
SELECT kind::text AS kind, started_at, ends_at FROM (
    SELECT kind, ends_at, lag(ends_at) OVER (ORDER BY ends_at) AS started_at FROM outages
) r
WHERE started_at IS NOT NULL AND ends_at > :start AND started_at < :end
ORDER BY ends_at
""")

_LAST = (
    select(outages.c.id, outages.c.ends_at, outages.c.kind)
    .order_by(outages.c.ends_at.desc())
    .limit(1)
)


class UptimeRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def tracking_start(self) -> datetime:
        async with self._engine.connect() as conn:
            query = select(outages.c.ends_at).where(outages.c.kind == "start")
            start: datetime = (await conn.execute(query)).scalar_one()
        return start

    async def ranges(self, start: datetime, end: datetime) -> list[Range]:
        """The ranges overlapping [start, end), oldest first."""
        async with self._engine.connect() as conn:
            rows = await conn.execute(_RANGES, {"start": start, "end": end})
        return [Range(r.kind, r.started_at, r.ends_at) for r in rows]

    async def last_alive(self) -> datetime | None:
        """The end of the last up range: when the service was last known to answer."""
        query = (
            select(outages.c.ends_at)
            .where(outages.c.kind == "up")
            .order_by(outages.c.ends_at.desc())
            .limit(1)
        )
        async with self._engine.connect() as conn:
            value: datetime | None = (await conn.execute(query)).scalar_one_or_none()
        return value

    async def beat(self, now: datetime, gap: timedelta) -> None:
        """Extend the current up range, or open one; a silence longer than gap was down."""
        async with self._engine.begin() as conn:
            last = (await conn.execute(_LAST.with_for_update())).one()
            if now <= last.ends_at:
                return
            if last.kind == "up" and now - last.ends_at <= gap:
                await _extend(conn, last.id, now)
            elif last.kind == "up":
                await _add(conn, now, "down")
            else:
                await _add(conn, now, "up")

    async def record_failover(
        self, last_alive: datetime | None, ec2_up_at: datetime, home_at: datetime
    ) -> None:
        """After a failover: up until last_alive, down until the backup server answered, then
        failover until home came back."""
        async with self._engine.begin() as conn:
            last = (await conn.execute(_LAST.with_for_update())).one()
            if last.ends_at >= home_at:
                return
            start = last.ends_at
            if last.kind == "up" and last_alive is not None and start < last_alive < home_at:
                start = last_alive
                await _extend(conn, last.id, start)
            if start < ec2_up_at:
                await _add(conn, ec2_up_at, "down")
            await _add(conn, home_at, "failover")


async def _extend(conn: AsyncConnection, id: int, ends_at: datetime) -> None:
    await conn.execute(update(outages).where(outages.c.id == id).values(ends_at=ends_at))


async def _add(conn: AsyncConnection, ends_at: datetime, kind: str) -> None:
    await conn.execute(insert(outages).values(ends_at=ends_at, kind=kind))
