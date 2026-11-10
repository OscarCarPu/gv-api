"""The plan router over the real service and a recording fake repository."""

import json
from datetime import UTC, date, datetime
from types import SimpleNamespace
from typing import Any, cast
from zoneinfo import ZoneInfo

import httpx
import pytest

from gv_api import plan
from gv_api.auth import AuthService
from gv_api.core.app import create_app
from gv_api.plan import PlanService
from gv_api.plan.errors import NotFoundError, TaskNotFoundError
from gv_api.plan.schemas import PlanBlockResponse, RecurringCommitmentResponse
from gv_api.plan.service import Repository
from gv_api.tasks.schemas import TimeEntrySummaryResponse

from .test_tasks_router import AUTH, CFG, FAIL, FULL, ping
from .test_tasks_service import FakeRepo

NOW = datetime(2026, 5, 10, 8, tzinfo=UTC)
BLOCK = PlanBlockResponse.model_construct(
    id=5,
    plan_date=date(2026, 5, 10),
    started_at=datetime(2026, 5, 10, 12, tzinfo=UTC),
    ended_at=datetime(2026, 5, 10, 13, tzinfo=UTC),
    label="comer",
)
COMMITMENT = RecurringCommitmentResponse.model_construct(
    id=7,
    task_id=42,
    task_name="Work",
    label="Work",
    days_of_week=[1],
    start_time="09:00",
    end_time="13:00",
    active=True,
)
NEW = {"started_at": "2026-05-10T12:00:00Z", "ended_at": "2026-05-10T13:00:00Z", "label": "comer"}


class Summary:
    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse:
        return TimeEntrySummaryResponse(today=1, week=2)


async def call(repo: FakeRepo, method: str, path: str, body: Any = None) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[plan.router])
    svc = PlanService(cast(Repository, repo), Summary(), ZoneInfo("Europe/Madrid"), lambda: NOW)
    app.state.container = SimpleNamespace(services={AuthService: AUTH, PlanService: svc})
    content = body if isinstance(body, str) or body is None else json.dumps(body)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.request(method, path, content=content, headers=FULL)
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


async def test_today() -> None:
    status, body = await call(FakeRepo(list_by_date=[BLOCK]), "GET", "/plan/today")
    assert status == 200
    assert body["date"] == "2026-05-10"
    assert body["blocks"][0] == {
        "id": 5,
        "plan_date": "2026-05-10T00:00:00Z",
        "started_at": "2026-05-10T12:00:00Z",
        "ended_at": "2026-05-10T13:00:00Z",
        "task_id": None,
        "task_name": None,
        "label": "comer",
        "note": None,
        "event_ref": None,
        "commitment_id": None,
        "task_type": None,
        "task_recurrence": None,
        "task_started_at": None,
        "task_finished_at": None,
    }
    assert body["totals"] == {"task_seconds": 0, "free_seconds": 3600}
    assert body["budget"]["week"] == 2
    assert await call(FakeRepo(list_by_date=FAIL), "GET", "/plan/today") == (
        500,
        {"error": "Failed to get today's plan"},
    )


@pytest.mark.parametrize(
    ("query", "message"),
    [
        ("", "invalid or missing from"),
        ("?from=2026-05-04&to=x", "invalid or missing to"),
        ("?from=2026-05-04&to=2026-05-04", "to must be after from"),
    ],
)
async def test_range_rejects(query: str, message: str) -> None:
    assert await call(FakeRepo(), "GET", "/plan/range" + query) == (400, {"error": message})


async def test_range() -> None:
    repo = FakeRepo(list_active_commitments=[], list_by_range=[BLOCK])
    status, body = await call(repo, "GET", "/plan/range?from=2026-05-04&to=2026-05-11")
    assert status == 200
    assert (body["from"], body["to"], len(body["blocks"])) == ("2026-05-04", "2026-05-11", 1)
    repo = FakeRepo(list_active_commitments=FAIL)
    assert (await call(repo, "GET", "/plan/range?from=2026-05-04&to=2026-05-11"))[0] == 500


async def test_create_block() -> None:
    status, body = await call(
        FakeRepo(has_overlap=False, create=BLOCK), "POST", "/plan/blocks", NEW
    )
    assert (status, body["id"]) == (201, 5)


@pytest.mark.parametrize(
    ("repo", "body", "expected"),
    [
        (FakeRepo(), "not json", (400, {"error": "Invalid Body"})),
        (
            FakeRepo(),
            NEW | {"ended_at": "2026-05-10T11:00:00Z"},
            (400, {"error": "ended_at must be after started_at"}),
        ),
        (
            FakeRepo(get_task_name=TaskNotFoundError()),
            NEW | {"label": None, "task_id": 999},
            (400, {"error": "task not found"}),
        ),
        (
            FakeRepo(has_overlap=True),
            NEW,
            (400, {"error": "plan block overlaps with an existing one"}),
        ),
        (FakeRepo(has_overlap=FAIL), NEW, (500, {"error": "Failed to create plan block"})),
    ],
)
async def test_create_block_errors(
    repo: FakeRepo, body: Any, expected: tuple[int, dict[str, str]]
) -> None:
    assert await call(repo, "POST", "/plan/blocks", body) == expected


