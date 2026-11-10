import json
from datetime import date, datetime
from decimal import Decimal
from typing import Any

from sqlalchemy import Row, Table, delete, select, text, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine

from gv_api.history import Point
from gv_api.tasks.errors import (
    ActiveTimeEntryExistsError,
    CircularDependencyError,
    NotFoundError,
    ParentNotFoundError,
    ProjectCycleError,
)
from gv_api.tasks.schemas import (
    ActiveProject,
    ActiveTimeEntryResponse,
    ProjectChildNode,
    ProjectChildrenResponse,
    ProjectDetailResponse,
    ProjectFastResponse,
    ProjectParentCandidate,
    ProjectResponse,
    TaskByDueDateResponse,
    TaskDepRef,
    TaskDetailResponse,
    TaskFastResponse,
    TaskFullResponse,
    TaskResponse,
    TaskTimeEntriesResponse,
    TimeEntryResponse,
    TimeEntrySummaryResponse,
    TimeEntryWithTaskResponse,
    TodoResponse,
    UnfinishedTask,
)
from gv_api.tasks.tables import projects, task_dependencies, tasks, time_entries, todos

# The dependency columns every task listing shares: open dependencies, dependents, and whether
# an open dependency blocks it.
_DEPS = """
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at)
                              ORDER BY t2.name)
              FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on
              WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name)
              FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id
              WHERE td.depends_on = t.id), '[]')::json AS blocks,
    EXISTS (SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on
            WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked"""

_TODOS = """
    COALESCE((SELECT json_agg(json_build_object('id', td.id, 'name', td.name, 'is_done', td.is_done)
                              ORDER BY td.is_done ASC NULLS LAST, td.id)
              FROM todos td WHERE td.task_id = t.id), '[]')::json AS todos"""

_TIME_SPENT = (
    "COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint"
    " AS time_spent"
)

_PARENT_CANDIDATES = text("""
WITH RECURSIVE
subtree AS (
    SELECT id FROM projects WHERE id = :id
    UNION
    SELECT c.id FROM projects c JOIN subtree s ON c.parent_id = s.id
),
tree AS (
    SELECT id, name, ARRAY[name::text] AS path FROM projects WHERE parent_id IS NULL
    UNION ALL
    SELECT c.id, c.name, t.path || c.name::text FROM projects c JOIN tree t ON c.parent_id = t.id
)
SELECT t.id, t.name, array_to_string(t.path, ' / ') AS path
FROM tree t JOIN projects p ON p.id = t.id
WHERE t.id NOT IN (SELECT id FROM subtree)
  AND (p.finished_at IS NULL OR t.id = (SELECT parent_id FROM projects WHERE id = :id))
ORDER BY t.path
""")

_SUBTREE_CONTAINS = text("""
WITH RECURSIVE subtree AS (
    SELECT id FROM projects WHERE id = :root
    UNION
    SELECT c.id FROM projects c JOIN subtree s ON c.parent_id = s.id
)
SELECT EXISTS (SELECT 1 FROM subtree WHERE id = :candidate)
""")

_TASKS_FAST = text("""
WITH RECURSIVE project_tree AS (
    SELECT id, name, ARRAY[name::text] AS sort_path
    FROM projects
    WHERE parent_id IS NULL AND finished_at IS NULL AND started_at IS NOT NULL
    UNION ALL
    SELECT c.id, c.name, pt.sort_path || c.name::text
    FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
    WHERE c.finished_at IS NULL AND c.started_at IS NOT NULL
)
SELECT t.id, t.name, t.project_id, pt.name AS project_name, t.task_type, t.recurrence, t.priority
FROM tasks t LEFT JOIN project_tree pt ON t.project_id = pt.id
WHERE t.finished_at IS NULL AND (t.project_id IS NULL OR pt.id IS NOT NULL)
ORDER BY CASE WHEN t.project_id IS NULL THEN 1 ELSE 0 END, pt.sort_path, t.name
""")

_UNFINISHED_TASKS = text(f"""
SELECT t.id, t.project_id, t.name, t.description, e.effective_due_at AS due_at,
    t.started_at, t.task_type, t.recurrence, t.priority, {_DEPS}
FROM tasks t JOIN task_effective_due e ON e.task_id = t.id
WHERE t.finished_at IS NULL
  AND (CAST(:min_priority AS int) IS NULL OR t.priority <= CAST(:min_priority AS int))
ORDER BY
  CASE
    WHEN t.task_type = 'standard' AND t.started_at IS NOT NULL THEN 0
    WHEN t.task_type = 'continuous' THEN 1
    WHEN t.task_type = 'recurring' THEN 2
    ELSE 3
  END,
  t.name
""")  # noqa: S608

