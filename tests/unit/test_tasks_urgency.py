"""By-due-date urgency and the due-soon slack, through TasksService with fake providers."""

from dataclasses import dataclass
from datetime import date, datetime, time, timedelta
from decimal import Decimal
from typing import Any, cast
from zoneinfo import ZoneInfo

import pytest

from gv_api.tasks import TasksService
from gv_api.tasks.schemas import TaskByDueDateResponse, TaskDepRef
from gv_api.tasks.service import Repository

MADRID = ZoneInfo("Europe/Madrid")
TODAY = date(2026, 3, 16)
MIDNIGHT = datetime.combine(TODAY, time(), MADRID)


def day(n: int) -> date:
    return TODAY + timedelta(days=n)


def row(id: int, due: int | None, estimate: str | None = None, **kw: Any) -> TaskByDueDateResponse:
    data: dict[str, Any] = {
        "id": id,
        "name": kw.pop("name", f"t{id}"),
        "task_type": "standard",
        "priority": 3,
        "due_at": day(due) if due is not None else None,
        "estimate_hours": Decimal(estimate) if estimate else None,
    }
    return TaskByDueDateResponse.model_construct(**(data | kw))


def refs(*ids: int) -> list[TaskDepRef]:
    return [TaskDepRef(id=i, name=f"t{i}") for i in ids]


@dataclass
class Day:
    date: date
    free_hours: Decimal
    capacity_hours: Decimal = Decimal(0)


class Capacity:
    def __init__(self, days: list[Day]) -> None:
        self.days = days
        self.calls: list[tuple[date, date]] = []

    async def free_busy_range(self, start: date, end: date) -> list[Day]:
        self.calls.append((start, end))
        return self.days


class Plan:
    def __init__(self, planned: dict[int, Decimal] | None = None) -> None:
        self.planned = planned or {}

    async def planned_hours_by_task(
        self, task_ids: list[int], since: datetime
    ) -> dict[int, Decimal]:
        return self.planned


class Rows:
    def __init__(self, rows: list[TaskByDueDateResponse]) -> None:
        self.rows = rows

    async def get_tasks_by_due_date(self) -> list[TaskByDueDateResponse]:
        return [r.model_copy() for r in self.rows]


def days(n: int, free: str, capacity: str = "0") -> list[Day]:
    return [Day(day(i), Decimal(free), Decimal(capacity)) for i in range(n)]


def service(
    rows: list[TaskByDueDateResponse],
    free: list[Day] | None,
    now: datetime = MIDNIGHT,
    plan: Plan | None = None,
) -> TasksService:
    svc = TasksService(cast(Repository, Rows(rows)), MADRID, lambda: now)
    svc.set_urgency_providers(Capacity(free or []), plan or Plan())
    return svc


async def by_id(
    svc: TasksService, min_priority: int | None = None
) -> dict[int, TaskByDueDateResponse]:
    return {r.id: r for r in await svc.get_tasks_by_due_date(min_priority)}


async def test_without_providers_nothing_is_computed() -> None:
    svc = TasksService(cast(Repository, Rows([row(1, 1, "2")])), MADRID, lambda: MIDNIGHT)
    (got,) = await svc.get_tasks_by_due_date(None)
    assert (got.start_by, got.work_order, got.remaining_hours, got.effective_priority) == (
        None,
        None,
        None,
        3,
    )
    assert (await svc.get_due_soon_slack()).slack_minutes == 0


async def test_due_tomorrow_starts_today_whether_or_not_it_fits() -> None:
    got = await by_id(service([row(1, 1, "25"), row(2, 1, "1")], [Day(TODAY, Decimal(14))]))
    assert got[1].urgent and got[2].urgent


async def test_all_estimated_tasks_overdue() -> None:
    capacity = Capacity([])
    svc = TasksService(
        cast(Repository, Rows([row(1, -20, "6"), row(2, -20)])), MADRID, lambda: MIDNIGHT
    )
    svc.set_urgency_providers(capacity, Plan())
    first, second = await svc.get_tasks_by_due_date(None)
    assert capacity.calls == [], "no reversed range is asked for"
    assert (first.urgent, first.start_by, first.remaining_hours) == (True, TODAY, Decimal(6))
    assert (second.urgent, second.start_by) == (False, None)


