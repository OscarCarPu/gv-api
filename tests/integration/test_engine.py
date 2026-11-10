"""The engine: reads in autocommit, begin() as one transaction."""

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

INSERT = text("INSERT INTO concello_marks (name, visited_on) VALUES ('x', DATE '2026-01-01')")
COUNT = text("SELECT count(*) FROM concello_marks")


async def count(db: AsyncEngine) -> int:
    async with db.connect() as conn:
        return int((await conn.execute(COUNT)).scalar_one())


async def test_connect_commits_each_statement(db: AsyncEngine) -> None:
    async with db.connect() as conn:
        await conn.execute(INSERT)
        assert await count(db) == 1, "visible to another connection before the block ends"


async def test_begin_is_one_transaction(db: AsyncEngine) -> None:
    async with db.begin() as conn:
        await conn.execute(INSERT)
        assert await count(db) == 0, "not visible until commit"
    assert await count(db) == 1

    with pytest.raises(RuntimeError):
        async with db.begin() as conn:
            await conn.execute(INSERT)
            raise RuntimeError
    assert await count(db) == 1
