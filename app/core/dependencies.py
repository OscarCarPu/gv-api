from typing import Annotated

from fastapi import Depends
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncSession

from app.core.config import Settings
from app.core.database import get_engine

SettingsDep = Annotated[Settings, Depends(Settings)]


async def get_db_connection():
    async with get_engine().connect() as conn:
        yield conn


DbConnectionDep = Annotated[AsyncConnection, Depends(get_db_connection)]


async def get_session():
    async with AsyncSession(get_engine()) as session:
        yield session


SessionDep = Annotated[AsyncSession, Depends(get_session)]
