"""TasksService over a recording fake repository, the weekly pace and request validation."""

from collections.abc import Awaitable, Callable
from datetime import UTC, date, datetime, time
from decimal import Decimal
from typing import Any, cast
from zoneinfo import ZoneInfo

import pytest

from gv_api.history import Point
from gv_api.tasks import TasksService, budget, validation
from gv_api.tasks.errors import NotFoundError
from gv_api.tasks.schemas import (
    ActiveProject,
    CreateProjectRequest,
    CreateTaskRequest,
    CreateTimeEntryRequest,
    CreateTodoRequest,
    ProjectDetailResponse,
    TaskDepRef,
    TaskResponse,
    TimeEntrySummaryResponse,
    UnfinishedTask,
    UpdateProjectRequest,
    UpdateTaskRequest,
    UpdateTimeEntryRequest,
    UpdateTodoRequest,
)
from gv_api.tasks.service import Repository

MADRID = ZoneInfo("Europe/Madrid")
NOW = datetime(2026, 3, 18, 9, 30, tzinfo=UTC)  # a Wednesday


class FakeRepo:
    """Any repository method: records (name, *args) and returns `returns[name]` (raised when an
    exception)."""

    def __init__(self, **returns: Any) -> None:
        self.returns = returns
        self.calls: list[tuple[Any, ...]] = []

    def __getattr__(self, name: str) -> Callable[..., Awaitable[Any]]:
        async def method(*args: Any) -> Any:
            self.calls.append((name, *args))
            value = self.returns.get(name)
            if isinstance(value, Exception):
                raise value
            return value

        return method

    def names(self) -> list[str]:
        return [c[0] for c in self.calls]


def svc(repo: FakeRepo) -> TasksService:
    return TasksService(cast(Repository, repo), MADRID, lambda: NOW)


def task(id: int = 1, **kw: Any) -> TaskResponse:
    return TaskResponse.model_construct(id=id, name="t", task_type="standard", priority=3, **kw)


DEPS = ([TaskDepRef(id=2, name="b", due_at="2026-03-20")], [], True)


def body(model: Any, **data: Any) -> Any:
    return model.model_validate(data)


# Projects


async def test_create_project_defaults_priority() -> None:
    repo = FakeRepo()
    await svc(repo).create_project(body(CreateProjectRequest, name="p"))
    await svc(repo).create_project(body(CreateProjectRequest, name="q", priority=1, parent_id=4))
    assert [c[1:] for c in repo.calls] == [("p", None, None, None, 3), ("q", None, None, 4, 1)]


async def test_update_project_sends_only_given_fields_and_cascades_finish() -> None:
    repo = FakeRepo()
    req = body(UpdateProjectRequest, name="n", description=None, parent_id=None, finished_at=NOW)
    await svc(repo).update_project(7, req)
    assert repo.calls == [
        ("update_project", 7, {"name": "n", "parent_id": None, "finished_at": NOW}),
        ("finish_subtree", 7),
    ]


async def test_update_project_without_finish_does_not_cascade() -> None:
    repo = FakeRepo()
    await svc(repo).update_project(7, body(UpdateProjectRequest, finished_at=None))
    assert repo.calls == [("update_project", 7, {"finished_at": None})]


async def test_update_project_error_skips_the_cascade() -> None:
    repo = FakeRepo(update_project=NotFoundError())
    with pytest.raises(NotFoundError):
        await svc(repo).update_project(7, body(UpdateProjectRequest, finished_at=NOW))
    assert repo.names() == ["update_project"]


@pytest.mark.parametrize(
    ("method", "args"),
    [
        ("list_project_parent_candidates", (3,)),
        ("list_projects_fast", ()),
        ("get_root_projects", ()),
        ("get_project", (3,)),
        ("get_project_children", (3,)),
        ("delete_project", (3,)),
        ("get_task", (3,)),
        ("get_task_time_entries", (3,)),
        ("list_tasks_fast", ()),
        ("delete_task", (3,)),
        ("delete_todo", (3,)),
        ("delete_time_entry", (3,)),
        ("get_active_time_entry", ()),
    ],
)
async def test_pass_through(method: str, args: tuple[Any, ...]) -> None:
    repo = FakeRepo(**{method: "result"})
    assert await getattr(svc(repo), method)(*args) in ("result", None)
    assert repo.calls == [(method, *args)]


# Active tree


def active(id: int, name: str, parent: int | None = None) -> ActiveProject:
    return ActiveProject(id=id, name=name, parent_id=parent, due_at=date(2026, 4, 1))


def unfinished(id: int, project: int | None, **kw: Any) -> UnfinishedTask:
    return UnfinishedTask(
        id=id, project_id=project, name=f"t{id}", task_type="standard", priority=3, **kw
    )


