import asyncpg
import pytest
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import create_async_engine

from app.core import database
from app.core.config import get_settings
from app.main import app


@pytest.fixture(scope="session", autouse=True)
async def setup_test_db():
    settings = get_settings()

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

    yield


@pytest.fixture(scope="session")
def test_engine():
    """Use a test database."""
    settings = get_settings()
    return create_async_engine(f"{settings.database_url}_test", echo=True)


@pytest.fixture(autouse=True)
def override_engine(test_engine):
    """Inject test engine, bypassing lifespan init."""
    database.engine = test_engine
    yield
    database.engine = None


@pytest.fixture
async def client():
    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as ac:
        yield ac
