"""From internal/habits/repository_test.go."""

from datetime import UTC, date, datetime, timedelta

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.habits import HabitsRepository
from gv_api.habits.errors import NotFoundError
from gv_api.habits.schemas import Habit, HabitWithLog


@pytest.fixture
def repo(db: AsyncEngine) -> HabitsRepository:
    return HabitsRepository(db)


def d(iso: str) -> date:
    return date.fromisoformat(iso)


def utc_today() -> date:
    return datetime.now(UTC).date()


async def create(
    repo: HabitsRepository,
    frequency: str = "daily",
    target_min: float | None = None,
    target_max: float | None = None,
    recording_required: bool = True,
    name: str = "h",
) -> Habit:
    return await repo.create_habit(
        name, None, frequency, target_min, target_max, recording_required
    )


async def streaks(db: AsyncEngine, habit_id: int) -> tuple[int, int]:
    async with db.connect() as conn:
        row = (
            await conn.execute(
                text("SELECT current_streak, longest_streak FROM habits WHERE id = :id"),
                {"id": habit_id},
            )
        ).one()
    return row.current_streak, row.longest_streak


async def execute(db: AsyncEngine, sql: str, **params: object) -> None:
    async with db.begin() as conn:
        await conn.execute(text(sql), params)


async def log_all(repo: HabitsRepository, habit_id: int, logs: dict[str, float]) -> None:
    for day, value in logs.items():
        await repo.upsert_log(habit_id, d(day), value)


@pytest.mark.parametrize(
    ("habit", "logs", "expected"),
    [
        pytest.param(
            {"target_min": 1},
            {"2026-03-13": 1, "2026-03-15": 1, "2026-03-16": 1, "2026-03-17": 1},
            (3, 3),
            id="daily chain",
        ),
        pytest.param(
            {"target_min": 1},
            {"2026-03-15": 1.5, "2026-03-16": 1, "2026-03-17": 0.5},
            (2, 2),
            id="current period not met, skips",
        ),
        pytest.param(
            {"target_min": 1},
            {"2026-03-15": 1, "2026-03-17": 0.5},
            (0, 1),
            id="broken streak resets to zero",
        ),
        pytest.param(
            {"frequency": "weekly", "target_min": 3},
            {
                "2026-03-16": 1,
                "2026-03-18": 1,
                "2026-03-09": 1,
                "2026-03-10": 1,
                "2026-03-12": 1,
                "2026-03-14": 1,
                "2026-03-02": 1,
                "2026-03-04": 1,
                "2026-03-06": 1,
            },
            (2, 2),
            id="weekly target",
        ),
        pytest.param(
            {"target_min": 60, "target_max": 80},
            {"2026-03-15": 90, "2026-03-16": 65, "2026-03-17": 70},
            (2, 2),
            id="range target",
        ),
        pytest.param(
            {"target_min": 1, "recording_required": False},
            {"2026-03-14": 1, "2026-03-17": 1},
            (4, 4),
            id="recording not required, carry forward",
        ),
        pytest.param(
            {"target_min": 1, "recording_required": False},
            {"2026-03-14": 1, "2026-03-15": 1, "2026-03-16": 0},
            (0, 2),
            id="recording not required, breaks on zero",
        ),
    ],
)
async def test_recalculate_streak(
    db: AsyncEngine,
    repo: HabitsRepository,
    habit: dict[str, object],
    logs: dict[str, float],
    expected: tuple[int, int],
) -> None:
    h = await create(repo, **habit)  # type: ignore[arg-type]
    await log_all(repo, h.id, logs)
    await repo.recalculate_streak(h.id, d("2026-03-17"))
    assert await streaks(db, h.id) == expected


async def test_longest_streak_is_replaced_not_maxed(
    db: AsyncEngine, repo: HabitsRepository
) -> None:
    h = await create(repo, target_min=1)
    await execute(db, "UPDATE habits SET longest_streak = 99 WHERE id = :id", id=h.id)
    await log_all(repo, h.id, {"2026-03-15": 1, "2026-03-16": 1, "2026-03-17": 1})
    await repo.recalculate_streak(h.id, d("2026-03-17"))
    assert await streaks(db, h.id) == (3, 3)


async def test_zero_max_without_logs_falls_back_to_created_at(
    db: AsyncEngine, repo: HabitsRepository
) -> None:
    h = await create(repo, target_max=0)
    await repo.recalculate_streak(h.id, utc_today())
    assert await streaks(db, h.id) == (1, 1)


def row_of(rows: list[HabitWithLog], habit_id: int) -> HabitWithLog:
    return next(r for r in rows if r.id == habit_id)


async def test_zero_max_no_log_ever_counts_from_creation(repo: HabitsRepository) -> None:
    h = await create(repo, target_max=0, name="no smoking")
    today = utc_today()
    row = row_of(await repo.get_habits_with_logs(today, today), h.id)
    assert (row.current_streak, row.longest_streak) == (1, 1)


async def test_zero_max_positive_log_breaks_the_streak(repo: HabitsRepository) -> None:
    h = await create(repo, target_max=0, name="no smoking")
    await log_all(repo, h.id, {"2026-03-15": 0, "2026-03-16": 2, "2026-03-17": 0})
    row = row_of(await repo.get_habits_with_logs(d("2026-03-17"), d("2026-03-17")), h.id)
    assert (row.current_streak, row.longest_streak) == (1, 1)


