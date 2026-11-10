"""The tasks router over the real service and a recording fake repository."""

import json
from datetime import UTC, date, datetime, timedelta
from decimal import Decimal
from types import SimpleNamespace
from typing import Any, cast
from zoneinfo import ZoneInfo

import httpx
import pytest

from gv_api import tasks
from gv_api.auth import AuthService
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.tasks import TasksService
from gv_api.tasks.errors import (
    ActiveTimeEntryExistsError,
    NotFoundError,
    ParentNotFoundError,
    ProjectCycleError,
)
from gv_api.tasks.schemas import (
    ProjectChildrenResponse,
    ProjectDetailResponse,
    ProjectResponse,
    TaskByDueDateResponse,
    TaskResponse,
    TimeEntryResponse,
    TimeEntrySummaryResponse,
    TodoResponse,
)
from gv_api.tasks.service import Repository

from .test_tasks_service import FakeRepo

CFG = Config(
    password="p",
    semiprivate_password="s",
    jwt_secret="secret",
    totp_secret="t",
    allowed_origins=["http://x"],
)
AUTH = AuthService(CFG)
FULL = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "full")}
NOW = datetime(2026, 3, 18, 9, 30, tzinfo=UTC)
FAIL = RuntimeError("db")


async def ping() -> None:
    return None


async def call(repo: FakeRepo, method: str, path: str, body: Any = None) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[tasks.router])
    svc = TasksService(cast(Repository, repo), ZoneInfo("Europe/Madrid"), lambda: NOW)
    app.state.container = SimpleNamespace(services={AuthService: AUTH, TasksService: svc})
    content = body if isinstance(body, str) or body is None else json.dumps(body)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.request(method, path, content=content, headers=FULL)
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


PROJECT = ProjectResponse.model_construct(id=1, name="p", priority=3, due_at=date(2026, 4, 1))
TASK = TaskResponse.model_construct(id=1, name="t", task_type="standard", priority=3)
DEPS = ([], [], False)


def err(message: str) -> dict[str, str]:
    return {"error": message}


# Projects


async def test_create_project() -> None:
    status, body = await call(
        FakeRepo(create_project=PROJECT), "POST", "/tasks/projects", {"name": "p"}
    )
    assert (status, body["due_at"], body["started_at"]) == (201, "2026-04-01T00:00:00Z", None)


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        ("not json", "Invalid Body"),
        ({"name": "x" * 41}, "name must be at most 40 characters"),
        ({"name": "p", "priority": 9}, "priority must be between 1 and 5"),
        ({"name": "p", "due_at": "2026-04-01"}, "Invalid Body"),
    ],
)
async def test_create_project_validation(payload: Any, message: str) -> None:
    assert await call(FakeRepo(), "POST", "/tasks/projects", payload) == (400, err(message))


@pytest.mark.parametrize(
    ("returns", "status", "body"),
    [
        ({"update_project": PROJECT}, 200, None),
        ({"update_project": NotFoundError()}, 404, err("project not found")),
        ({"update_project": ProjectCycleError()}, 409, err(ProjectCycleError.message)),
        ({"update_project": ParentNotFoundError()}, 400, err("parent project not found")),
        ({"update_project": FAIL}, 500, err("Failed to update project")),
    ],
)
async def test_update_project(returns: dict[str, Any], status: int, body: Any) -> None:
    got_status, got = await call(
        FakeRepo(**returns), "PATCH", "/tasks/projects/1", {"parent_id": 2}
    )
    assert got_status == status
    if body is not None:
        assert got == body


async def test_update_project_null_parent_moves_to_root() -> None:
    repo = FakeRepo(update_project=PROJECT)
    await call(repo, "PATCH", "/tasks/projects/1", {"parent_id": None})
    await call(repo, "PATCH", "/tasks/projects/1", {"name": "n"})
    assert [c[2] for c in repo.calls] == [{"parent_id": None}, {"name": "n"}]
    assert await call(repo, "PATCH", "/tasks/projects/x", {}) == (400, err("invalid project id"))
    assert await call(repo, "PATCH", "/tasks/projects/1", {"name": ""}) == (
        200,
        PROJECT.model_dump(mode="json"),
    )


@pytest.mark.parametrize(
    ("path", "method", "failure", "missing"),
    [
        ("/tasks/projects/1", "get_project", "Failed to get project", "project not found"),
        (
            "/tasks/projects/1/children",
            "get_project_children",
            "Failed to get project children",
            "project not found",
        ),
        (
            "/tasks/projects/1/parent-candidates",
            "list_project_parent_candidates",
            "Failed to list parent candidates",
            "project not found",
        ),
        ("/tasks/tasks/1", "get_task", "Failed to get task", "task not found"),
        (
            "/tasks/tasks/1/time-entries",
            "get_task_time_entries",
            "Failed to get task time entries",
            "task not found",
        ),
        (
            "/tasks/time-entries/active",
            "get_active_time_entry",
            "Failed to get active time entry",
            "no active time entry",
        ),
    ],
)
async def test_gets_with_not_found(path: str, method: str, failure: str, missing: str) -> None:
    assert await call(FakeRepo(**{method: NotFoundError()}), "GET", path) == (404, err(missing))
    assert await call(FakeRepo(**{method: FAIL}), "GET", path) == (500, err(failure))


