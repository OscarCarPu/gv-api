from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager

from sqlalchemy import text
from sqlalchemy.engine import Engine as SyncEngine
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine, create_async_engine


def async_url(url: str) -> str:
    """DATABASE_URL is a libpq URL (postgresql://...?sslmode=...); asyncpg wants its own form."""
    for prefix in ("postgresql://", "postgres://"):
        if url.startswith(prefix):
            url = "postgresql+asyncpg://" + url[len(prefix) :]
            break
    return url.replace("sslmode=", "ssl=")


class Engine(AsyncEngine):
    """`connect()` is in autocommit: each statement commits on its own, one round trip instead
    of BEGIN, statement and ROLLBACK. Use it for reads and single-statement writes, and
    `begin()` for several statements as one transaction. Both share one pool."""

    def __init__(self, sync_engine: SyncEngine) -> None:
        super().__init__(sync_engine)
        self._reads = AsyncEngine(sync_engine.execution_options(isolation_level="AUTOCOMMIT"))

    def connect(self) -> AsyncConnection:
        return self._reads.connect()

    @asynccontextmanager
    async def begin(self) -> AsyncGenerator[AsyncConnection]:
        async with super().connect() as conn, conn.begin():
            yield conn


def create_engine(url: str, *, pool_size: int = 10) -> AsyncEngine:
    # No pre-ping: it costs a round trip per checkout. A connection Postgres dropped fails one
    # request, and SQLAlchemy then discards the whole pool. LIFO keeps reusing the connection
    # whose prepared statements are warm.
    engine = create_async_engine(
        async_url(url), pool_size=pool_size, pool_pre_ping=False, pool_use_lifo=True
    )
    return Engine(engine.sync_engine)


async def ping(engine: AsyncEngine) -> None:
    async with engine.connect() as conn:
        await conn.execute(text("SELECT 1"))
