"""RutasRepository against the test database."""

from datetime import date

import pytest
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.rutas import RutasRepository
from gv_api.rutas.errors import NotFoundError


@pytest.fixture
def repo(db: AsyncEngine) -> RutasRepository:
    return RutasRepository(db)


JUNE_11 = date(2026, 6, 11)


async def test_create_get_round_trip(repo: RutasRepository) -> None:
    created = await repo.create("Arzúa", JUNE_11, "queixo")
    assert created.id
    got = await repo.get(created.id)
    assert got == created
    assert (got.name, got.description, got.visited_on) == ("Arzúa", "queixo", JUNE_11)


async def test_get_not_found(repo: RutasRepository) -> None:
    with pytest.raises(NotFoundError):
        await repo.get(99999)


async def test_list_orders_by_date_desc_then_name(repo: RutasRepository) -> None:
    await repo.create("Sobrado", JUNE_11, "")
    await repo.create("Arzúa", JUNE_11, "")
    await repo.create("Melide", date(2026, 6, 12), "")
    assert [m.name for m in await repo.list()] == ["Melide", "Arzúa", "Sobrado"]


async def test_list_empty(repo: RutasRepository) -> None:
    assert await repo.list() == []


async def test_update_not_found(repo: RutasRepository) -> None:
    with pytest.raises(NotFoundError):
        await repo.update(99999, JUNE_11, "")


async def test_update_keeps_name(repo: RutasRepository) -> None:
    created = await repo.create("Melide", date(2026, 6, 1), "old")
    updated = await repo.update(created.id, JUNE_11, "new")
    assert (updated.name, updated.description, updated.visited_on) == ("Melide", "new", JUNE_11)


async def test_delete(repo: RutasRepository) -> None:
    created = await repo.create("Toques", JUNE_11, "")
    await repo.delete(created.id)
    with pytest.raises(NotFoundError):
        await repo.get(created.id)
    await repo.delete(created.id)  # deleting a missing id is not an error
