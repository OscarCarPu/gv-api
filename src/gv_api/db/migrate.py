"""Startup migrations: `alembic upgrade head` in one transaction."""

from pathlib import Path

from alembic import command
from alembic.config import Config as AlembicConfig
from sqlalchemy import text
from sqlalchemy.engine import Connection
from sqlalchemy.ext.asyncio import AsyncEngine

_LOCK = 0x6776617069  # pg advisory lock key, "gvapi"


def _upgrade(conn: Connection, script_location: Path) -> None:
    conn.execute(text("SELECT pg_advisory_xact_lock(:key)"), {"key": _LOCK})
    cfg = AlembicConfig()
    cfg.set_main_option("script_location", str(script_location))
    cfg.attributes["connection"] = conn
    command.upgrade(cfg, "head")


async def migrate(engine: AsyncEngine, script_location: Path | None = None) -> None:
    """Apply pending migrations; a failure rolls all of them back."""
    location = script_location or Path.cwd() / "migrations"
    async with engine.begin() as conn:
        await conn.run_sync(_upgrade, location)