@pytest.mark.parametrize(
    ("path", "method", "failure"),
    [
        ("/tasks/projects", "get_root_projects", "Failed to get projects"),
        ("/tasks/projects/list-fast", "list_projects_fast", "Failed to list projects"),
        ("/tasks/tasks/list-fast", "list_tasks_fast", "Failed to list tasks"),
        ("/tasks/tasks/by-due-date", "get_tasks_by_due_date", "Failed to get tasks by due date"),
        ("/tasks/tasks/by-due-date/slack", "get_tasks_by_due_date", "Failed to get due-soon slack"),
    ],
)
async def test_lists(path: str, method: str, failure: str) -> None:
    assert (await call(FakeRepo(**{method: []}), "GET", path))[0] == 200
    assert await call(FakeRepo(**{method: FAIL}), "GET", path) == (500, err(failure))


async def test_tree_and_summary() -> None:
    tree = FakeRepo(get_active_projects=[], get_unfinished_tasks=[])
    assert await call(tree, "GET", "/tasks/tree") == (200, [])
    assert await call(FakeRepo(get_active_projects=FAIL), "GET", "/tasks/tree") == (
        500,
        err("Failed to get active tree"),
    )
    summary = TimeEntrySummaryResponse(today=60, week=120)
    status, body = await call(
        FakeRepo(get_time_entry_summary=summary), "GET", "/tasks/time-entries/summary"
    )
    assert (status, body["today"], body["weekly_target_seconds"]) == (200, 60, 288000)
    assert await call(
        FakeRepo(get_time_entry_summary=FAIL), "GET", "/tasks/time-entries/summary"
    ) == (
        500,
        err("Failed to get time entry summary"),
    )


async def test_static_subpaths_win_over_ids() -> None:
    repo = FakeRepo(list_projects_fast=[], list_tasks_fast=[], get_tasks_by_due_date=[])
    for path in ("/tasks/projects/list-fast", "/tasks/tasks/list-fast", "/tasks/tasks/by-due-date"):
        assert await call(repo, "GET", path) == (200, [])
    assert await call(repo, "GET", "/tasks/tasks/by-due-date/slack") == (200, {"slack_minutes": 0})


async def test_project_children_shape() -> None:
    children = ProjectChildrenResponse.model_construct(
        project=ProjectDetailResponse.model_construct(id=1, name="p", time_spent=5, priority=3),
        children=[],
    )
    status, body = await call(
        FakeRepo(get_project_children=children), "GET", "/tasks/projects/1/children"
    )
    assert (status, body["children"], body["project"]["time_spent"]) == (200, [], 5)


@pytest.mark.parametrize("raw", ["0", "6", "x", "1.5"])
async def test_min_priority_must_be_1_to_5(raw: str) -> None:
    for path in ("/tasks/tree", "/tasks/tasks/by-due-date"):
        assert await call(FakeRepo(), "GET", f"{path}?min_priority={raw}") == (
            400,
            err("min_priority must be between 1 and 5"),
        )


async def test_min_priority_filters() -> None:
    rows = [
        TaskByDueDateResponse.model_construct(
            id=1, name="a", task_type="standard", priority=1, due_at=date(2026, 3, 20)
        ),
        TaskByDueDateResponse.model_construct(
            id=2, name="b", task_type="standard", priority=4, due_at=date(2026, 3, 20)
        ),
    ]
    repo = FakeRepo(get_tasks_by_due_date=rows, get_active_projects=[], get_unfinished_tasks=[])
    status, body = await call(repo, "GET", "/tasks/tasks/by-due-date?min_priority=2")
    assert (status, [r["id"] for r in body]) == (200, [1])
    assert body[0]["start_by"] is None and body[0]["dependent_hours"] == []
    await call(repo, "GET", "/tasks/tree?min_priority=3")
    assert repo.calls[-1] == ("get_unfinished_tasks", 3)


# Tasks


async def test_create_task() -> None:
    repo = FakeRepo(create_task=TASK, get_task_dependencies=DEPS)
    status, body = await call(repo, "POST", "/tasks/tasks", {"name": "t", "estimate_hours": "2.50"})
    assert (status, body["depends_on"], body["recurrence"]) == (201, [], None)
    assert repo.calls[0][-1] == Decimal("2.50")


