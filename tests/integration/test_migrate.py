"""Startup migrations against throwaway databases (schema-migrations spec)."""

import shutil
import uuid
from collections.abc import AsyncIterator, Awaitable, Callable
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit

import asyncpg
import pytest
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.db.engine import create_engine
from gv_api.db.migrate import migrate

from .conftest import require_db_url

REPO = Path(__file__).resolve().parents[2]
BASELINE_SQL = REPO / "migrations" / "baseline.sql"

NewDB = Callable[[], Awaitable[tuple[AsyncEngine, asyncpg.Connection]]]


def with_db(url: str, name: str) -> str:
    return urlunsplit(urlsplit(url)._replace(path="/" + name))


def plain(url: str) -> str:
    return url.replace("?sslmode=disable", "")


@pytest.fixture
async def new_db() -> AsyncIterator[NewDB]:
    base = require_db_url()
    admin = await asyncpg.connect(plain(with_db(base, "postgres")))
    created: list[tuple[str, AsyncEngine, asyncpg.Connection]] = []

    async def make() -> tuple[AsyncEngine, asyncpg.Connection]:
        name = "gvtest_mig_" + uuid.uuid4().hex[:8]
        await admin.execute(f'CREATE DATABASE "{name}"')
        engine = create_engine(with_db(base, name), pool_size=1)
        raw = await asyncpg.connect(plain(with_db(base, name)))
        created.append((name, engine, raw))
        return engine, raw

    yield make
    for name, engine, raw in created:
        await raw.close()
        await engine.dispose()
        await admin.execute(f'DROP DATABASE "{name}" WITH (FORCE)')
    await admin.close()


async def revision(raw: asyncpg.Connection) -> str | None:
    if not await raw.fetchval("SELECT to_regclass('alembic_version') IS NOT NULL"):
        return None
    value: str | None = await raw.fetchval("SELECT version_num FROM alembic_version")
    return value


async def test_fresh_database(new_db: NewDB) -> None:
    engine, raw = await new_db()
    await migrate(engine)
    assert await revision(raw) == "0001_baseline"
    assert await raw.fetchval("SELECT count(*) FROM outages WHERE kind = 'start'") == 1
    assert await raw.fetchval("SELECT to_regclass('schema_migrations') IS NULL")


async def test_up_to_date_database_runs_nothing(new_db: NewDB) -> None:
    engine, raw = await new_db()
    await migrate(engine)
    await raw.execute("INSERT INTO habits (name, frequency) VALUES ('Read', 'daily')")
    await migrate(engine)
    assert await revision(raw) == "0001_baseline"
    assert await raw.fetchval("SELECT count(*) FROM habits") == 1
    assert await raw.fetchval("SELECT count(*) FROM outages") == 1


async def test_populated_database_without_history_fails_untouched(new_db: NewDB) -> None:
    """A database that already has the schema but was never stamped."""
    engine, raw = await new_db()
    await raw.execute(BASELINE_SQL.read_text().replace("SET LOCAL", "SET"))
    await raw.execute("INSERT INTO habits (name, frequency) VALUES ('Read', 'daily')")
    with pytest.raises(Exception, match="already exists"):
        await migrate(engine)
    assert await revision(raw) is None
    assert await raw.fetchval("SELECT count(*) FROM habits") == 1


async def test_failing_migration_leaves_schema_untouched(new_db: NewDB, tmp_path: Path) -> None:
    shutil.copytree(REPO / "migrations", tmp_path / "migrations")
    (tmp_path / "migrations" / "versions" / "0002_broken.py").write_text(
        "from alembic import op\n"
        'revision = "0002_broken"\ndown_revision = "0001_baseline"\n'
        "branch_labels = None\ndepends_on = None\n"
        'def upgrade() -> None:\n    op.execute("ALTER TABLE nope ADD COLUMN x int")\n'
        "def downgrade() -> None:\n    pass\n"
    )
    engine, raw = await new_db()
    with pytest.raises(Exception, match="nope"):
        await migrate(engine, tmp_path / "migrations")
    assert await raw.fetchval("SELECT to_regclass('habits') IS NULL")
    assert await revision(raw) is None
