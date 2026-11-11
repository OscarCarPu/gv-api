"""CalendarService over a recording fake repository, and its router."""

import json
from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from typing import Any, cast

import httpx
import pytest

from gv_api import calendar
from gv_api.auth import AuthService
from gv_api.calendar import CalendarService
from gv_api.calendar.errors import InvalidError, NotFoundError
from gv_api.calendar.schemas import CreateEventRequest, Event, UpdateEventRequest
from gv_api.calendar.service import Repository
from gv_api.core.app import create_app
from gv_api.core.dates import parse_instant

from .test_tasks_router import AUTH, CFG, FAIL, FULL, ping
from .test_tasks_service import FakeRepo

T0 = datetime(2026, 8, 20, 9, tzinfo=UTC)
HOUR = timedelta(hours=1)


def event(start: datetime = T0, end: datetime = T0 + HOUR, name: str = "x") -> Event:
    return Event.model_construct(id=7, name=name, start_time=start, end_time=end)


class PlanSync:
    def __init__(self, err: Exception | None = None) -> None:
        self.err = err
        self.synced: list[tuple[str, datetime, datetime]] = []
        self.detached: list[str] = []

    async def sync_event_time(self, event_ref: str, start: datetime, end: datetime) -> None:
        self.synced.append((event_ref, start, end))
        if self.err:
            raise self.err

    async def detach_event(self, event_ref: str) -> None:
        self.detached.append(event_ref)
        if self.err:
            raise self.err


def svc(repo: FakeRepo, plan: PlanSync | None = None) -> CalendarService:
    return CalendarService(cast(Repository, repo), plan or PlanSync())


async def test_create_validation() -> None:
    with pytest.raises(InvalidError, match="invalid request: name is required"):
        await svc(FakeRepo()).create_event(
            CreateEventRequest(name=" ", start_time=T0, end_time=T0 + HOUR)
        )
    for start, end in [(T0, T0), (None, T0)]:
        with pytest.raises(InvalidError, match="end_time must be after start_time"):
            await svc(FakeRepo()).create_event(
                CreateEventRequest(name="x", start_time=start, end_time=end)
            )
    repo = FakeRepo(create=event())
    await svc(repo).create_event(CreateEventRequest(name=" x ", start_time=T0, end_time=T0 + HOUR))
    assert repo.calls == [("create", "x", T0, T0 + HOUR)]


async def test_list_rejects_bad_ranges() -> None:
    with pytest.raises(InvalidError, match="to must be after from"):
        await svc(FakeRepo()).list_events(T0, T0)
    with pytest.raises(InvalidError, match="longer than two years"):
        await svc(FakeRepo()).list_events(T0, T0 + timedelta(days=3 * 365))
    repo = FakeRepo(list=[])
    assert await svc(repo).list_events(T0, T0 + HOUR) == []
    assert await svc(FakeRepo(get=event())).get_event(7) == event()


async def test_update_moves_the_linked_plan_block() -> None:
    plan = PlanSync()
    moved = event(end=T0 + 2 * HOUR)
    repo = FakeRepo(get=event(), update=moved)
    assert (
        await svc(repo, plan).update_event(7, UpdateEventRequest(end_time=T0 + 2 * HOUR)) == moved
    )
    assert repo.calls[1] == ("update", 7, {"end_time": T0 + 2 * HOUR})
    assert plan.synced == [("7", T0, T0 + 2 * HOUR)]


async def test_update_name_only_leaves_the_plan_alone() -> None:
    plan = PlanSync()
    repo = FakeRepo(get=event(), update=event(name="renamed"))
    await svc(repo, plan).update_event(7, UpdateEventRequest(name=" renamed "))
    assert repo.calls[1] == ("update", 7, {"name": "renamed"})
    assert plan.synced == []


async def test_update_rejects() -> None:
    with pytest.raises(InvalidError, match="nothing to update"):
        await svc(FakeRepo()).update_event(7, UpdateEventRequest())
    with pytest.raises(InvalidError, match="name cannot be empty"):
        await svc(FakeRepo(get=event())).update_event(7, UpdateEventRequest(name=""))
    repo = FakeRepo(get=event())
    with pytest.raises(InvalidError, match="end_time must be after start_time"):
        await svc(repo).update_event(7, UpdateEventRequest(end_time=T0 - HOUR))
    assert repo.names() == ["get"]
    with pytest.raises(NotFoundError):
        await svc(FakeRepo(get=NotFoundError())).update_event(7, UpdateEventRequest(name="x"))


async def test_delete_detaches_the_plan_block() -> None:
    plan = PlanSync()
    await svc(FakeRepo(), plan).delete_event(7)
    assert plan.detached == ["7"]
    with pytest.raises(NotFoundError):
        await svc(FakeRepo(delete=NotFoundError()), plan).delete_event(8)
    assert plan.detached == ["7"]