@pytest.mark.parametrize(
    ("payload", "message"),
    [
        ("{", "Invalid Body"),
        ({}, "name is required"),
        (
            {"name": "t", "task_type": "weekly"},
            "task_type must be standard, continuous, or recurring",
        ),
        (
            {"name": "t", "task_type": "recurring"},
            "recurrence is required when task_type is recurring",
        ),
        (
            {"name": "t", "task_type": "recurring", "recurrence": 0},
            "recurrence must be a positive number of days",
        ),
        ({"name": "t", "recurrence": 3}, "recurrence is only valid when task_type is recurring"),
        ({"name": "t", "priority": 0}, "priority must be between 1 and 5"),
        ({"name": "t", "estimate_hours": "abc"}, "Invalid Body"),
    ],
)
async def test_create_task_validation(payload: Any, message: str) -> None:
    assert await call(FakeRepo(), "POST", "/tasks/tasks", payload) == (400, err(message))


async def test_create_task_errors_are_500s() -> None:
    assert await call(FakeRepo(create_task=FAIL), "POST", "/tasks/tasks", {"name": "t"}) == (
        500,
        err("Failed to create task"),
    )
    missing_project = FakeRepo(get_project=NotFoundError())
    assert await call(missing_project, "POST", "/tasks/tasks", {"name": "t", "project_id": 9}) == (
        500,
        err("Failed to create task"),
    )


async def test_update_task() -> None:
    repo = FakeRepo(update_task=TASK, get_task_dependencies=DEPS)
    assert (await call(repo, "PATCH", "/tasks/tasks/1", {"due_at": None}))[0] == 200
    await call(repo, "PATCH", "/tasks/tasks/1", {"name": "n"})
    updates = [c[2] for c in repo.calls if c[0] == "update_task"]
    assert updates == [{"due_at": None}, {"name": "n"}]


@pytest.mark.parametrize(
    ("path", "payload", "expected"),
    [
        ("/tasks/tasks/x", {}, (400, err("invalid task id"))),
        ("/tasks/tasks/1", "[", (400, err("Invalid Body"))),
        ("/tasks/tasks/1", {"name": "x" * 41}, (400, err("name must be at most 40 characters"))),
        (
            "/tasks/tasks/1",
            {"task_type": "x"},
            (400, err("task_type must be standard, continuous, or recurring")),
        ),
        (
            "/tasks/tasks/1",
            {"task_type": "recurring"},
            (400, err("recurrence is required when task_type is recurring")),
        ),
        (
            "/tasks/tasks/1",
            {"task_type": "standard", "recurrence": 2},
            (400, err("recurrence is only valid when task_type is recurring")),
        ),
        (
            "/tasks/tasks/1",
            {"recurrence": -1},
            (400, err("recurrence must be a positive number of days")),
        ),
        ("/tasks/tasks/1", {"priority": 6}, (400, err("priority must be between 1 and 5"))),
    ],
)
async def test_update_task_validation(path: str, payload: Any, expected: tuple[int, Any]) -> None:
    assert await call(FakeRepo(), "PATCH", path, payload) == expected


async def test_update_task_errors() -> None:
    assert await call(FakeRepo(update_task=NotFoundError()), "PATCH", "/tasks/tasks/1", {}) == (
        404,
        err("task not found"),
    )
    assert await call(FakeRepo(update_task=FAIL), "PATCH", "/tasks/tasks/1", {}) == (
        500,
        err("Failed to update task"),
    )


# Todos


async def test_todos() -> None:
    todo = TodoResponse(id=1, task_id=2, name="x")
    assert await call(
        FakeRepo(create_todo=todo), "POST", "/tasks/todos", {"task_id": 2, "name": "x"}
    ) == (
        201,
        {"id": 1, "task_id": 2, "name": "x", "is_done": False},
    )
    assert await call(FakeRepo(), "POST", "/tasks/todos", {"name": "x"}) == (
        400,
        err("task_id is required"),
    )
    assert await call(FakeRepo(), "POST", "/tasks/todos", {"task_id": 2}) == (
        400,
        err("name is required"),
    )
    assert await call(FakeRepo(), "POST", "/tasks/todos", "x") == (400, err("Invalid Body"))
    assert await call(
        FakeRepo(create_todo=FAIL), "POST", "/tasks/todos", {"task_id": 2, "name": "x"}
    ) == (
        500,
        err("Failed to create todo"),
    )
    assert (await call(FakeRepo(update_todo=todo), "PATCH", "/tasks/todos/1", {"is_done": True}))[
        0
    ] == 200
    assert await call(FakeRepo(), "PATCH", "/tasks/todos/1", {"name": "x" * 41}) == (
        400,
        err("name must be at most 40 characters"),
    )
    assert await call(FakeRepo(update_todo=NotFoundError()), "PATCH", "/tasks/todos/1", {}) == (
        404,
        err("todo not found"),
    )


