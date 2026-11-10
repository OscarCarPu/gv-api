"""PlanRepository, and the block generation that runs on it, against the test database."""

import asyncio
from datetime import UTC, date, datetime, time, timedelta
from decimal import Decimal
from zoneinfo import ZoneInfo

import pytest
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.plan import PlanRepository, PlanService
from gv_api.plan.errors import NotFoundError, TaskNotFoundError
from gv_api.plan.schemas import (
    CreateCommitmentRequest,
    NewBlock,
    UpdateCommitmentRequest,
    UpdatePlanBlockRequest,
)
from gv_api.tasks import TasksRepository
from gv_api.tasks.schemas import TimeEntrySummaryResponse

MADRID = ZoneInfo("Europe/Madrid")
MONDAY = date(2026, 9, 7)


class NoSummary:
    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse:
        return TimeEntrySummaryResponse(today=0, week=0)


@pytest.fixture
def repo(db: AsyncEngine) -> PlanRepository:
    return PlanRepository(db)


@pytest.fixture
def tasks_repo(db: AsyncEngine) -> TasksRepository:
    return TasksRepository(db)


def service(repo: PlanRepository, now: datetime | None = None) -> PlanService:
    if now is None:
        return PlanService(repo, NoSummary(), MADRID)
    return PlanService(repo, NoSummary(), MADRID, lambda: now)


async def new_task(tasks_repo: TasksRepository, name: str = "Work") -> int:
    task = await tasks_repo.create_task(None, name, None, None, "standard", None, 3, None)
    return task.id


def at(day: date, hour: float) -> datetime:
    return datetime.combine(day, time(), MADRID) + timedelta(hours=hour)


async def add(
    repo: PlanRepository,
    start: datetime,
    end: datetime,
    label: str = "x",
    task_id: int | None = None,
    note: str | None = None,
    event_ref: str | None = None,
) -> int:
    block = await repo.create(NewBlock(start.date(), start, end, label, task_id, note, event_ref))
    return block.id


async def test_create_get_and_joined_task_fields(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo, "Refactor")
    day = date(2026, 9, 1)
    id = await add(repo, at(day, 9), at(day, 10), "Refactor", task_id=task_id, note="n")
    block = await repo.get(id)
    assert (block.task_name, block.task_type, block.note) == ("Refactor", "standard", "n")
    assert block.plan_date == day
    assert [b.id for b in await repo.list_by_date(day)] == [id]
    with pytest.raises(NotFoundError):
        await repo.get(id + 1)


async def test_sum_busy_hours_counts_task_or_event_blocks_only(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo, "Refactor")
    day = date(2026, 9, 1)
    await add(repo, at(day, 9), at(day, 10.5), task_id=task_id)
    await add(repo, at(day, 14), at(day, 15), event_ref="42")
    await add(repo, at(day, 18), at(day, 18.5), "Free time")
    busy = await repo.sum_busy_hours_by_date(day, day, MADRID.key)
    assert busy == {day: Decimal("2.5")}


