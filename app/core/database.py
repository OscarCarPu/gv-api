import re
from datetime import datetime
from functools import partial

from sqlalchemy import DateTime
from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine
from sqlalchemy.orm import DeclarativeBase, Mapped, declared_attr, mapped_column

from app.core import TZ
from app.core.config import get_settings


class Base(DeclarativeBase):
    id: Mapped[int] = mapped_column(primary_key=True)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), insert_default=partial(datetime.now, TZ)
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        insert_default=partial(datetime.now, TZ),
        onupdate=partial(datetime.now, TZ),
    )

    @declared_attr.directive
    def __tablename__(cls) -> str:
        # Convert CamelCase to snake_case
        name = re.sub(r"(?<!^)(?=[A-Z])", "_", cls.__name__).lower()
        return name

    def to_dict(self) -> dict:
        """Convert model to dictionary with column values."""
        return {c.name: getattr(self, c.name) for c in self.__table__.columns}


engine: AsyncEngine | None = None


async def init_engine() -> None:
    global engine
    settings = get_settings()
    engine = create_async_engine(
        settings.database_url,
        echo=settings.is_dev,
        pool_size=5,
        max_overflow=10,
        pool_pre_ping=True,
        pool_recycle=3600,
    )


async def create_tables() -> None:
    if engine is None:
        raise RuntimeError("Database engine not initialized")
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)


async def dispose_engine() -> None:
    global engine
    if engine:
        await engine.dispose()
        engine = None


def get_engine() -> AsyncEngine:
    if engine is None:
        raise RuntimeError("Database engine not initialized")
    return engine
