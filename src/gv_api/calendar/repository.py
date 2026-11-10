from datetime import datetime
from typing import Any

from sqlalchemy import and_, delete, or_, select, update
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.calendar.errors import NotFoundError
from gv_api.calendar.schemas import Event
from gv_api.calendar.tables import calendar_events as ev

_COLUMNS = (ev.c.id, ev.c.name, ev.c.start_time, ev.c.end_time)


class CalendarRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def list(self, start: datetime, end: datetime) -> list[Event]:
        """Events overlapping [start, end); a zero-length event counts at its instant."""
        query = (
            select(*_COLUMNS)
            .where(
                ev.c.start_time < end,
                or_(
                    ev.c.end_time > start,
                    and_(ev.c.end_time == ev.c.start_time, ev.c.start_time >= start),
                ),
            )
            .order_by(ev.c.start_time, ev.c.id)
        )
        async with self._engine.connect() as conn:
            rows = await conn.execute(query)
        return [Event.model_construct(**r._mapping) for r in rows]

    async def get(self, id: int) -> Event:
        async with self._engine.connect() as conn:
            row = (await conn.execute(select(*_COLUMNS).where(ev.c.id == id))).one_or_none()
        if row is None:
            raise NotFoundError
        return Event.model_construct(**row._mapping)

    async def create(self, name: str, start: datetime, end: datetime) -> Event:
        stmt = ev.insert().values(name=name, start_time=start, end_time=end).returning(*_COLUMNS)
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return Event.model_construct(**row._mapping)

    async def update(self, id: int, values: dict[str, Any]) -> Event:
        stmt = update(ev).where(ev.c.id == id).values(values).returning(*_COLUMNS)
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return Event.model_construct(**row._mapping)

    async def delete(self, id: int) -> None:
        async with self._engine.begin() as conn:
            result = await conn.execute(delete(ev).where(ev.c.id == id))
        if result.rowcount == 0:
            raise NotFoundError