async def test_plan_failures_are_logged_not_raised(caplog: pytest.LogCaptureFixture) -> None:
    plan = PlanSync(RuntimeError("plan down"))
    repo = FakeRepo(get=event(), update=event())
    await svc(repo, plan).update_event(7, UpdateEventRequest(start_time=T0 - HOUR))
    await svc(repo, plan).delete_event(7)
    assert [r.getMessage() for r in caplog.records] == [
        "calendar: syncing linked plan block",
        "calendar: detaching plan block",
    ]


def test_parse_instant() -> None:
    assert parse_instant("2026-08-20") == datetime(2026, 8, 20, tzinfo=UTC)
    assert parse_instant("2026-08-20T09:00:00Z") == T0
    assert parse_instant("2026-08-20T11:00:00.5+02:00") == T0 + timedelta(milliseconds=500)
    for raw in ["", "2026-08-20T09:00:00", "yesterday", "2026-8-20"]:
        with pytest.raises(ValueError):
            parse_instant(raw)


# Router


async def call(repo: FakeRepo, method: str, path: str, body: Any = None) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[calendar.router])
    services = {AuthService: AUTH, CalendarService: svc(repo)}
    app.state.container = SimpleNamespace(services=services)
    content = body if isinstance(body, str) or body is None else json.dumps(body)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.request(method, path, content=content, headers=FULL)
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


NEW = {"name": "Dentist", "start_time": "2026-08-20T09:00:00Z", "end_time": "2026-08-20T10:00:00Z"}


async def test_router() -> None:
    repo = FakeRepo(list=[event()])
    status, body = await call(repo, "GET", "/calendar/events?from=2026-08-20&to=2026-08-21")
    assert (status, body) == (
        200,
        [
            {
                "id": 7,
                "name": "x",
                "start_time": "2026-08-20T09:00:00Z",
                "end_time": "2026-08-20T10:00:00Z",
            }
        ],
    )
    assert (await call(FakeRepo(create=event()), "POST", "/calendar/events", NEW))[0] == 201
    assert (await call(FakeRepo(get=event()), "GET", "/calendar/events/7"))[0] == 200
    repo = FakeRepo(get=event(), update=event())
    assert (await call(repo, "PATCH", "/calendar/events/7", {"name": "y"}))[0] == 200
    assert await call(FakeRepo(), "DELETE", "/calendar/events/7") == (204, "")


@pytest.mark.parametrize(
    ("repo", "method", "path", "body", "expected"),
    [
        (
            FakeRepo(),
            "GET",
            "/calendar/events?to=2026-08-21",
            None,
            (400, {"error": "from is required (RFC3339 or YYYY-MM-DD)"}),
        ),
        (
            FakeRepo(),
            "GET",
            "/calendar/events?from=2026-08-20",
            None,
            (400, {"error": "to is required (RFC3339 or YYYY-MM-DD)"}),
        ),
        (
            FakeRepo(),
            "GET",
            "/calendar/events?from=2026-08-21&to=2026-08-20",
            None,
            (400, {"error": "invalid request: to must be after from"}),
        ),
        (
            FakeRepo(list=FAIL),
            "GET",
            "/calendar/events?from=2026-08-20&to=2026-08-21",
            None,
            (500, {"error": "Failed to list events"}),
        ),
        (FakeRepo(), "POST", "/calendar/events", "{", (400, {"error": "invalid body"})),
        (
            FakeRepo(),
            "POST",
            "/calendar/events",
            NEW | {"name": ""},
            (400, {"error": "invalid request: name is required"}),
        ),
        (
            FakeRepo(create=FAIL),
            "POST",
            "/calendar/events",
            NEW,
            (500, {"error": "Failed to create event"}),
        ),
        (
            FakeRepo(get=NotFoundError()),
            "GET",
            "/calendar/events/7",
            None,
            (404, {"error": "not found"}),
        ),
        (FakeRepo(), "GET", "/calendar/events/x", None, (400, {"error": "invalid event id"})),
        (
            FakeRepo(get=NotFoundError()),
            "PATCH",
            "/calendar/events/999",
            {"name": "x"},
            (404, {"error": "not found"}),
        ),
        (FakeRepo(), "PATCH", "/calendar/events/7", "[1]", (400, {"error": "invalid body"})),
        (
            FakeRepo(delete=NotFoundError()),
            "DELETE",
            "/calendar/events/7",
            None,
            (404, {"error": "not found"}),
        ),
        (
            FakeRepo(delete=FAIL),
            "DELETE",
            "/calendar/events/7",
            None,
            (500, {"error": "Failed to delete event"}),
        ),
    ],
)
async def test_router_errors(
    repo: FakeRepo, method: str, path: str, body: Any, expected: tuple[int, Any]
) -> None:
    assert await call(repo, method, path, body) == expected