async def test_zero_max_future_view_date_is_not_met(repo: HabitsRepository) -> None:
    h = await create(repo, target_max=0, name="no smoking")
    today = utc_today()
    row = row_of(await repo.get_habits_with_logs(today + timedelta(days=1), today), h.id)
    assert row.current_streak == 1


@pytest.mark.parametrize(
    ("target_min", "target_max", "cached"),
    [(1, None, (7, 9)), (0, None, (3, 5))],
    ids=["target_max != 0", "target_min=0 without target_max"],
)
async def test_other_habits_read_the_cached_columns(
    db: AsyncEngine,
    repo: HabitsRepository,
    target_min: float,
    target_max: float | None,
    cached: tuple[int, int],
) -> None:
    h = await create(repo, target_min=target_min, target_max=target_max)
    await execute(
        db,
        "UPDATE habits SET current_streak = :c, longest_streak = :l WHERE id = :id",
        c=cached[0],
        l=cached[1],
        id=h.id,
    )
    today = utc_today()
    row = row_of(await repo.get_habits_with_logs(today, today), h.id)
    assert (row.current_streak, row.longest_streak) == cached


async def test_daily_view_fields(repo: HabitsRepository) -> None:
    weekly = await create(repo, frequency="weekly", name="weekly")
    plain = await create(repo, name="plain")
    await log_all(repo, weekly.id, {"2026-03-16": 1.5, "2026-03-18": 2, "2026-03-23": 9})
    rows = await repo.get_habits_with_logs(d("2026-03-18"), d("2026-03-18"))
    assert [r.id for r in rows] == [weekly.id, plain.id]
    assert (rows[0].log_value, rows[0].period_value) == (2, 3.5)
    assert (rows[1].log_value, rows[1].period_value) == (None, 0)


async def test_upsert_log_replaces_the_value(repo: HabitsRepository) -> None:
    h = await create(repo)
    await repo.upsert_log(h.id, d("2026-03-17"), 1)
    await repo.upsert_log(h.id, d("2026-03-17"), 64.22)
    rows = await repo.get_habits_with_logs(d("2026-03-17"), d("2026-03-17"))
    assert rows[0].log_value == pytest.approx(64.22, rel=1e-6)


async def test_history_fill_zeros(repo: HabitsRepository) -> None:
    h = await create(repo)
    await log_all(repo, h.id, {"2026-03-01": 5, "2026-03-03": 3})
    start, end = d("2026-03-01"), d("2026-03-04")

    filled = await repo.get_habit_history(h.id, "day", start, end, True)
    assert [(p.date, p.value) for p in filled] == [
        (d("2026-03-01"), 5),
        (d("2026-03-02"), 0),
        (d("2026-03-03"), 3),
        (d("2026-03-04"), 0),
    ]
    assert len(await repo.get_habit_history(h.id, "day", start, end, False)) == 2


async def test_history_avg_per_week(repo: HabitsRepository) -> None:
    h = await create(repo)
    await log_all(repo, h.id, {"2026-03-02": 4, "2026-03-04": 6, "2026-03-10": 1})
    points = await repo.get_habit_history_avg(h.id, "week", d("2026-03-02"), d("2026-03-16"), False)
    assert [(p.date, p.value) for p in points] == [(d("2026-03-02"), 5), (d("2026-03-09"), 1)]
    filled = await repo.get_habit_history_avg(h.id, "week", d("2026-03-02"), d("2026-03-16"), True)
    assert [p.value for p in filled] == [5, 1, 0]


async def test_update_round_trip(repo: HabitsRepository) -> None:
    h = await create(repo, target_min=1, name="Original")
    updated = await repo.update_habit(h.id, "Renamed", "Updated desc", "weekly", 2, 5, False)
    assert updated.model_dump() == {
        "id": h.id,
        "name": "Renamed",
        "description": "Updated desc",
        "frequency": "weekly",
        "target_min": 2,
        "target_max": 5,
        "recording_required": False,
        "current_streak": 0,
        "longest_streak": 0,
    }
    assert await repo.get_habit_by_id(h.id) == updated


async def test_update_not_found(repo: HabitsRepository) -> None:
    with pytest.raises(NotFoundError):
        await repo.update_habit(99999, "X", None, "daily", None, None, True)


async def test_get_by_id_not_found(repo: HabitsRepository) -> None:
    with pytest.raises(NotFoundError):
        await repo.get_habit_by_id(99999)


async def test_delete_cascades_to_logs(db: AsyncEngine, repo: HabitsRepository) -> None:
    h = await create(repo)
    await repo.upsert_log(h.id, d("2026-03-17"), 1)
    await repo.delete_habit(h.id)
    await repo.delete_habit(h.id)  # deleting again is not an error
    assert await repo.get_habits_with_logs(d("2026-03-17"), d("2026-03-17")) == []
    async with db.connect() as conn:
        assert (await conn.execute(text("SELECT count(*) FROM habit_logs"))).scalar_one() == 0
