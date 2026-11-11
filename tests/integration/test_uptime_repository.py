"""UptimeRepository against the test database."""

from datetime import UTC, datetime, timedelta

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.uptime import UptimeRepository
from gv_api.uptime.schemas import Range
from gv_api.uptime.service import GAP

ORIGIN = datetime(2026, 8, 1, tzinfo=UTC)


def minutes(n: float) -> datetime:
    return ORIGIN + timedelta(minutes=n)


async def add(db: AsyncEngine, kind: str, at: datetime) -> None:
    async with db.begin() as conn:
        await conn.execute(
            text("INSERT INTO outages (ends_at, kind) VALUES (:at, CAST(:kind AS outage_kind))"),
            {"at": at, "kind": kind},
        )


async def rows(db: AsyncEngine) -> list[tuple[str, datetime]]:
    async with db.connect() as conn:
        result = await conn.execute(
            text("SELECT kind::text, ends_at FROM outages ORDER BY ends_at")
        )
        return [(r[0], r[1]) for r in result]


@pytest.fixture
async def repo(db: AsyncEngine) -> UptimeRepository:
    await add(db, "start", ORIGIN)
    return UptimeRepository(db)


async def test_first_beat_starts_an_up_range(repo: UptimeRepository, db: AsyncEngine) -> None:
    await repo.beat(minutes(1), GAP)
    assert await rows(db) == [("start", ORIGIN), ("up", minutes(1))]


async def test_beats_within_the_gap_extend_the_range(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    for m in (1, 1.5, 2.5):
        await repo.beat(minutes(m), GAP)
    assert await rows(db) == [("start", ORIGIN), ("up", minutes(2.5))]


async def test_a_silence_longer_than_the_gap_is_downtime(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    for m in (1, 10, 10.5):
        await repo.beat(minutes(m), GAP)
    assert await rows(db) == [
        ("start", ORIGIN),
        ("up", minutes(1)),
        ("down", minutes(10)),
        ("up", minutes(10.5)),
    ]


async def test_a_beat_after_a_failover_starts_an_up_range(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    await add(db, "failover", minutes(5))
    await repo.beat(minutes(6), GAP)
    assert await rows(db) == [("start", ORIGIN), ("failover", minutes(5)), ("up", minutes(6))]


async def test_a_clock_behind_the_last_row_writes_nothing(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    await repo.beat(minutes(5), GAP)
    await repo.beat(minutes(4), GAP)
    assert await rows(db) == [("start", ORIGIN), ("up", minutes(5))]


async def test_ranges_start_where_the_previous_one_ends(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    for kind, m in [("up", 10), ("down", 20), ("failover", 30), ("up", 40)]:
        await add(db, kind, minutes(m))
    assert await repo.ranges(minutes(15), minutes(35)) == [
        Range("down", minutes(10), minutes(20)),
        Range("failover", minutes(20), minutes(30)),
        Range("up", minutes(30), minutes(40)),
    ]
    assert await repo.ranges(ORIGIN - timedelta(hours=1), ORIGIN) == []


async def test_tracking_start(repo: UptimeRepository) -> None:
    assert await repo.tracking_start() == ORIGIN


async def test_record_failover(repo: UptimeRepository, db: AsyncEngine) -> None:
    await repo.beat(minutes(1), GAP)
    await repo.record_failover(minutes(1.4), minutes(20), minutes(60))
    expected = [
        ("start", ORIGIN),
        ("up", minutes(1.4)),
        ("down", minutes(20)),
        ("failover", minutes(60)),
    ]
    assert await rows(db) == expected
    await repo.record_failover(None, minutes(20), minutes(60))
    assert await rows(db) == expected, "recording it twice changes nothing"


async def test_record_failover_ignores_a_last_alive_older_than_the_up_range(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    await repo.beat(minutes(5), GAP)
    await repo.record_failover(minutes(2), minutes(20), minutes(60))
    assert (await rows(db))[1] == ("up", minutes(5))


async def test_no_downtime_when_the_backup_was_up_before_the_last_beat(
    repo: UptimeRepository, db: AsyncEngine
) -> None:
    await repo.beat(minutes(30), GAP)
    await repo.record_failover(None, minutes(20), minutes(60))
    assert await rows(db) == [("start", ORIGIN), ("up", minutes(30)), ("failover", minutes(60))]


async def test_last_alive(repo: UptimeRepository) -> None:
    assert await repo.last_alive() is None
    await repo.beat(minutes(3), GAP)
    assert await repo.last_alive() == minutes(3)
