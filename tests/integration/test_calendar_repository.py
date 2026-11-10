"""CalendarRepository, and the calendar service linked to the real plan, against the test
database."""

from datetime import UTC, date, datetime, timedelta
from zoneinfo import ZoneInfo

import pytest
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.calendar import CalendarRepository, CalendarService
from gv_api.calendar.errors import NotFoundError
from gv_api.calendar.schemas import CreateEventRequest, UpdateEventRequest
from gv_api.plan import PlanRepository, PlanService
from gv_api.plan.schemas import NewBlock
from gv_api.tasks.schemas import TimeEntrySummaryResponse

T0 = datetime(2026, 8, 20, 9, tzinfo=UTC)
HOUR = timedelta(hours=1)


class NoSummary:
    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse:
        return TimeEntrySummaryResponse(today=0, week=0)


@pytest.fixture
def repo(db: AsyncEngine) -> CalendarRepository:
    return CalendarRepository(db)


async def test_crud(repo: CalendarRepository) -> None:
    created = await repo.create("Dentist", T0, T0 + HOUR)
    assert (created.name, created.start_time, created.end_time) == ("Dentist", T0, T0 + HOUR)
    assert await repo.get(created.id) == created
    updated = await repo.update(created.id, {"end_time": T0 + 2 * HOUR})
    assert (updated.name, updated.end_time) == ("Dentist", T0 + 2 * HOUR)
    await repo.delete(created.id)
    for call in (repo.get(created.id), repo.update(created.id, {"name": "x"})):
        with pytest.raises(NotFoundError):
            await call
    with pytest.raises(NotFoundError):
        await repo.delete(created.id)


async def test_list_overlapping_the_window(repo: CalendarRepository) -> None:
    a = await repo.create("before", T0 - 2 * HOUR, T0)
    b = await repo.create("across", T0 - HOUR, T0 + HOUR)
    c = await repo.create("instant", T0, T0)
    d = await repo.create("inside", T0 + HOUR, T0 + 2 * HOUR)
    await repo.create("at the end", T0 + 3 * HOUR, T0 + 4 * HOUR)
    window = await repo.list(T0, T0 + 3 * HOUR)
    assert [e.id for e in window] == [b.id, c.id, d.id]
    assert a.id not in [e.id for e in await repo.list(T0, T0 + HOUR)]


async def test_moving_or_deleting_an_event_updates_the_linked_block(db: AsyncEngine) -> None:
    plan_repo = PlanRepository(db)
    calendar = CalendarService(
        CalendarRepository(db), PlanService(plan_repo, NoSummary(), ZoneInfo("Europe/Madrid"))
    )
    ev = await calendar.create_event(
        CreateEventRequest(name="Concert", start_time=T0, end_time=T0 + HOUR)
    )
    block = await plan_repo.create(
        NewBlock(T0.date(), T0, T0 + HOUR, "Concert", event_ref=str(ev.id))
    )

    later = datetime(2026, 8, 21, 23, tzinfo=UTC)
    await calendar.update_event(ev.id, UpdateEventRequest(start_time=later, end_time=later + HOUR))
    moved = await plan_repo.get(block.id)
    assert (moved.started_at, moved.ended_at, moved.plan_date) == (
        later,
        later + HOUR,
        date(2026, 8, 21),
    )

    await calendar.delete_event(ev.id)
    detached = await plan_repo.get(block.id)
    assert detached.event_ref is None