async def test_update_block() -> None:
    repo = FakeRepo(update=BLOCK)
    assert (await call(repo, "PUT", "/plan/blocks/5", {"label": "updated"}))[0] == 200
    assert repo.calls == [("update", 5, {"label": "updated"})]
    assert await call(FakeRepo(), "PUT", "/plan/blocks/abc", {}) == (
        400,
        {"error": "invalid plan block id"},
    )
    assert await call(FakeRepo(update=NotFoundError()), "PUT", "/plan/blocks/5", {}) == (
        404,
        {"error": "plan block not found"},
    )
    assert await call(FakeRepo(), "PUT", "/plan/blocks/5", {"label": ""}) == (
        400,
        {"error": "label or task_id is required"},
    )
    assert (await call(FakeRepo(update=FAIL), "PUT", "/plan/blocks/5", {}))[0] == 500
    assert (await call(FakeRepo(), "PUT", "/plan/blocks/5", "[]"))[0] == 400


async def test_delete_block() -> None:
    repo = FakeRepo(get=BLOCK)
    assert await call(repo, "DELETE", "/plan/blocks/7") == (204, "")
    assert repo.names() == ["get", "delete"]
    assert await call(FakeRepo(get=NotFoundError()), "DELETE", "/plan/blocks/7") == (
        500,
        {"error": "Failed to delete plan block"},
    )


async def test_delete_future() -> None:
    repo = FakeRepo()
    assert await call(repo, "DELETE", "/plan/blocks/future") == (204, "")
    assert repo.calls == [("delete_ending_after", NOW)]
    assert (await call(FakeRepo(delete_ending_after=FAIL), "DELETE", "/plan/blocks/future"))[
        0
    ] == 500


COMMITMENT_BODY = {
    "task_id": 42,
    "label": "Work",
    "days_of_week": [1],
    "start_time": "09:00",
    "end_time": "13:00",
}


async def test_commitments() -> None:
    status, body = await call(FakeRepo(list_commitments=[COMMITMENT]), "GET", "/plan/commitments")
    assert (status, body[0]["start_time"]) == (200, "09:00")
    assert (await call(FakeRepo(list_commitments=FAIL), "GET", "/plan/commitments"))[0] == 500

    repo = FakeRepo(get_task_name="Work", create_commitment=COMMITMENT)
    assert (await call(repo, "POST", "/plan/commitments", COMMITMENT_BODY))[0] == 201
    for body, message in [
        (COMMITMENT_BODY | {"label": " "}, "label or task_id is required"),
        (COMMITMENT_BODY | {"days_of_week": None}, "days_of_week must not be empty"),
        ("{", "Invalid Body"),
    ]:
        assert await call(FakeRepo(), "POST", "/plan/commitments", body) == (
            400,
            {"error": message},
        )
    missing = FakeRepo(get_task_name=TaskNotFoundError())
    assert await call(missing, "POST", "/plan/commitments", COMMITMENT_BODY) == (
        400,
        {"error": "task not found"},
    )
    bad_time = COMMITMENT_BODY | {"start_time": "25:00"}
    assert await call(FakeRepo(get_task_name="Work"), "POST", "/plan/commitments", bad_time) == (
        500,
        {"error": "Failed to create commitment"},
    )


async def test_update_and_delete_commitment() -> None:
    repo = FakeRepo(update_commitment=COMMITMENT)
    assert (await call(repo, "PUT", "/plan/commitments/7", {"label": "Job"}))[0] == 200
    assert await call(
        FakeRepo(update_commitment=NotFoundError()), "PUT", "/plan/commitments/7", {}
    ) == (404, {"error": "commitment not found"})
    assert await call(FakeRepo(), "PUT", "/plan/commitments/7", {"days_of_week": []}) == (
        400,
        {"error": "days_of_week must not be empty"},
    )
    assert (await call(FakeRepo(update_commitment=FAIL), "PUT", "/plan/commitments/7", {}))[
        0
    ] == 500
    assert (await call(FakeRepo(), "PUT", "/plan/commitments/x", {}))[0] == 400

    repo = FakeRepo()
    assert await call(repo, "DELETE", "/plan/commitments/7") == (204, "")
    assert repo.names() == ["delete_future_commitment_blocks", "delete_commitment"]
    assert (await call(FakeRepo(delete_commitment=FAIL), "DELETE", "/plan/commitments/7"))[0] == 500
    assert (await call(FakeRepo(), "DELETE", "/plan/commitments/0"))[0] == 400
