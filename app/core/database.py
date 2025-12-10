from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine

from app.core.config import get_settings

engine: AsyncEngine | None = None


async def init_engine() -> None:
    global engine
    settings = get_settings()
    engine = create_async_engine(settings.database_url, echo=settings.is_dev)


async def dispose_engine() -> None:
    global engine
    if engine:
        await engine.dispose()
        engine = None


def get_engine() -> AsyncEngine:
    if engine is None:
        raise RuntimeError("Database engine not initialized")
    return engine