_PROJECT_WITH_DESCENDANTS = text("""
WITH RECURSIVE project_tree AS (
    SELECT p.id, p.parent_id, p.name, p.description, p.due_at, p.started_at, p.finished_at,
        p.priority, 0 AS depth, ARRAY[p.id]::int[] AS path
    FROM projects p WHERE p.id = :id
    UNION ALL
    SELECT c.id, c.parent_id, c.name, c.description, c.due_at, c.started_at, c.finished_at,
        c.priority, pt.depth + 1, pt.path || c.id
    FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
),
project_direct_time AS (
    SELECT t.project_id,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint
            AS direct_time
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.project_id IN (SELECT id FROM project_tree)
    GROUP BY t.project_id
)
SELECT pt.id, pt.parent_id, pt.name, pt.description, pt.due_at, pt.started_at, pt.finished_at,
    pt.priority,
    COALESCE((SELECT SUM(pdt.direct_time)::bigint
              FROM project_direct_time pdt JOIN project_tree d ON d.id = pdt.project_id
              WHERE pt.id = ANY(d.path)), 0)::bigint AS time_spent
FROM project_tree pt
ORDER BY pt.depth, pt.due_at ASC NULLS LAST, pt.name
""")

_TASKS_OF_PROJECTS = text(f"""
SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
    t.task_type, t.recurrence, t.priority, {_TIME_SPENT}, {_DEPS}, {_TODOS}
FROM tasks t
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.project_id = ANY(:project_ids)
GROUP BY t.id
ORDER BY
    CASE WHEN t.finished_at IS NOT NULL THEN 2 WHEN t.started_at IS NOT NULL THEN 0 ELSE 1 END,
    t.due_at ASC NULLS LAST,
    t.name
""")  # noqa: S608

_TASKS_BY_DUE_DATE = text(f"""
SELECT t.id, t.name, t.description, e.effective_due_at AS due_at, t.started_at, t.task_type,
    t.recurrence, t.priority, t.estimate_hours,
    p.id AS project_id, p.name AS project_name, p.due_at AS project_due_at,
    {_TIME_SPENT}, {_DEPS}
FROM tasks t
JOIN task_effective_due e ON e.task_id = t.id
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.finished_at IS NULL AND (e.effective_due_at IS NOT NULL OR p.due_at IS NOT NULL)
GROUP BY t.id, p.id, e.effective_due_at
ORDER BY e.effective_due_at ASC NULLS LAST, p.due_at ASC NULLS LAST, t.name
""")  # noqa: S608

_TASK = text(f"""
SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
    t.task_type, t.recurrence, t.priority, t.estimate_hours, p.name AS project_name,
    {_TIME_SPENT}, {_DEPS}, {_TODOS}
FROM tasks t
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.id = :id
GROUP BY t.id, p.name
""")  # noqa: S608

_TASK_WITH_ENTRIES = text(f"""
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        t.task_type, t.recurrence, t.priority, {_TIME_SPENT}, {_DEPS}
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.id = :id
    GROUP BY t.id
)
SELECT ti.*, te.id AS entry_id, te.started_at AS entry_started_at,
    te.finished_at AS entry_finished_at, te.comment
FROM task_info ti LEFT JOIN time_entries te ON te.task_id = ti.id
ORDER BY te.started_at
""")  # noqa: S608

_TASK_DEPENDENCIES = text(f"""
SELECT {_DEPS} FROM (SELECT CAST(:id AS int) AS id) AS t
""")  # noqa: S608

_PROJECT = text("""
WITH RECURSIVE project_tree AS (
    SELECT id FROM projects WHERE id = :id
    UNION ALL
    SELECT c.id FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
),
project_direct_time AS (
    SELECT t.project_id,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint
            AS direct_time
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.project_id IN (SELECT id FROM project_tree)
    GROUP BY t.project_id
)
SELECT p.id, p.parent_id, p.name, p.description, p.due_at, p.started_at, p.finished_at,
    p.priority,
    COALESCE((SELECT SUM(pdt.direct_time)::bigint FROM project_direct_time pdt), 0)::bigint
        AS time_spent
FROM projects p WHERE p.id = :id
""")

