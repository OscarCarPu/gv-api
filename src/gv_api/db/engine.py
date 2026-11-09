from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine


def async_url(url: str) -> str:
    """DATABASE_URL is a libpq URL (postgresql://...?sslmode=...); asyncpg wants its own form."""
    for prefix in ("postgresql://", "postgres://"):
        if url.startswith(prefix):
            url = "postgresql+asyncpg://" + url[len(prefix) :]
            break
    return url.replace("sslmode=", "ssl=")


def create_engine(url: str, *, pool_size: int = 10) -> AsyncEngine:
    return create_async_engine(async_url(url), pool_size=pool_size, pool_pre_ping=True)


async def ping(engine: AsyncEngine) -> None:
    async with engine.connect() as conn:
        await conn.execute(text("SELECT 1"))
