"""Integration fixtures: a migrated test database (TEST_DB_URL) truncated before each test."""

import os
from collections.abc import AsyncIterator

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.db.engine import create_engine
from gv_api.db.migrate import migrate

KEEP = ("schema_migrations", "alembic_version")


def require_db_url() -> str:
    url = os.environ.get("TEST_DB_URL", "")
    if not url:
        pytest.skip("TEST_DB_URL not set")
    return url


@pytest.fixture(scope="session")
async def engine() -> AsyncIterator[AsyncEngine]:
    eng = create_engine(require_db_url(), pool_size=5)
    await migrate(eng)
    yield eng
    await eng.dispose()


async def truncate_all(eng: AsyncEngine) -> None:
    async with eng.begin() as conn:
        tables = (
            await conn.execute(text("SELECT tablename FROM pg_tables WHERE schemaname = 'public'"))
        ).scalars()
        names = [f'"{t}"' for t in tables if t not in KEEP]
        if names:
            await conn.execute(text(f"TRUNCATE {', '.join(names)} RESTART IDENTITY CASCADE"))


@pytest.fixture
async def db(engine: AsyncEngine) -> AsyncEngine:
    await truncate_all(engine)
    return engine