_FINISH_SUBTREE_PROJECTS = text("""
WITH RECURSIVE project_tree AS (
    SELECT id FROM projects WHERE id = :id
    UNION ALL
    SELECT c.id FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
)
UPDATE projects p SET finished_at = now()
FROM project_tree pt
WHERE p.id = pt.id AND p.id <> :id AND p.finished_at IS NULL
""")

_FINISH_SUBTREE_TASKS = text("""
WITH RECURSIVE project_tree AS (
    SELECT id FROM projects WHERE id = :id
    UNION ALL
    SELECT c.id FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
)
UPDATE tasks t SET finished_at = now()
FROM project_tree pt
WHERE t.project_id = pt.id AND t.finished_at IS NULL
""")

_ACTIVE_ENTRY = text("""
SELECT te.id, te.task_id, te.started_at, te.finished_at, te.comment,
       t.name AS task_name, t.description AS task_description,
       t.task_type, t.recurrence, t.priority, p.name AS project_name
FROM time_entries te
JOIN tasks t ON t.id = te.task_id
LEFT JOIN projects p ON p.id = t.project_id
WHERE te.finished_at IS NULL
""")

_SUMMARY = text("""
SELECT
    COALESCE(SUM(CASE WHEN finished_at >= :today_start
        THEN EXTRACT(EPOCH FROM (finished_at - GREATEST(started_at, :today_start)))::bigint
        ELSE 0 END), 0)::bigint AS today,
    COALESCE(SUM(EXTRACT(EPOCH FROM (finished_at - GREATEST(started_at, :week_start)))::bigint),
             0)::bigint AS week
FROM time_entries
WHERE finished_at IS NOT NULL AND finished_at >= :week_start
""")

# Entries are split at period boundaries in the zone and summed; empty periods are zero.
_ENTRY_HISTORY = text("""
WITH all_periods AS (
    SELECT period_start::date AS date
    FROM generate_series(CAST(:start_at AS date), CAST(:end_at AS date),
                         ('1 ' || CAST(:frequency AS text))::interval) AS period_start
),
entry_periods AS (
    SELECT te.started_at, te.finished_at, gs.period_start
    FROM time_entries te,
    LATERAL generate_series(
        date_trunc(CAST(:frequency AS text), te.started_at AT TIME ZONE CAST(:tz AS text)),
        date_trunc(CAST(:frequency AS text), te.finished_at AT TIME ZONE CAST(:tz AS text)),
        ('1 ' || CAST(:frequency AS text))::interval
    ) AS gs(period_start)
    WHERE te.finished_at IS NOT NULL
      AND (te.started_at AT TIME ZONE CAST(:tz AS text))::date >= CAST(:start_at AS date)
      AND (te.started_at AT TIME ZONE CAST(:tz AS text))::date <= CAST(:end_at AS date)
),
sums AS (
    SELECT ep.period_start::date AS date,
        (SUM(EXTRACT(EPOCH FROM (
            LEAST(ep.finished_at, (ep.period_start + ('1 ' || CAST(:frequency AS text))::interval)
                                  AT TIME ZONE CAST(:tz AS text))
            - GREATEST(ep.started_at, ep.period_start AT TIME ZONE CAST(:tz AS text))
        ))) / 3600)::real AS value
    FROM entry_periods ep
    GROUP BY ep.period_start
)
SELECT ap.date, COALESCE(s.value, 0)::real AS value
FROM all_periods ap LEFT JOIN sums s ON s.date = ap.date
ORDER BY ap.date
""")

_ENTRIES_IN_RANGE = text("""
SELECT te.id, te.task_id, t.name AS task_name, t.task_type, t.recurrence, t.priority,
    t.finished_at AS task_finished_at, p.id AS project_id, p.name AS project_name,
    te.started_at, te.finished_at, te.comment,
    EXTRACT(EPOCH FROM (COALESCE(te.finished_at, now()) - te.started_at))::bigint AS time_spent
FROM time_entries te
JOIN tasks t ON t.id = te.task_id
LEFT JOIN projects p ON p.id = t.project_id
WHERE te.started_at <= :end_time AND (te.finished_at >= :start_time OR te.finished_at IS NULL)
ORDER BY te.started_at DESC
""")

