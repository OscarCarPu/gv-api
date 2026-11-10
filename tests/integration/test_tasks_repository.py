"""TasksRepository against the test database."""

from datetime import UTC, date, datetime, timedelta
from decimal import Decimal
from typing import Any

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.tasks import TasksRepository
from gv_api.tasks.errors import (
    ActiveTimeEntryExistsError,
    CircularDependencyError,
    NotFoundError,
    ParentNotFoundError,
    ProjectCycleError,
)
from gv_api.tasks.schemas import ProjectResponse, TaskResponse

NOW = datetime(2026, 3, 17, 10, 0, tzinfo=UTC)


@pytest.fixture
def repo(db: AsyncEngine) -> TasksRepository:
    return TasksRepository(db)


async def project(
    repo: TasksRepository, name: str, parent: int | None = None, **kw: Any
) -> ProjectResponse:
    created = await repo.create_project(name, None, kw.pop("due_at", None), parent, 3)
    if kw:
        return await repo.update_project(created.id, kw)
    return created


async def task(
    repo: TasksRepository,
    name: str,
    project_id: int | None = None,
    priority: int = 3,
    due: date | None = None,
    estimate: str | None = None,
    task_type: str = "standard",
) -> TaskResponse:
    due_at = datetime(due.year, due.month, due.day, tzinfo=UTC) if due else None
    return await repo.create_task(
        project_id,
        name,
        None,
        due_at,
        task_type,
        7 if task_type == "recurring" else None,
        priority,
        Decimal(estimate) if estimate else None,
    )


async def entry(
    repo: TasksRepository, task_id: int, start: datetime, minutes: int | None = None
) -> None:
    end = start + timedelta(minutes=minutes) if minutes is not None else None
    await repo.create_time_entry(task_id, start, end, None)


# Time entries


async def test_one_active_time_entry(repo: TasksRepository) -> None:
    t = await task(repo, "t")
    await entry(repo, t.id, NOW)
    with pytest.raises(ActiveTimeEntryExistsError):
        await entry(repo, t.id, NOW)


async def test_time_entry_for_an_unknown_task_is_another_error(repo: TasksRepository) -> None:
    with pytest.raises(Exception) as err:
        await entry(repo, 99999, NOW, 10)
    assert not isinstance(err.value, ActiveTimeEntryExistsError)


async def test_update_time_entry_reopens_and_not_found(repo: TasksRepository) -> None:
    t = await task(repo, "t")
    created = await repo.create_time_entry(t.id, NOW, NOW + timedelta(hours=1), "c")
    reopened = await repo.update_time_entry(created.id, {"finished_at": None, "comment": "x"})
    assert (reopened.finished_at, reopened.comment) == (None, "x")
    assert (await repo.update_time_entry(created.id, {})).id == created.id
    with pytest.raises(NotFoundError):
        await repo.update_time_entry(99999, {})


async def test_task_time_entries(repo: TasksRepository) -> None:
    t = await task(repo, "t")
    empty = await repo.get_task_time_entries(t.id)
    assert (empty.task.id, empty.time_entries, empty.task.time_spent) == (t.id, [], 0)
    await entry(repo, t.id, NOW + timedelta(hours=2), 30)
    await entry(repo, t.id, NOW, 60)
    got = await repo.get_task_time_entries(t.id)
    assert [e.started_at for e in got.time_entries] == [NOW, NOW + timedelta(hours=2)]
    assert got.task.time_spent == 5400
    with pytest.raises(NotFoundError):
        await repo.get_task_time_entries(99999)


async def test_active_entry_summary_and_range(repo: TasksRepository) -> None:
    p = await project(repo, "p")
    t = await task(repo, "t", p.id)
    with pytest.raises(NotFoundError):
        await repo.get_active_time_entry()
    await entry(repo, t.id, NOW - timedelta(days=2), 60)
    await entry(repo, t.id, NOW - timedelta(minutes=30), 20)
    await entry(repo, t.id, NOW)
    active = await repo.get_active_time_entry()
    assert (active.task_name, active.project_name, active.finished_at) == ("t", "p", None)

    summary = await repo.get_time_entry_summary(
        NOW.replace(hour=0), NOW.replace(hour=0) - timedelta(days=1)
    )
    assert (summary.today, summary.week) == (1200, 1200)

    in_range = await repo.get_time_entries_by_date_range(NOW - timedelta(hours=1), NOW)
    assert [(e.project_name, e.finished_at is None) for e in in_range] == [
        ("p", True),
        ("p", False),
    ]


async def test_time_entry_history_splits_at_period_bounds(repo: TasksRepository) -> None:
    t = await task(repo, "t")
    await entry(repo, t.id, datetime(2026, 3, 1, 23, 0, tzinfo=UTC), 120)
    points = await repo.get_time_entry_history("day", "UTC", date(2026, 3, 1), date(2026, 3, 3))
    assert [(p.date.day, p.value) for p in points] == [(1, 1), (2, 1), (3, 0)]
    madrid = await repo.get_time_entry_history(
        "day", "Europe/Madrid", date(2026, 3, 1), date(2026, 3, 2)
    )
    assert [p.value for p in madrid] == [0, 2]


