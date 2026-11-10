from datetime import date
from typing import Protocol

from gv_api.core.dates import parse_day
from gv_api.core.encoding import go_quote
from gv_api.rutas.schemas import ConcelloMark, CreateMarkRequest, UpdateMarkRequest


class Repository(Protocol):
    async def list(self) -> list[ConcelloMark]: ...
    async def get(self, id: int) -> ConcelloMark: ...
    async def create(self, name: str, visited_on: date, description: str) -> ConcelloMark: ...
    async def update(self, id: int, visited_on: date, description: str) -> ConcelloMark: ...
    async def delete(self, id: int) -> None: ...


def parse_date(raw: str) -> date:
    try:
        return parse_day(raw)
    except ValueError:
        raise ValueError(f"invalid date {go_quote(raw)}: must be YYYY-MM-DD") from None


class RutasService:
    def __init__(self, repo: Repository) -> None:
        self._repo = repo

    async def list(self) -> list[ConcelloMark]:
        return await self._repo.list()

    async def get(self, id: int) -> ConcelloMark:
        return await self._repo.get(id)

    async def create(self, req: CreateMarkRequest) -> ConcelloMark:
        return await self._repo.create(req.name, parse_date(req.visited_on), req.description)

    async def update(self, id: int, req: UpdateMarkRequest) -> ConcelloMark:
        return await self._repo.update(id, parse_date(req.visited_on), req.description)

    async def delete(self, id: int) -> None:
        await self._repo.delete(id)