# Time entries

ENTRY = TimeEntryResponse(id=1, task_id=2, started_at=NOW)


async def test_create_time_entry() -> None:
    payload = {"task_id": 2, "started_at": "2026-03-18T09:30:00Z"}
    assert await call(
        FakeRepo(create_time_entry=ENTRY), "POST", "/tasks/time-entries", payload
    ) == (
        201,
        {
            "id": 1,
            "task_id": 2,
            "started_at": "2026-03-18T09:30:00Z",
            "finished_at": None,
            "comment": None,
        },
    )
    assert await call(
        FakeRepo(create_time_entry=ActiveTimeEntryExistsError()),
        "POST",
        "/tasks/time-entries",
        payload,
    ) == (
        409,
        err("an active time entry already exists"),
    )
    assert await call(FakeRepo(create_time_entry=FAIL), "POST", "/tasks/time-entries", payload) == (
        500,
        err("Failed to create time entry"),
    )
    assert await call(
        FakeRepo(), "POST", "/tasks/time-entries", {"started_at": payload["started_at"]}
    ) == (
        400,
        err("task_id is required"),
    )
    assert await call(FakeRepo(), "POST", "/tasks/time-entries", {"task_id": 2}) == (
        400,
        err("started_at is required"),
    )
    assert await call(
        FakeRepo(), "POST", "/tasks/time-entries", {"task_id": 2, "started_at": "yesterday"}
    ) == (
        400,
        err("Invalid Body"),
    )


async def test_update_time_entry() -> None:
    repo = FakeRepo(update_time_entry=ENTRY)
    assert (await call(repo, "PATCH", "/tasks/time-entries/1", {"finished_at": None}))[0] == 200
    assert repo.calls == [("update_time_entry", 1, {"finished_at": None})]
    assert await call(FakeRepo(), "PATCH", "/tasks/time-entries/0", {}) == (
        400,
        err("invalid time entry id"),
    )
    assert await call(
        FakeRepo(update_time_entry=NotFoundError()), "PATCH", "/tasks/time-entries/1", {}
    ) == (
        404,
        err("time entry not found"),
    )


@pytest.mark.parametrize(
    ("query", "expected"),
    [
        ("", (400, err("start_time is required"))),
        ("?start_time=2026-3-1", (400, err("invalid start_time format, expected YYYY-MM-DD"))),
        (
            "?start_time=2026-03-01&end_time=x",
            (400, err("invalid end_time format, expected YYYY-MM-DD")),
        ),
        ("?start_time=2026-03-01&end_time=2026-03-02", (200, [])),
        ("?start_time=2026-03-01", (200, [])),
    ],
)
async def test_entries_by_date_range(query: str, expected: tuple[int, Any]) -> None:
    repo = FakeRepo(get_time_entries_by_date_range=[])
    assert await call(repo, "GET", "/tasks/time-entries" + query) == expected


@pytest.mark.parametrize(
    ("query", "expected"),
    [
        ("", (400, err("frequency is required"))),
        ("?frequency=yearly", (400, err("frequency must be daily, weekly, or monthly"))),
        ("?frequency=daily&start_at=bad", (500, err("Failed to get time entry history"))),
    ],
)
async def test_time_entry_history_errors(query: str, expected: tuple[int, Any]) -> None:
    assert await call(FakeRepo(), "GET", "/tasks/time-entries/history" + query) == expected


async def test_time_entry_history() -> None:
    repo = FakeRepo(get_time_entry_history=[])
    status, body = await call(
        repo,
        "GET",
        "/tasks/time-entries/history?frequency=daily&start_at=2026-03-01&end_at=2026-03-02",
    )
    assert (status, body) == (200, {"start_at": "2026-03-01", "end_at": "2026-03-02", "data": []})


# Deletes


@pytest.mark.parametrize(
    ("path", "method", "entity"),
    [
        ("/tasks/projects/1", "delete_project", "project"),
        ("/tasks/tasks/1", "delete_task", "task"),
        ("/tasks/todos/1", "delete_todo", "todo"),
        ("/tasks/time-entries/1", "delete_time_entry", "time entry"),
    ],
)
async def test_deletes(path: str, method: str, entity: str) -> None:
    assert await call(FakeRepo(), "DELETE", path) == (204, "")
    assert await call(FakeRepo(**{method: NotFoundError()}), "DELETE", path) == (
        404,
        err(f"{entity} not found"),
    )
    failure = f"Failed to delete {entity}"
    assert await call(FakeRepo(**{method: FAIL}), "DELETE", path) == (500, err(failure))
    assert await call(FakeRepo(), "DELETE", path.replace("/1", "/abc")) == (
        400,
        err(f"invalid {entity} id"),
    )
