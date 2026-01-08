import asyncpg
import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy import delete
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine

from app.core import database
from app.core.config import get_settings
from app.core.database import Base
from app.core.security import require_auth
from app.habits.models import Habit, HabitLog
from app.habits.repository import HabitLogRepository, HabitRepository
from app.habits.service import HabitLogService, HabitService
from app.main import app

# Use a module-level flag to track if DB is set up
_db_setup_done = False


@pytest.fixture
async def test_engine():
    """Create test engine per test function."""
    global _db_setup_done
    settings = get_settings()

    # Only create DB once
    if not _db_setup_done:
        conn = await asyncpg.connect(
            user=settings.postgres_user,
            password=settings.postgres_password,
            host=settings.postgres_host,
            port=settings.postgres_port,
            database="postgres",
        )
        await conn.execute(f"DROP DATABASE IF EXISTS {settings.postgres_db}_test;")
        await conn.execute(f"CREATE DATABASE {settings.postgres_db}_test;")
        await conn.close()
        _db_setup_done = True

    engine = create_async_engine(
        f"{settings.database_url}_test",
        echo=True,
        pool_pre_ping=True,
    )
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    database.engine = engine
    yield engine
    await engine.dispose()
    database.engine = None


@pytest.fixture
async def client(test_engine):
    """HTTP client fixture."""
    app.dependency_overrides[require_auth] = lambda: "test_user"
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        yield ac
    app.dependency_overrides.clear()


@pytest.fixture
async def _clean_habits(test_engine):
    """Clean habits and logs tables before and after each test that uses this fixture."""
    async with AsyncSession(test_engine) as session:
        await session.execute(delete(HabitLog))
        await session.execute(delete(Habit))
        await session.commit()
    yield
    async with AsyncSession(test_engine) as session:
        await session.execute(delete(HabitLog))
        await session.execute(delete(Habit))
        await session.commit()


# --- Repository Fixtures ---


@pytest.fixture
async def session(test_engine):
    """Provide an async session for tests."""
    async with AsyncSession(test_engine, expire_on_commit=False) as session:
        yield session


@pytest.fixture
def habit_repository(session: AsyncSession) -> HabitRepository:
    return HabitRepository(session)


@pytest.fixture
def log_repository(session: AsyncSession) -> HabitLogRepository:
    return HabitLogRepository(session)


# --- Service Fixtures ---


@pytest.fixture
def habit_service(
    habit_repository: HabitRepository,
    log_repository: HabitLogRepository,
) -> HabitService:
    return HabitService(habit_repository, log_repository)


@pytest.fixture
def log_service(
    habit_repository: HabitRepository,
    log_repository: HabitLogRepository,
) -> HabitLogService:
    return HabitLogService(habit_repository, log_repository)
