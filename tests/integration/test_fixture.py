from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine


async def test_fixture_truncates_between_tests(db: AsyncEngine) -> None:
    async with db.begin() as conn:
        await conn.execute(text("CREATE TABLE IF NOT EXISTS fixture_probe (id serial PRIMARY KEY)"))
        await conn.execute(text("INSERT INTO fixture_probe DEFAULT VALUES"))


async def test_fixture_left_an_empty_table(db: AsyncEngine) -> None:
    async with db.begin() as conn:
        count = (await conn.execute(text("SELECT count(*) FROM fixture_probe"))).scalar_one()
        next_id = (
            await conn.execute(text("INSERT INTO fixture_probe DEFAULT VALUES RETURNING id"))
        ).scalar_one()
        await conn.execute(text("DROP TABLE fixture_probe"))
    assert (count, next_id) == (0, 1)