async def test_dependent_hours() -> None:
    rows = [
        row(1, 30, "2", name="A", blocks=refs(2)),
        row(2, 30, "3", name="B", depends_on=refs(1), blocks=refs(3)),
        row(3, 30, "4", name="C", depends_on=refs(2)),
    ]
    got = await service(rows, days(31, "8", "8")).get_tasks_by_due_date(None)
    assert got[2].dependent_hours == []
    b, c = got[0].dependent_hours
    assert (b.id, b.remaining_hours, b.cumulative_hours) == (2, 3, 5)
    assert (c.id, c.remaining_hours, c.cumulative_hours) == (3, 4, 9)


async def test_competing_tasks_the_higher_priority_starts_today() -> None:
    rows = [row(1, 5, "16", priority=1), row(2, 5, "3")]
    got = await by_id(service(rows, days(5, "4")))
    assert got[1].urgent
    assert not got[2].urgent


async def test_priority_then_deadline() -> None:
    rows = [
        row(1, 4, "8", priority=2),
        row(2, 3, "4", priority=3),
        row(3, 2, "8", priority=2),
    ]
    got = await by_id(service(rows, days(5, "4", "14")))
    assert {i: got[i].start_by for i in got} == {1: day(2), 2: day(3), 3: day(0)}
    assert [got[i].urgent for i in (1, 2, 3)] == [False, False, True]


def chain(free: str) -> TasksService:
    # A -> B -> C -> D -> E (ids 1..5), listed end first.
    rows = [
        row(5, 5, "4", depends_on=refs(4), blocked=True),
        row(4, 5, "4", depends_on=refs(3), blocks=refs(5), blocked=True),
        row(3, 5, "4", depends_on=refs(2), blocks=refs(4), blocked=True),
        row(2, 5, "4", depends_on=refs(1), blocks=refs(3), blocked=True),
        row(1, 5, "4", blocks=refs(2)),
    ]
    return service(rows, days(5, free, "14"))


async def test_chain_work_order() -> None:
    got = await by_id(chain("8"))
    assert got[1].work_order == 1
    assert (got[1].work_order or 0) < (got[2].work_order or 0)


async def test_chain_one_task_per_day() -> None:
    got = await by_id(chain("4"))
    assert (got[1].start_by, got[1].urgent, got[1].finish_by) == (day(0), True, day(1))
    assert (got[2].start_by, got[2].urgent, got[2].finish_by) == (day(1), False, day(2))


async def test_chain_shares_the_day_its_dependent_starts() -> None:
    got = await by_id(chain("8"))
    assert (got[1].start_by, got[2].start_by) == (day(2), day(3))


async def test_future_days_are_capped() -> None:
    free = days(20, "14", "14")
    free[3].free_hours = Decimal(2)
    rows = [row(1, 19, "20"), row(2, 3, "2"), row(3, 11, "17")]
    first, second, third = await service(rows, free).get_tasks_by_due_date(None)
    assert (first.start_by, second.start_by, third.start_by) == (day(16), day(3), day(10))


async def test_chain_inherits_priority() -> None:
    rows = [
        row(1, 2, "4", name="exam", priority=2, depends_on=refs(2), blocked=True),
        row(2, 2, "4", name="study", blocks=refs(1)),
        row(3, 1, "4", name="chore"),
    ]
    svc = service(rows, days(3, "4", "14"))
    got = await by_id(svc)
    study = got[2]
    assert (study.start_by, study.urgent, study.finish_by) == (day(0), True, day(2))
    assert (study.priority, study.effective_priority) == (3, 2)
    assert not got[3].urgent
    assert list(await by_id(svc, 2)) == [1, 2]


async def test_recurring_tasks_take_no_part() -> None:
    rows = [row(1, 3, "1.5", task_type="recurring", recurrence=7), row(2, 3, "8")]
    first, second = await service(rows, days(3, "4")).get_tasks_by_due_date(None)
    assert (first.start_by, first.urgent, first.remaining_hours) == (None, False, None)
    assert second.start_by == day(1)


async def test_time_spent_and_planned_hours_reduce_what_is_left() -> None:
    rows = [row(1, 5, "4", time_spent=3600)]
    (got,) = await service(
        rows, days(5, "8"), plan=Plan({1: Decimal("0.5")})
    ).get_tasks_by_due_date(None)
    assert got.remaining_hours == Decimal("2.5")


async def test_hours_are_rounded_to_16_places() -> None:
    (got,) = await service([row(1, 5, "1", time_spent=100)], days(5, "8")).get_tasks_by_due_date(
        None
    )
    assert str(got.remaining_hours) == "0.9722222222222222"