# Projects


async def test_finishing_a_project_finishes_its_subtree(repo: TasksRepository) -> None:
    root = await project(repo, "root")
    mid = await project(repo, "mid", root.id)
    leaf = await project(repo, "leaf", mid.id)
    t = await task(repo, "t", leaf.id)
    await repo.finish_subtree(root.id)
    assert (await repo.get_task(t.id)).finished_at is not None
    assert (await repo.get_project(leaf.id)).finished_at is not None
    assert (await repo.get_project(root.id)).finished_at is None


async def test_move_project(repo: TasksRepository) -> None:
    a = await project(repo, "a")
    b = await project(repo, "b")
    child = await project(repo, "child", a.id)
    grandchild = await project(repo, "grandchild", child.id)

    assert (await repo.update_project(child.id, {"parent_id": b.id})).parent_id == b.id
    assert (await repo.update_project(child.id, {"name": "c2"})).parent_id == b.id
    assert (await repo.update_project(child.id, {"parent_id": None})).parent_id is None
    for parent in (child.id, grandchild.id):
        with pytest.raises(ProjectCycleError):
            await repo.update_project(child.id, {"parent_id": parent})
    assert (await repo.update_project(grandchild.id, {"parent_id": a.id})).parent_id == a.id
    with pytest.raises(ParentNotFoundError):
        await repo.update_project(child.id, {"parent_id": 99999})
    with pytest.raises(NotFoundError):
        await repo.update_project(99999, {"parent_id": a.id})
    with pytest.raises(NotFoundError):
        await repo.update_project(99999, {})


async def test_parent_candidates(repo: TasksRepository) -> None:
    root = await project(repo, "root")
    me = await project(repo, "me", root.id)
    await project(repo, "mine", me.id)
    other = await project(repo, "other", root.id)
    await project(repo, "deep", other.id)
    await project(repo, "done", finished_at=NOW)
    paths = [c.path for c in await repo.list_project_parent_candidates(me.id)]
    assert paths == ["root", "root / other", "root / other / deep"]

    await repo.update_project(root.id, {"finished_at": NOW})
    assert [c.name for c in await repo.list_project_parent_candidates(me.id)] == [
        "root",
        "other",
        "deep",
    ]
    with pytest.raises(NotFoundError):
        await repo.list_project_parent_candidates(99999)


async def test_projects_fast_root_and_active(repo: TasksRepository) -> None:
    started = await project(repo, "b", started_at=NOW, priority=1)
    await project(repo, "a")
    await project(repo, "c", started.id, started_at=NOW)
    await project(repo, "z", started_at=NOW, finished_at=NOW)
    assert [(p.name, p.priority) for p in await repo.list_projects_fast()] == [
        ("b", 1),
        ("c", 3),
    ]
    assert [p.name for p in await repo.get_root_projects()] == ["a", "b"]
    assert [p.name for p in await repo.get_active_projects()] == ["b", "c"]


async def test_project_children(repo: TasksRepository) -> None:
    root = await project(repo, "root")
    sub = await project(repo, "sub", root.id, started_at=NOW)
    first = await task(repo, "b first", root.id)
    second = await task(repo, "a second", root.id)
    await repo.replace_task_dependencies(second.id, [first.id])
    sub_task = await task(repo, "deep", sub.id)
    await entry(repo, sub_task.id, NOW, 60)
    await entry(repo, first.id, NOW + timedelta(hours=2), 30)
    await repo.create_todo(first.id, "done later")
    todo = await repo.create_todo(first.id, "done")
    await repo.update_todo(todo.id, {"is_done": True})

    got = await repo.get_project_children(root.id)
    assert (got.project.id, got.project.time_spent) == (root.id, 5400)
    assert [(c.type, c.name) for c in got.children] == [
        ("project", "sub"),
        ("task", "b first"),
        ("task", "a second"),
    ]
    sub_node, first_node, second_node = got.children
    assert (sub_node.time_spent, sub_node.parent_id, sub_node.todos) == (3600, root.id, None)
    assert [t.name for t in first_node.todos or []] == ["done later", "done"]
    assert [d.name for d in second_node.depends_on or []] == ["b first"]
    assert (second_node.blocked, first_node.blocks[0].due_at) == (True, None)  # type: ignore[index]
    assert (await repo.get_project(root.id)).time_spent == 5400
    with pytest.raises(NotFoundError):
        await repo.get_project_children(99999)
    with pytest.raises(NotFoundError):
        await repo.get_project(99999)


async def test_delete_project(repo: TasksRepository) -> None:
    p = await project(repo, "p")
    await repo.delete_project(p.id)
    with pytest.raises(NotFoundError):
        await repo.delete_project(p.id)


# Tasks