async def test_active_tree() -> None:
    repo = FakeRepo(
        get_active_projects=[
            active(1, "a root"),
            active(2, "b child", 1),
            active(3, "c child", 1),
            active(4, "grandchild", 2),
            active(5, "under inactive", 99),
        ],
        get_unfinished_tasks=[
            unfinished(10, 1, depends_on=[TaskDepRef(id=11, name="t11")], blocked=True),
            unfinished(11, 4),
            unfinished(12, None),
            unfinished(13, 77),  # its project is not active
        ],
    )
    tree = await svc(repo).get_active_tree(2)
    assert repo.calls[1] == ("get_unfinished_tasks", 2)
    dumped = [n.model_dump(mode="json") for n in tree]
    assert [(n["type"], n["id"]) for n in dumped] == [("project", 1), ("project", 5), ("task", 12)]
    root = tree[0]
    assert [(c.type, c.id) for c in root.children or []] == [
        ("project", 3),
        ("project", 2),
        ("task", 10),
    ]
    child_b = (root.children or [])[1]
    assert [c.id for c in (child_b.children or [])[0].children or []] == [11]
    assert dumped[0]["depends_on"] is None and dumped[0]["due_at"] == "2026-04-01T00:00:00Z"
    task_node = (root.children or [])[2]
    assert (task_node.blocked, task_node.children, task_node.task_type) == (True, None, "standard")
    assert dumped[1]["children"] == []


async def test_empty_tree() -> None:
    assert (
        await svc(FakeRepo(get_active_projects=[], get_unfinished_tasks=[])).get_active_tree(None)
        == []
    )


# Tasks


async def test_create_task_with_dependencies() -> None:
    repo = FakeRepo(create_task=task(5), get_task_dependencies=DEPS)
    req = body(CreateTaskRequest, name="t", depends_on=[2], estimate_hours="1.5", priority=1)
    got = await svc(repo).create_task(req)
    assert repo.calls == [
        ("create_task", None, "t", None, None, "standard", None, 1, Decimal("1.5")),
        ("replace_task_dependencies", 5, [2]),
        ("get_task_dependencies", 5),
    ]
    assert (got.depends_on, got.blocked) == (DEPS[0], True)


async def test_create_task_takes_its_projects_priority() -> None:
    project = ProjectDetailResponse.model_construct(id=4, name="p", time_spent=0, priority=1)
    repo = FakeRepo(get_project=project, create_task=task(5), get_task_dependencies=([], [], False))
    await svc(repo).create_task(
        body(CreateTaskRequest, name="t", project_id=4, task_type="continuous")
    )
    assert repo.calls[0] == ("get_project", 4)
    assert repo.calls[1][5:8] == ("continuous", None, 1)
    assert "replace_task_dependencies" not in repo.names()


async def test_create_task_dependency_error_propagates() -> None:
    repo = FakeRepo(create_task=task(5), replace_task_dependencies=RuntimeError("cycle"))
    with pytest.raises(RuntimeError):
        await svc(repo).create_task(body(CreateTaskRequest, name="t", depends_on=[2]))


async def test_update_task_values_and_dependencies() -> None:
    repo = FakeRepo(update_task=task(1), get_task_dependencies=DEPS)
    req = body(
        UpdateTaskRequest,
        name="n",
        due_at=None,
        estimate_hours=None,
        task_type="standard",
        depends_on=[],
        blocks=[5, 6],
        started_at=NOW,
    )
    await svc(repo).update_task(1, req)
    assert repo.calls[:3] == [
        (
            "update_task",
            1,
            {
                "name": "n",
                "task_type": "standard",
                "due_at": None,
                "started_at": NOW,
                "estimate_hours": None,
                "recurrence": None,
            },
        ),
        ("replace_task_dependencies", 1, []),
        ("replace_task_blocks", 1, [5, 6]),
    ]


async def test_update_task_recurrence_and_absent_lists() -> None:
    repo = FakeRepo(update_task=task(1), get_task_dependencies=([], [], False))
    await svc(repo).update_task(1, body(UpdateTaskRequest, recurrence=3, depends_on=None))
    assert repo.calls[0] == ("update_task", 1, {"recurrence": 3})
    assert repo.names() == ["update_task", "get_task_dependencies"]


# Todos and time entries


async def test_todos() -> None:
    repo = FakeRepo()
    await svc(repo).create_todo(body(CreateTodoRequest, task_id=1, name="x"))
    await svc(repo).update_todo(2, body(UpdateTodoRequest, is_done=False, name=None))
    assert repo.calls == [("create_todo", 1, "x"), ("update_todo", 2, {"is_done": False})]


async def test_time_entries() -> None:
    repo = FakeRepo()
    await svc(repo).create_time_entry(body(CreateTimeEntryRequest, task_id=1, started_at=NOW))
    await svc(repo).update_time_entry(
        2, body(UpdateTimeEntryRequest, finished_at=None, comment="c", started_at=None)
    )
    assert repo.calls == [
        ("create_time_entry", 1, NOW, None, None),
        ("update_time_entry", 2, {"comment": "c", "finished_at": None}),
    ]


