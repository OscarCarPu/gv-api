"""CapacityService, its router, and the tasks -> capacity -> plan wiring of urgency."""

from datetime import UTC, date, datetime
from decimal import Decimal
from types import SimpleNamespace
from typing import Any, cast
from zoneinfo import ZoneInfo

import httpx
import pytest

from gv_api import capacity
from gv_api.auth import AuthService
from gv_api.capacity import CapacityService
from gv_api.core.app import create_app
from gv_api.main import build
from gv_api.plan import PlanService
from gv_api.plan import service as plan_service
from gv_api.tasks import TasksService
from gv_api.tasks import service as tasks_service

from .test_tasks_router import AUTH, CFG, FAIL, FULL, ping
from .test_tasks_service import FakeRepo
from .test_tasks_urgency import MADRID, MIDNIGHT, TODAY, row

UTC_ZONE = ZoneInfo("UTC")
NOON = datetime(2026, 9, 20, 12, tzinfo=UTC)


class Busy:
    def __init__(self, busy: dict[date, Decimal] | Exception | None = None) -> None:
        self.busy = busy or {}
        self.calls: list[tuple[date, date]] = []

    async def busy_hours_by_date(self, first: date, end: date) -> dict[date, Decimal]:
        self.calls.append((first, end))
        if isinstance(self.busy, Exception):
            raise self.busy
        return self.busy


def svc(busy: Busy, daily: float = 8, now: datetime = NOON) -> CapacityService:
    return CapacityService(daily, busy, UTC_ZONE, lambda: now)


async def test_one_entry_per_day() -> None:
    busy = Busy({date(2026, 9, 21): Decimal(3)})
    got = await svc(busy).free_busy_range(date(2026, 9, 22), date(2026, 9, 25))
    assert [(d.date.day, d.busy_hours, d.free_hours) for d in got] == [
        (22, Decimal(0), Decimal(8)),
        (23, Decimal(0), Decimal(8)),
        (24, Decimal(0), Decimal(8)),
    ]
    got = await svc(busy).free_busy_range(date(2026, 9, 21), date(2026, 9, 22))
    assert got[0].free_hours == Decimal(5), "busy hours come off capacity"
    assert busy.calls[-1] == (date(2026, 9, 21), date(2026, 9, 22))


async def test_busy_beyond_capacity_floors_free_time_at_zero() -> None:
    busy = Busy({date(2026, 9, 21): Decimal(12)})
    (got,) = await svc(busy).free_busy_range(date(2026, 9, 21), date(2026, 9, 22))
    assert got.free_hours == 0
    assert got.model_dump(mode="json") == {
        "date": "2026-09-21",
        "capacity_hours": "8",
        "busy_hours": "12",
        "free_hours": "0",
    }


@pytest.mark.parametrize("end", [date(2026, 9, 1), date(2026, 9, 20)])
async def test_reversed_or_empty_range_has_no_days(end: date) -> None:
    busy = Busy()
    assert await svc(busy).free_busy_range(date(2026, 9, 20), end) == []
    assert busy.calls == [], "no point asking the plan about a range with no days"


async def test_today_shrinks_with_the_clock() -> None:
    late = datetime(2026, 5, 10, 23, 30, tzinfo=UTC)
    got = await svc(Busy(), 14, late).free_busy_range(date(2026, 5, 10), date(2026, 5, 12))
    assert [d.free_hours for d in got] == [Decimal("0.5"), Decimal(14)]


async def test_today_is_local() -> None:
    late = datetime(2026, 5, 10, 22, 30, tzinfo=UTC)  # 00:30 on the 11th in Madrid
    service = CapacityService(30, Busy(), MADRID, lambda: late)
    got = await service.free_busy_range(date(2026, 5, 10), date(2026, 5, 12))
    assert [d.free_hours for d in got] == [Decimal(30), Decimal("23.5")]


# Router


async def call(busy: Busy, query: str) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[capacity.router])
    services = {AuthService: AUTH, CapacityService: svc(busy)}
    app.state.container = SimpleNamespace(services=services)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.get("/capacity/free-busy" + query, headers=FULL)
    return resp.status_code, resp.json()


@pytest.mark.parametrize(
    ("query", "message"),
    [
        ("", "invalid or missing from"),
        ("?from=2026-09-01", "invalid or missing to"),
        ("?from=2026-09-01&to=2026-09-01", "to must be after from"),
        ("?from=2026-09-01&to=2026-12-01", "range is longer than 90 days"),
    ],
)
async def test_router_rejects(query: str, message: str) -> None:
    assert await call(Busy(), query) == (400, {"error": message})


async def test_router() -> None:
    status, body = await call(Busy(), "?from=2026-09-01&to=2026-11-30")
    assert status == 200
    assert (body["from"], body["to"], len(body["days"])) == ("2026-09-01", "2026-11-30", 90)
    assert await call(Busy(FAIL), "?from=2026-09-01&to=2026-09-02") == (
        500,
        {"error": "Failed to get free/busy range"},
    )


# Urgency wiring


def test_build_wires_both_urgency_providers() -> None:
    cfg = CFG.model_copy(update={"database_url": "postgresql://u:p@127.0.0.1:1/db"})
    services = build(cfg).state.container.services
    tasks = services[TasksService]
    assert tasks._capacity is services[CapacityService]
    assert tasks._plan is services[PlanService]


async def test_urgency_uses_capacity_and_plan() -> None:
    plan_repo = FakeRepo(
        list_active_commitments=[],
        sum_busy_hours_by_date={TODAY: Decimal(10)},
        sum_planned_hours_by_task={1: Decimal(1)},
    )
    plan = PlanService(cast(plan_service.Repository, plan_repo), None, MADRID, lambda: MIDNIGHT)  # type: ignore[arg-type]
    cap = CapacityService(14, plan, MADRID, lambda: MIDNIGHT)
    tasks_repo = FakeRepo(get_tasks_by_due_date=[row(1, 0, "5")])
    tasks = TasksService(cast(tasks_service.Repository, tasks_repo), MADRID, lambda: MIDNIGHT)
    tasks.set_urgency_providers(cap, plan)

    (got,) = await tasks.get_tasks_by_due_date(None)
    assert "sum_busy_hours_by_date" in plan_repo.names()
    assert ("sum_planned_hours_by_task", [1], MIDNIGHT) in plan_repo.calls
    # 5 h estimated, 1 h already planned; today has 14 - 10 = 4 h free.
    assert (got.remaining_hours, got.start_by, got.urgent) == (Decimal(4), TODAY, True)