async def test_sum_busy_hours_splits_multi_day_blocks(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    day = date(2026, 9, 1)
    await add(repo, at(day, 18), at(day + timedelta(days=2), 8), task_id=task_id)
    busy = await repo.sum_busy_hours_by_date(day, day + timedelta(days=3), MADRID.key)
    assert busy == {
        day: Decimal(6),
        day + timedelta(days=1): Decimal(24),
        day + timedelta(days=2): Decimal(8),
    }


async def test_overlap_catches_the_second_day_of_a_multi_day_block(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    day = date(2026, 9, 1)
    id = await add(repo, at(day, 20), at(day + timedelta(days=2), 10), task_id=task_id)
    day2 = day + timedelta(days=1)
    assert await repo.has_overlap(at(day2, 9), at(day2, 11), None)
    assert not await repo.has_overlap(at(day2, 9), at(day2, 11), id)


async def test_planned_hours_skip_past_blocks_and_clip_running_ones(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    other = await new_task(tasks_repo, "Other")
    now = datetime(2026, 9, 10, 12, tzinfo=UTC)
    await add(repo, now - timedelta(days=2), now - timedelta(days=2, hours=-2), task_id=task_id)
    await add(repo, now + timedelta(days=2), now + timedelta(days=2, hours=3), task_id=task_id)
    await add(repo, now - timedelta(hours=1), now + timedelta(hours=2), task_id=other)
    planned = await repo.sum_planned_hours_by_task([task_id, other], now)
    assert planned == {task_id: Decimal(3), other: Decimal(2)}


async def test_event_ref_is_unique(repo: PlanRepository) -> None:
    day = date(2026, 9, 1)
    id = await add(repo, at(day, 9), at(day, 10), event_ref="99")
    with pytest.raises(IntegrityError):
        await add(repo, at(day, 11), at(day, 12), event_ref="99")
    assert (await repo.get_by_event_ref("99")).id == id
    await repo.clear_event_ref(id)
    with pytest.raises(NotFoundError):
        await repo.get_by_event_ref("99")


async def test_update_times_and_delete_ending_after(repo: PlanRepository) -> None:
    day = date(2026, 9, 1)
    a = await add(repo, at(day, 9), at(day, 10))
    b = await add(repo, at(day, 11), at(day, 12))
    moved = at(day + timedelta(days=1), 1)
    await repo.update_times(a, moved, moved + timedelta(hours=1))
    block = await repo.get(a)
    assert (block.plan_date, block.started_at) == (day + timedelta(days=1), moved)
    await repo.delete_ending_after(at(day, 12))
    assert [x.id for x in await repo.list_by_range(at(day, 0), at(day, 48))] == []
    with pytest.raises(NotFoundError):
        await repo.get(b)


async def test_update_missing_block(repo: PlanRepository) -> None:
    with pytest.raises(NotFoundError):
        await repo.update(1, {"label": "x"})
    with pytest.raises(TaskNotFoundError):
        await repo.get_task_name(1)


async def test_generation_respects_weekdays_and_skips(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    svc = service(repo)
    await svc.create_commitment(
        CreateCommitmentRequest(
            task_id=task_id,
            label="Work",
            days_of_week=[1, 2, 3, 4, 5],
            start_time="09:00",
            end_time="17:00",
        )
    )
    week = (MONDAY, MONDAY + timedelta(days=7))
    await svc.ensure_recurring_blocks(*week)
    blocks = await repo.list_by_range(at(week[0], 0), at(week[1], 0))
    assert len(blocks) == 5
    assert blocks[0].started_at == at(MONDAY, 9)

    wednesday = next(b for b in blocks if b.plan_date.weekday() == 2)
    await svc.delete(wednesday.id)
    await svc.ensure_recurring_blocks(*week)
    assert len(await repo.list_by_range(at(week[0], 0), at(week[1], 0))) == 4


async def test_concurrent_generation_never_duplicates(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    svc = service(repo)
    await svc.create_commitment(
        CreateCommitmentRequest(
            task_id=task_id,
            label="Work",
            days_of_week=[1, 2, 3, 4, 5],
            start_time="09:00",
            end_time="17:00",
        )
    )
    week = (MONDAY, MONDAY + timedelta(days=7))
    await asyncio.gather(*(svc.ensure_recurring_blocks(*week) for _ in range(8)))
    assert len(await repo.list_by_range(at(week[0], 0), at(week[1], 0))) == 5


async def test_commitment_edits_move_and_clean_up_generated_blocks(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    svc = service(repo, now=datetime(2026, 9, 6, 12, tzinfo=UTC))
    c = await svc.create_commitment(
        CreateCommitmentRequest(
            task_id=task_id,
            label="Work",
            days_of_week=[1, 2, 3, 4, 5],
            start_time="07:30",
            end_time="11:30",
        )
    )
    assert (c.task_name, c.start_time, c.days_of_week, c.active) == (
        "Work",
        "07:30",
        [1, 2, 3, 4, 5],
        True,
    )
    first, end = MONDAY, MONDAY + timedelta(days=14)  # crosses no DST change

    async def blocks() -> list[tuple[int, str, str]]:
        return [
            (
                b.plan_date.weekday(),
                b.started_at.astimezone(MADRID).strftime("%H:%M"),
                b.ended_at.astimezone(MADRID).strftime("%H:%M"),
            )
            for b in await repo.list_by_range(at(first, 0), at(end, 0))
        ]

    await svc.ensure_recurring_blocks(first, end)
    await svc.update_commitment(c.id, UpdateCommitmentRequest(start_time="15:30", end_time="19:30"))
    assert {(s, e) for _, s, e in await blocks()} == {("15:30", "19:30")}

    await svc.update_commitment(c.id, UpdateCommitmentRequest(days_of_week=[1]))
    assert {wd for wd, _, _ in await blocks()} == {0}

    await svc.update_commitment(c.id, UpdateCommitmentRequest(active=False))
    assert await blocks() == []
    assert await repo.list_active_commitments() == []

    await svc.update_commitment(c.id, UpdateCommitmentRequest(active=True))
    await svc.ensure_recurring_blocks(first, end)
    assert len(await blocks()) == 2

    await svc.delete_commitment(c.id)
    assert await blocks() == []
    assert await repo.list_commitments() == []


async def test_moving_a_generated_block_detaches_it(
    repo: PlanRepository, tasks_repo: TasksRepository
) -> None:
    task_id = await new_task(tasks_repo)
    svc = service(repo)
    c = await svc.create_commitment(
        CreateCommitmentRequest(
            task_id=task_id, label="Gym", days_of_week=[1], start_time="18:00", end_time="19:00"
        )
    )
    week = (MONDAY, MONDAY + timedelta(days=7))
    await svc.ensure_recurring_blocks(*week)
    [block] = await repo.list_by_range(at(week[0], 0), at(week[1], 0))
    assert block.commitment_id == c.id

    tuesday = MONDAY + timedelta(days=1)
    moved = await svc.update(
        block.id,
        UpdatePlanBlockRequest(started_at=at(tuesday, 18), ended_at=at(tuesday, 19)),
    )
    assert moved.commitment_id is None
    await svc.ensure_recurring_blocks(*week)
    assert len(await repo.list_by_range(at(week[0], 0), at(week[1], 0))) == 1