_WOULD_CYCLE = text("SELECT task_dependency_would_cycle(:id, CAST(:deps AS int[]))")
_BLOCKS_WOULD_CYCLE = text("""
SELECT EXISTS (
    SELECT 1 FROM unnest(CAST(:blocks AS int[])) AS b(id)
    WHERE task_dependency_would_cycle(b.id, ARRAY[CAST(:id AS int)])
)
""")
_UPSERT_DEPENDENCIES = text("""
INSERT INTO task_dependencies (task_id, depends_on)
SELECT :id, t.id FROM unnest(CAST(:deps AS int[])) AS dep_id
JOIN tasks t ON t.id = dep_id AND t.finished_at IS NULL
ON CONFLICT (task_id, depends_on) DO NOTHING
""")
_UPSERT_BLOCKS = text("""
INSERT INTO task_dependencies (task_id, depends_on)
SELECT t.id, :id FROM unnest(CAST(:blocks AS int[])) AS block_id
JOIN tasks t ON t.id = block_id AND t.finished_at IS NULL
ON CONFLICT (task_id, depends_on) DO NOTHING
""")

_PROJECT_COLUMNS = (
    projects.c.id,
    projects.c.parent_id,
    projects.c.name,
    projects.c.description,
    projects.c.due_at,
    projects.c.started_at,
    projects.c.finished_at,
    projects.c.priority,
)
_TASK_COLUMNS = (
    tasks.c.id,
    tasks.c.project_id,
    tasks.c.name,
    tasks.c.description,
    tasks.c.due_at,
    tasks.c.started_at,
    tasks.c.finished_at,
    tasks.c.task_type,
    tasks.c.recurrence,
    tasks.c.priority,
    tasks.c.estimate_hours,
)
_ENTRY_COLUMNS = (
    time_entries.c.id,
    time_entries.c.task_id,
    time_entries.c.started_at,
    time_entries.c.finished_at,
    time_entries.c.comment,
)


def _refs(raw: Any) -> list[TaskDepRef]:
    items = json.loads(raw) if isinstance(raw, str) else raw
    return [TaskDepRef.model_construct(**{"due_at": None} | r) for r in items or []]


def _todos(raw: Any, task_id: int) -> list[TodoResponse]:
    items = json.loads(raw) if isinstance(raw, str) else raw
    return [TodoResponse.model_construct(task_id=task_id, **r) for r in items or []]


def _with_deps(row: Row[Any]) -> dict[str, Any]:
    data = dict(row._mapping)
    data["depends_on"] = _refs(data["depends_on"])
    data["blocks"] = _refs(data["blocks"])
    return data


def _due_date(value: datetime | None) -> date | None:
    return None if value is None else value.date()


class TasksRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def _one(self, query: Any, params: dict[str, Any] | None = None) -> Row[Any] | None:
        async with self._engine.connect() as conn:
            return (await conn.execute(query, params or {})).one_or_none()

    async def _all(self, query: Any, params: dict[str, Any] | None = None) -> list[Row[Any]]:
        async with self._engine.connect() as conn:
            return list(await conn.execute(query, params or {}))

    async def _delete(self, table: Table, id: int) -> None:
        async with self._engine.begin() as conn:
            result = await conn.execute(delete(table).where(table.c.id == id))
        if result.rowcount == 0:
            raise NotFoundError

    # Projects

    async def create_project(
        self,
        name: str,
        description: str | None,
        due_at: datetime | None,
        parent_id: int | None,
        priority: int,
    ) -> ProjectResponse:
        stmt = (
            projects.insert()
            .values(
                name=name,
                description=description,
                due_at=_due_date(due_at),
                parent_id=parent_id,
                priority=priority,
            )
            .returning(*_PROJECT_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return ProjectResponse.model_construct(**row._mapping)

    async def update_project(self, id: int, values: dict[str, Any]) -> ProjectResponse:
        """`values` holds only the columns to change; a parent move is checked for cycles."""
        if "due_at" in values:
            values["due_at"] = _due_date(values["due_at"])
        async with self._engine.begin() as conn:
            if values.get("parent_id") is not None:
                await self._check_project_move(conn, id, values["parent_id"])
            stmt = (
                update(projects)
                .where(projects.c.id == id)
                .values(values or {"id": projects.c.id})
                .returning(*_PROJECT_COLUMNS)
            )
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return ProjectResponse.model_construct(**row._mapping)

    @staticmethod
    async def _check_project_move(conn: AsyncConnection, id: int, parent_id: int) -> None:
        # The lock serialises moves, so two cannot pass the cycle check together.
        await conn.execute(text("SELECT pg_advisory_xact_lock(hashtext('projects_tree'))"))
        exists = select(projects.c.id).where(projects.c.id == id).exists()
        if not (await conn.execute(select(exists))).scalar_one():
            raise NotFoundError
        parent = select(projects.c.id).where(projects.c.id == parent_id).exists()
        if not (await conn.execute(select(parent))).scalar_one():
            raise ParentNotFoundError
        params = {"root": id, "candidate": parent_id}
        if (await conn.execute(_SUBTREE_CONTAINS, params)).scalar_one():
            raise ProjectCycleError

    async def list_project_parent_candidates(self, id: int) -> list[ProjectParentCandidate]:
        if await self._one(select(projects.c.id).where(projects.c.id == id)) is None:
            raise NotFoundError
        rows = await self._all(_PARENT_CANDIDATES, {"id": id})
        return [ProjectParentCandidate.model_construct(**r._mapping) for r in rows]

    async def list_projects_fast(self) -> list[ProjectFastResponse]:
        stmt = (
            select(projects.c.id, projects.c.name, projects.c.priority)
            .where(projects.c.started_at.is_not(None), projects.c.finished_at.is_(None))
            .order_by(projects.c.name)
        )
        return [ProjectFastResponse.model_construct(**r._mapping) for r in await self._all(stmt)]

    async def get_root_projects(self) -> list[ProjectResponse]:
        stmt = (
            select(*_PROJECT_COLUMNS)
            .where(projects.c.parent_id.is_(None), projects.c.finished_at.is_(None))
            .order_by(projects.c.name)
        )
        return [ProjectResponse.model_construct(**r._mapping) for r in await self._all(stmt)]

    async def get_active_projects(self) -> list[ActiveProject]:
        stmt = (
            select(projects.c.id, projects.c.parent_id, projects.c.name, projects.c.due_at)
            .where(projects.c.started_at.is_not(None), projects.c.finished_at.is_(None))
            .order_by(projects.c.name)
        )
        return [ActiveProject.model_construct(**r._mapping) for r in await self._all(stmt)]

    async def get_project(self, id: int) -> ProjectDetailResponse:
        row = await self._one(_PROJECT, {"id": id})
        if row is None:
            raise NotFoundError
        return ProjectDetailResponse.model_construct(**row._mapping)

    async def get_project_children(self, id: int) -> ProjectChildrenResponse:
        tree = await self._all(_PROJECT_WITH_DESCENDANTS, {"id": id})
        if not tree:
            raise NotFoundError
        task_rows = await self._all(_TASKS_OF_PROJECTS, {"project_ids": [p.id for p in tree]})
        tasks_by_project: dict[int, list[ProjectChildNode]] = {}
        for row in task_rows:
            data = _with_deps(row)
            data["todos"] = _todos(data["todos"], row.id)
            node = ProjectChildNode.model_construct(type="task", **data)
            tasks_by_project.setdefault(row.project_id, []).append(node)

        root, *descendants = tree
        children = [
            ProjectChildNode.model_construct(
                type="project", **{k: v for k, v in p._mapping.items() if k != "priority"}
            )
            for p in descendants
            if p.parent_id == id
        ]
        children += _dependency_order(tasks_by_project.get(id, []))
        children.sort(key=_status_rank)
        return ProjectChildrenResponse.model_construct(
            project=ProjectDetailResponse.model_construct(**root._mapping), children=children
        )

    async def finish_subtree(self, id: int) -> None:
        """Finish the open sub-projects and tasks under project `id`."""
        async with self._engine.begin() as conn:
            await conn.execute(_FINISH_SUBTREE_PROJECTS, {"id": id})
            await conn.execute(_FINISH_SUBTREE_TASKS, {"id": id})

    async def delete_project(self, id: int) -> None:
        await self._delete(projects, id)

    # Tasks

    async def create_task(
        self,
        project_id: int | None,
        name: str,
        description: str | None,
        due_at: datetime | None,
        task_type: str,
        recurrence: int | None,
        priority: int,
        estimate_hours: Decimal | None,
    ) -> TaskResponse:
        stmt = (
            tasks.insert()
            .values(
                project_id=project_id,
                name=name,
                description=description,
                due_at=_due_date(due_at),
                task_type=task_type,
                recurrence=recurrence,
                priority=priority,
                estimate_hours=estimate_hours,
            )
            .returning(*_TASK_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return TaskResponse.model_construct(**row._mapping)

    async def update_task(self, id: int, values: dict[str, Any]) -> TaskResponse:
        if "due_at" in values:
            values["due_at"] = _due_date(values["due_at"])
        stmt = (
            update(tasks)
            .where(tasks.c.id == id)
            .values(values or {"id": tasks.c.id})
            .returning(*_TASK_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return TaskResponse.model_construct(**row._mapping)

    async def get_unfinished_tasks(self, min_priority: int | None) -> list[UnfinishedTask]:
        rows = await self._all(_UNFINISHED_TASKS, {"min_priority": min_priority})
        return [UnfinishedTask.model_construct(**_with_deps(r)) for r in rows]

    async def get_task(self, id: int) -> TaskFullResponse:
        row = await self._one(_TASK, {"id": id})
        if row is None:
            raise NotFoundError
        data = _with_deps(row)
        data["todos"] = _todos(data["todos"], id)
        return TaskFullResponse.model_construct(**data)

    async def get_tasks_by_due_date(self) -> list[TaskByDueDateResponse]:
        rows = await self._all(_TASKS_BY_DUE_DATE)
        return [
            TaskByDueDateResponse.model_construct(**(_with_deps(r) | {"dependent_hours": []}))
            for r in rows
        ]

    async def delete_task(self, id: int) -> None:
        await self._delete(tasks, id)

    async def replace_task_dependencies(self, id: int, depends_on: list[int]) -> None:
        async with self._engine.begin() as conn:
            params = {"id": id, "deps": depends_on}
            if depends_on and (await conn.execute(_WOULD_CYCLE, params)).scalar_one():
                raise CircularDependencyError
            await conn.execute(
                delete(task_dependencies).where(
                    task_dependencies.c.task_id == id,
                    task_dependencies.c.depends_on.not_in(depends_on),
                )
            )
            if depends_on:
                await conn.execute(_UPSERT_DEPENDENCIES, params)

    async def replace_task_blocks(self, id: int, blocks: list[int]) -> None:
        async with self._engine.begin() as conn:
            params = {"id": id, "blocks": blocks}
            if blocks and (await conn.execute(_BLOCKS_WOULD_CYCLE, params)).scalar_one():
                raise CircularDependencyError
            await conn.execute(
                delete(task_dependencies).where(
                    task_dependencies.c.depends_on == id,
                    task_dependencies.c.task_id.not_in(blocks),
                )
            )
            if blocks:
                await conn.execute(_UPSERT_BLOCKS, params)

    async def get_task_dependencies(
        self, id: int
    ) -> tuple[list[TaskDepRef], list[TaskDepRef], bool]:
        row = await self._one(_TASK_DEPENDENCIES, {"id": id})
        assert row is not None  # noqa: S101 - a SELECT without FROM rows always returns one
        return _refs(row.depends_on), _refs(row.blocks), row.blocked

    async def list_tasks_fast(self) -> list[TaskFastResponse]:
        return [
            TaskFastResponse.model_construct(**r._mapping) for r in await self._all(_TASKS_FAST)
        ]

    # Todos

    async def create_todo(self, task_id: int, name: str) -> TodoResponse:
        stmt = (
            todos.insert()
            .values(task_id=task_id, name=name)
            .returning(todos.c.id, todos.c.task_id, todos.c.name, todos.c.is_done)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return TodoResponse.model_construct(**row._mapping)

    async def update_todo(self, id: int, values: dict[str, Any]) -> TodoResponse:
        stmt = (
            update(todos)
            .where(todos.c.id == id)
            .values(values or {"id": todos.c.id})
            .returning(todos.c.id, todos.c.task_id, todos.c.name, todos.c.is_done)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return TodoResponse.model_construct(**row._mapping)

    async def delete_todo(self, id: int) -> None:
        await self._delete(todos, id)

    # Time entries

    async def create_time_entry(
        self,
        task_id: int,
        started_at: datetime,
        finished_at: datetime | None,
        comment: str | None,
    ) -> TimeEntryResponse:
        stmt = (
            time_entries.insert()
            .values(
                task_id=task_id, started_at=started_at, finished_at=finished_at, comment=comment
            )
            .returning(*_ENTRY_COLUMNS)
        )
        try:
            async with self._engine.begin() as conn:
                row = (await conn.execute(stmt)).one()
        except IntegrityError as err:
            if getattr(err.orig, "sqlstate", None) == "23505":  # the one-active-entry index
                raise ActiveTimeEntryExistsError from err
            raise
        return TimeEntryResponse.model_construct(**row._mapping)

    async def update_time_entry(self, id: int, values: dict[str, Any]) -> TimeEntryResponse:
        stmt = (
            update(time_entries)
            .where(time_entries.c.id == id)
            .values(values or {"id": time_entries.c.id})
            .returning(*_ENTRY_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return TimeEntryResponse.model_construct(**row._mapping)

    async def delete_time_entry(self, id: int) -> None:
        await self._delete(time_entries, id)

    async def get_task_time_entries(self, id: int) -> TaskTimeEntriesResponse:
        rows = await self._all(_TASK_WITH_ENTRIES, {"id": id})
        if not rows:
            raise NotFoundError
        entries = [
            TimeEntryResponse.model_construct(
                id=r.entry_id,
                task_id=id,
                started_at=r.entry_started_at,
                finished_at=r.entry_finished_at,
                comment=r.comment,
            )
            for r in rows
            if r.entry_id is not None
        ]
        task = TaskDetailResponse.model_construct(**_with_deps(rows[0]))
        return TaskTimeEntriesResponse.model_construct(task=task, time_entries=entries)

    async def get_active_time_entry(self) -> ActiveTimeEntryResponse:
        row = await self._one(_ACTIVE_ENTRY)
        if row is None:
            raise NotFoundError
        return ActiveTimeEntryResponse.model_construct(**row._mapping)

    async def get_time_entry_summary(
        self, today_start: datetime, week_start: datetime
    ) -> TimeEntrySummaryResponse:
        row = await self._one(_SUMMARY, {"today_start": today_start, "week_start": week_start})
        assert row is not None  # noqa: S101 - an aggregate always returns one row
        return TimeEntrySummaryResponse(today=row.today, week=row.week)

    async def get_time_entry_history(
        self, frequency: str, tz: str, start_at: date, end_at: date
    ) -> list[Point]:
        params = {"frequency": frequency, "tz": tz, "start_at": start_at, "end_at": end_at}
        rows = await self._all(_ENTRY_HISTORY, params)
        return [Point.model_construct(date=r.date, value=r.value) for r in rows]

    async def get_time_entries_by_date_range(
        self, start: datetime, end: datetime
    ) -> list[TimeEntryWithTaskResponse]:
        rows = await self._all(_ENTRIES_IN_RANGE, {"start_time": start, "end_time": end})
        return [TimeEntryWithTaskResponse.model_construct(**r._mapping) for r in rows]


def _dependency_order(nodes: list[ProjectChildNode]) -> list[ProjectChildNode]:
    """Each task after the listed tasks it depends on, otherwise in the given order."""
    index = {n.id: i for i, n in enumerate(nodes)}
    waiting = [0] * len(nodes)
    unblocks: list[list[int]] = [[] for _ in nodes]
    for i, node in enumerate(nodes):
        for dep in node.depends_on or []:
            if dep.id in index:
                waiting[i] += 1
                unblocks[index[dep.id]].append(i)
    ready = sorted(i for i, n in enumerate(waiting) if n == 0)
    out: list[ProjectChildNode] = []
    while ready:
        current = ready.pop(0)
        out.append(nodes[current])
        for j in unblocks[current]:
            waiting[j] -= 1
            if waiting[j] == 0:
                ready.append(j)
                ready.sort()
    return out if len(out) == len(nodes) else nodes


def _status_rank(child: ProjectChildNode) -> int:
    """Projects in progress, projects not started, then continuous, recurring, in-progress and
    to-do tasks, then finished projects and finished tasks."""
    if child.type == "project":
        if child.finished_at is not None:
            return 6
        return 0 if child.started_at is not None else 1
    if child.finished_at is not None:
        return 7
    if child.started_at is None:
        return 5
    return {"continuous": 2, "recurring": 3}.get(child.task_type or "", 4)
