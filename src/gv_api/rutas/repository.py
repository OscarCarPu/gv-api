from datetime import date

from sqlalchemy import delete, select, update
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.rutas.errors import NotFoundError
from gv_api.rutas.schemas import ConcelloMark
from gv_api.rutas.tables import concello_marks as marks

_COLUMNS = (marks.c.id, marks.c.name, marks.c.visited_on, marks.c.description)


class RutasRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def list(self) -> list[ConcelloMark]:
        stmt = select(*_COLUMNS).order_by(marks.c.visited_on.desc(), marks.c.name.asc())
        async with self._engine.connect() as conn:
            rows = await conn.execute(stmt)
        return [ConcelloMark.model_construct(**r._mapping) for r in rows]

    async def get(self, id: int) -> ConcelloMark:
        async with self._engine.connect() as conn:
            row = (await conn.execute(select(*_COLUMNS).where(marks.c.id == id))).one_or_none()
        if row is None:
            raise NotFoundError
        return ConcelloMark.model_construct(**row._mapping)

    async def create(self, name: str, visited_on: date, description: str) -> ConcelloMark:
        stmt = (
            marks.insert()
            .values(name=name, visited_on=visited_on, description=description)
            .returning(*_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return ConcelloMark.model_construct(**row._mapping)

    async def update(self, id: int, visited_on: date, description: str) -> ConcelloMark:
        stmt = (
            update(marks)
            .where(marks.c.id == id)
            .values(visited_on=visited_on, description=description)
            .returning(*_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return ConcelloMark.model_construct(**row._mapping)

    async def delete(self, id: int) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(delete(marks).where(marks.c.id == id))