async def test_task_round_trip(repo: TasksRepository) -> None:
    p = await project(repo, "p")
    t = await task(repo, "t", p.id, due=date(2026, 3, 20), estimate="2.50")
    assert (t.due_at, t.estimate_hours) == (date(2026, 3, 20), Decimal("2.50"))
    got = await repo.get_task(t.id)
    assert (got.project_name, got.todos, got.depends_on, got.time_spent) == ("p", [], [], 0)
    updated = await repo.update_task(
        t.id, {"due_at": None, "estimate_hours": None, "name": "renamed"}
    )
    assert (updated.due_at, updated.estimate_hours, updated.name) == (None, None, "renamed")
    with pytest.raises(NotFoundError):
        await repo.update_task(99999, {})
    with pytest.raises(NotFoundError):
        await repo.get_task(99999)
    await repo.delete_task(t.id)
    with pytest.raises(NotFoundError):
        await repo.delete_task(t.id)


async def test_dependencies_replace_and_cycles(repo: TasksRepository) -> None:
    a, b, c = [await task(repo, n) for n in "abc"]
    await repo.replace_task_dependencies(c.id, [a.id, b.id])
    depends_on, blocks, blocked = await repo.get_task_dependencies(c.id)
    assert ([d.name for d in depends_on], blocks, blocked) == (["a", "b"], [], True)
    await repo.replace_task_dependencies(c.id, [b.id])
    assert [d.name for d in (await repo.get_task_dependencies(c.id))[0]] == ["b"]
    with pytest.raises(CircularDependencyError):
        await repo.replace_task_dependencies(b.id, [c.id])
    await repo.replace_task_blocks(a.id, [b.id])
    assert [d.name for d in (await repo.get_task_dependencies(a.id))[1]] == ["b"]
    with pytest.raises(CircularDependencyError):
        await repo.replace_task_blocks(c.id, [a.id])
    await repo.replace_task_dependencies(c.id, [])
    await repo.replace_task_blocks(a.id, [])
    assert await repo.get_task_dependencies(c.id) == ([], [], False)


async def test_unfinished_tasks_order_filter_and_effective_due(repo: TasksRepository) -> None:
    late = await task(repo, "late", due=date(2026, 4, 1), priority=1)
    early = await task(repo, "early blocked", due=date(2026, 3, 20), priority=4)
    await repo.replace_task_dependencies(early.id, [late.id])
    started = await task(repo, "started")
    await repo.update_task(started.id, {"started_at": NOW})
    await task(repo, "recurring", task_type="recurring")
    await task(repo, "continuous", task_type="continuous")
    rows = await repo.get_unfinished_tasks(None)
    assert [r.name for r in rows] == ["started", "continuous", "recurring", "early blocked", "late"]
    by_name = {r.name: r for r in rows}
    assert by_name["late"].due_at == date(2026, 3, 20), "inherits its dependent's earlier due"
    assert [r.name for r in await repo.get_unfinished_tasks(1)] == ["late"]


async def test_tasks_by_due_date(repo: TasksRepository) -> None:
    p = await project(repo, "p", due_at=datetime(2026, 5, 1, tzinfo=UTC))
    a = await task(repo, "a", due=date(2026, 4, 1), estimate="3", priority=5)
    b = await task(repo, "b", due=date(2026, 3, 20))
    c = await task(repo, "c", due=date(2026, 3, 25))
    await repo.replace_task_dependencies(b.id, [a.id])
    await repo.replace_task_dependencies(c.id, [b.id])
    await task(repo, "from project", p.id)
    await task(repo, "undated")
    await entry(repo, a.id, NOW, 90)
    rows = await repo.get_tasks_by_due_date()
    assert [r.name for r in rows] == ["a", "b", "c", "from project"]
    first = rows[0]
    assert (first.due_at, first.estimate_hours, first.time_spent) == (
        date(2026, 3, 20),
        Decimal("3.00"),
        5400,
    )
    assert rows[-1].project_due_at == date(2026, 5, 1)
    assert [r.priority for r in rows] == [5, 3, 3, 3]


async def test_tasks_fast(repo: TasksRepository) -> None:
    active = await project(repo, "active", started_at=NOW)
    idle = await project(repo, "idle")
    await task(repo, "in active", active.id)
    await task(repo, "in idle", idle.id)
    await task(repo, "loose")
    assert [(t.name, t.project_name) for t in await repo.list_tasks_fast()] == [
        ("in active", "active"),
        ("loose", None),
    ]


# Todos


async def test_todos(repo: TasksRepository) -> None:
    t = await task(repo, "t")
    todo = await repo.create_todo(t.id, "x")
    assert (todo.task_id, todo.is_done) == (t.id, False)
    assert (await repo.update_todo(todo.id, {"name": "y"})).name == "y"
    with pytest.raises(NotFoundError):
        await repo.update_todo(99999, {})
    await repo.delete_todo(todo.id)
    with pytest.raises(NotFoundError):
        await repo.delete_todo(todo.id)


async def test_deleting_a_task_with_entries_cascades(
    repo: TasksRepository, db: AsyncEngine
) -> None:
    t = await task(repo, "t")
    await entry(repo, t.id, NOW, 5)
    await repo.delete_task(t.id)
    async with db.connect() as conn:
        assert (await conn.execute(text("SELECT count(*) FROM time_entries"))).scalar_one() == 0