async def test_summary_uses_local_day_and_week() -> None:
    repo = FakeRepo(get_time_entry_summary=TimeEntrySummaryResponse(today=1800, week=7200))
    got = await svc(repo).get_time_entry_summary()
    _, today_start, week_start = repo.calls[0]
    assert today_start == datetime.combine(date(2026, 3, 18), time(), MADRID)
    assert week_start == datetime.combine(date(2026, 3, 16), time(), MADRID)
    assert got.weekly_target_seconds == budget.WEEKLY_TASK_TARGET_SECONDS
    assert got.daily_target_seconds == got.pace.weighted_today_share_seconds > 0


async def test_time_entry_history() -> None:
    repo = FakeRepo(get_time_entry_history=[Point(date=date(2026, 3, 1), value=1.5)])
    got = await svc(repo).get_time_entry_history("weekly", "2026-03-04", "2026-03-10")
    assert repo.calls == [
        ("get_time_entry_history", "week", "Europe/Madrid", date(2026, 3, 2), date(2026, 3, 16))
    ]
    assert len(got.data) == 1


@pytest.mark.parametrize(
    ("frequency", "start_at", "end_at"),
    [("yearly", "", ""), ("daily", "bad", ""), ("daily", "", "bad")],
)
async def test_time_entry_history_errors(frequency: str, start_at: str, end_at: str) -> None:
    with pytest.raises(ValueError):
        await svc(FakeRepo()).get_time_entry_history(frequency, start_at, end_at)


async def test_time_entry_history_default_dates() -> None:
    repo = FakeRepo(get_time_entry_history=[])
    got = await svc(repo).get_time_entry_history("daily", "", "")
    assert (got.start_at, got.end_at) == (date(2026, 2, 18), date(2026, 3, 18))


async def test_entries_by_date_range() -> None:
    repo = FakeRepo(get_time_entries_by_date_range=[])
    await svc(repo).get_time_entries_by_date_range(date(2026, 3, 1), date(2026, 3, 2))
    await svc(repo).get_time_entries_by_date_range(date(2026, 3, 1), None)
    midnight = lambda d: datetime.combine(d, time(), MADRID)  # noqa: E731
    assert [c[1:] for c in repo.calls] == [
        (midnight(date(2026, 3, 1)), midnight(date(2026, 3, 3))),
        (midnight(date(2026, 3, 1)), midnight(date(2026, 3, 19))),
    ]


# Pace


def at(day: int, hour: int) -> datetime:
    return datetime(2026, 5, day, hour, tzinfo=UTC)  # May 4, 2026 is a Monday


def test_pace_goal_reached() -> None:
    for week in (budget.WEEKLY_TASK_TARGET_SECONDS, budget.WEEKLY_TASK_TARGET_SECONDS + 50_000):
        pace = budget.calc_pace(at(4, 10), week, 0)
        assert pace.goal_reached
        assert budget.daily_target_seconds(pace) == 0


def test_pace_monday_morning() -> None:
    pace = budget.calc_pace(at(4, 8), 0, 0)
    assert (pace.goal_reached, pace.remaining_full_days) == (False, 6)
    assert pace.weighted_today_share_seconds == pace.weighted_weekday_seconds == 44917
    assert pace.weighted_weekend_seconds == 31706
    assert pace.uniform_per_day_seconds == pace.uniform_today_share_seconds == 41142
    assert budget.daily_target_seconds(pace) == 44917


def test_pace_ignores_the_clock_and_todays_seconds() -> None:
    shares = {budget.calc_pace(at(4, h), 0, 0).weighted_today_share_seconds for h in (8, 14, 22)}
    assert len(shares) == 1
    assert budget.calc_pace(at(4, 14), 20_000, 20_000) == budget.calc_pace(at(4, 14), 0, 0)


def test_pace_weekend() -> None:
    saturday = budget.calc_pace(at(9, 22), 200_000, 0)
    assert (saturday.remaining_full_days, saturday.weighted_today_share_seconds) == (1, 44000)
    assert (saturday.weighted_weekend_seconds, saturday.weighted_weekday_seconds) == (44000, 62333)
    sunday = budget.calc_pace(at(10, 23), 200_000, 0)
    assert (sunday.remaining_full_days, sunday.weighted_today_share_seconds) == (0, 88000)


# Validation


def test_validation_messages() -> None:
    assert validation.validate_name("") == "name is required"
    assert validation.validate_name("x" * 41) == "name must be at most 40 characters"
    assert validation.validate_name("ñ" * 40) == ""
    assert validation.validate_optional_name(None) == ""
    assert validation.validate_task_type("weekly").startswith("task_type must be")
    assert validation.validate_task_type(None) == validation.validate_task_type("recurring") == ""
    assert validation.validate_priority(0) == validation.validate_priority(6) != ""
    assert validation.validate_priority(None) == validation.validate_priority(5) == ""
    assert validation.validate_recurrence_value(0) != ""
    assert validation.validate_recurrence_value(1) == ""