@pytest.mark.parametrize(
    ("free", "rows", "first_before"),
    [
        (
            "0",
            [
                row(4, 9, "4", priority=2, depends_on=refs(3), blocked=True),
                row(3, 9, "4", priority=2, blocks=refs(4)),
                row(2, 3, "4", priority=2, depends_on=refs(1), blocked=True),
                row(1, 3, "4", priority=2, blocks=refs(2)),
            ],
            [(1, 3), (1, 2)],
        ),
        (
            "0",
            [
                row(3, 9, "4", priority=2, depends_on=refs(2), blocked=True),
                row(2, 9, "4", priority=2, blocks=refs(3)),
                row(1, 3, "5", priority=2),
            ],
            [(1, 2)],
        ),
        ("8", [row(2, 5, "2", priority=2), row(1, 5, "6", priority=2)], [(1, 2)]),
    ],
    ids=["squeezed chains keep deadlines apart", "later chain does not pass", "more hours first"],
)
async def test_work_order_tie_breaks(
    free: str, rows: list[TaskByDueDateResponse], first_before: list[tuple[int, int]]
) -> None:
    got = await by_id(service(rows, days(12, free, "14")))
    for a, b in first_before:
        assert (got[a].work_order or 0) < (got[b].work_order or 0)


async def test_shortage_lands_on_lowest_priority() -> None:
    rows = [row(1, 3, "12", priority=2), row(2, 3, "18")]
    got = await by_id(service(rows, days(4, "6", "14")))
    assert (got[1].urgent, got[1].start_by) == (False, day(2))
    assert got[2].urgent
    assert (got[1].work_order or 0) < (got[2].work_order or 0)


async def test_shortage_keeps_a_dependency_chain_on_its_days() -> None:
    rows = [
        row(1, 3, "4", priority=2, blocks=refs(2)),
        row(2, 3, "6", priority=2, depends_on=refs(1), blocks=refs(3)),
        row(3, 3, "3", priority=2, depends_on=refs(2), blocks=refs(4)),
        row(4, 3, "5", priority=2, depends_on=refs(3)),
        row(5, 3, "15"),
    ]
    got = await by_id(service(rows, days(4, "6", "14")))
    assert (got[5].urgent, got[5].start_by) == (True, day(0))
    assert (got[4].finish_by, got[4].start_by) == (day(3), day(3))
    assert not any(got[i].urgent for i in (1, 2, 3, 4))
    assert got[1].start_by == day(1)


async def test_a_dependency_cycle_is_still_placed() -> None:
    rows = [
        row(1, 3, "2", blocks=refs(2), depends_on=refs(2)),
        row(2, 3, "2", blocks=refs(1), depends_on=refs(1)),
    ]
    got = await by_id(service(rows, days(4, "8")))
    assert sorted(r.work_order or 0 for r in got.values()) == [1, 2]


# Slack


async def slack(
    rows: list[TaskByDueDateResponse], free: list[Day] | None, now: datetime = MIDNIGHT
) -> int:
    return (await service(rows, free, now).get_due_soon_slack()).slack_minutes


@pytest.mark.parametrize(
    ("rows", "free", "minutes"),
    [
        ([row(1, 3, "2")], days(4, "8", "8"), 8 * 60),
        ([row(1, 3, "28")], days(4, "8", "8"), 60),
        ([row(1, 3, "30.5")], days(4, "6", "8"), -6 * 60 - 30),
        ([row(1, -20, "3")], None, -3 * 60),
        ([row(1, 3)], days(4, "8", "8"), 8 * 60),
        ([row(1, 0, "5")], [Day(TODAY, Decimal(8), Decimal(14))], 3 * 60),
    ],
    ids=["free today", "reaches today", "overflows", "overdue", "no estimates", "due today"],
)
async def test_slack(
    rows: list[TaskByDueDateResponse], free: list[Day] | None, minutes: int
) -> None:
    assert await slack(rows, free) == minutes


@pytest.mark.parametrize(("hour", "minutes"), [(0, 11 * 60), (18, 3 * 60), (22, -60)])
async def test_today_shrinks_with_the_clock(hour: int, minutes: int) -> None:
    free = [Day(TODAY, Decimal(14), Decimal(14)), Day(day(1), Decimal(2), Decimal(14))]
    assert await slack([row(1, 1, "5")], free, MIDNIGHT + timedelta(hours=hour)) == minutes
