from datetime import date, datetime, time, timedelta
from decimal import ROUND_HALF_UP, Decimal
from typing import Any, Protocol
from zoneinfo import ZoneInfo

from gv_api import history
from gv_api.core.dates import Clock, utcnow
from gv_api.tasks import budget, urgency
from gv_api.tasks.schemas import (
    ActiveProject,
    ActiveTimeEntryResponse,
    ActiveTreeNode,
    CreateProjectRequest,
    CreateTaskRequest,
    CreateTimeEntryRequest,
    CreateTodoRequest,
    DueSoonSlackResponse,
    ProjectChildrenResponse,
    ProjectDetailResponse,
    ProjectFastResponse,
    ProjectParentCandidate,
    ProjectResponse,
    TaskByDueDateResponse,
    TaskDepRef,
    TaskFastResponse,
    TaskFullResponse,
    TaskResponse,
    TaskTimeEntriesResponse,
    TimeEntryResponse,
    TimeEntrySummaryResponse,
    TimeEntryWithTaskResponse,
    TodoResponse,
    UnfinishedTask,
    UpdateProjectRequest,
    UpdateTaskRequest,
    UpdateTimeEntryRequest,
    UpdateTodoRequest,
)

DEFAULT_PRIORITY = 3


class Repository(Protocol):
    async def create_project(
        self,
        name: str,
        description: str | None,
        due_at: datetime | None,
        parent_id: int | None,
        priority: int,
    ) -> ProjectResponse: ...
    async def update_project(self, id: int, values: dict[str, Any]) -> ProjectResponse: ...
    async def list_project_parent_candidates(self, id: int) -> list[ProjectParentCandidate]: ...
    async def list_projects_fast(self) -> list[ProjectFastResponse]: ...
    async def get_root_projects(self) -> list[ProjectResponse]: ...
    async def get_active_projects(self) -> list[ActiveProject]: ...
    async def get_project(self, id: int) -> ProjectDetailResponse: ...
    async def get_project_children(self, id: int) -> ProjectChildrenResponse: ...
    async def finish_subtree(self, id: int) -> None: ...
    async def delete_project(self, id: int) -> None: ...
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
    ) -> TaskResponse: ...
    async def update_task(self, id: int, values: dict[str, Any]) -> TaskResponse: ...
    async def get_unfinished_tasks(self, min_priority: int | None) -> list[UnfinishedTask]: ...
    async def get_task(self, id: int) -> TaskFullResponse: ...
    async def get_tasks_by_due_date(self) -> list[TaskByDueDateResponse]: ...
    async def delete_task(self, id: int) -> None: ...
    async def replace_task_dependencies(self, id: int, depends_on: list[int]) -> None: ...
    async def replace_task_blocks(self, id: int, blocks: list[int]) -> None: ...
    async def get_task_dependencies(
        self, id: int
    ) -> tuple[list[TaskDepRef], list[TaskDepRef], bool]: ...
    async def list_tasks_fast(self) -> list[TaskFastResponse]: ...
    async def create_todo(self, task_id: int, name: str) -> TodoResponse: ...
    async def update_todo(self, id: int, values: dict[str, Any]) -> TodoResponse: ...
    async def delete_todo(self, id: int) -> None: ...
    async def create_time_entry(
        self,
        task_id: int,
        started_at: datetime,
        finished_at: datetime | None,
        comment: str | None,
    ) -> TimeEntryResponse: ...
    async def update_time_entry(self, id: int, values: dict[str, Any]) -> TimeEntryResponse: ...
    async def delete_time_entry(self, id: int) -> None: ...
    async def get_task_time_entries(self, id: int) -> TaskTimeEntriesResponse: ...
    async def get_active_time_entry(self) -> ActiveTimeEntryResponse: ...
    async def get_time_entry_summary(
        self, today_start: datetime, week_start: datetime
    ) -> TimeEntrySummaryResponse: ...
    async def get_time_entry_history(
        self, frequency: str, tz: str, start_at: date, end_at: date
    ) -> list[history.Point]: ...
    async def get_time_entries_by_date_range(
        self, start: datetime, end: datetime
    ) -> list[TimeEntryWithTaskResponse]: ...


class DayFreeBusy(Protocol):
    @property
    def date(self) -> date: ...
    @property
    def free_hours(self) -> Decimal: ...
    @property
    def capacity_hours(self) -> Decimal: ...


class CapacityProvider(Protocol):
    async def free_busy_range(self, start: date, end: date) -> list[DayFreeBusy]: ...


class PlannedHoursProvider(Protocol):
    async def planned_hours_by_task(
        self, task_ids: list[int], since: datetime
    ) -> dict[int, Decimal]: ...


def _set(model: Any, *names: str) -> dict[str, Any]:
    """The given fields the client sent with a value."""
    return {n: getattr(model, n) for n in names if getattr(model, n) is not None}


def _sent(model: Any, *names: str) -> dict[str, Any]:
    """The given fields the client sent, null included (null clears)."""
    return {n: getattr(model, n) for n in names if n in model.model_fields_set}


class TasksService:
    def __init__(self, repo: Repository, loc: ZoneInfo, now: Clock = utcnow) -> None:
        self._repo = repo
        self._loc = loc
        self._now = now
        self._capacity: CapacityProvider | None = None
        self._plan: PlannedHoursProvider | None = None

    def set_urgency_providers(self, capacity: CapacityProvider, plan: PlannedHoursProvider) -> None:
        """A setter because the plan service depends on this one."""
        self._capacity = capacity
        self._plan = plan

    def _clock(self) -> datetime:
        return self._now().astimezone(self._loc)

    def _midnight(self, day: date) -> datetime:
        return datetime.combine(day, time(), self._loc)

    # Projects

    async def create_project(self, req: CreateProjectRequest) -> ProjectResponse:
        priority = DEFAULT_PRIORITY if req.priority is None else req.priority
        return await self._repo.create_project(
            req.name, req.description, req.due_at, req.parent_id, priority
        )

    async def update_project(self, id: int, req: UpdateProjectRequest) -> ProjectResponse:
        values = _set(req, "name", "description", "priority")
        values |= _sent(req, "due_at", "parent_id", "started_at", "finished_at")
        project = await self._repo.update_project(id, values)
        if req.finished_at is not None:
            await self._repo.finish_subtree(id)
        return project

    async def list_project_parent_candidates(self, id: int) -> list[ProjectParentCandidate]:
        return await self._repo.list_project_parent_candidates(id)

    async def list_projects_fast(self) -> list[ProjectFastResponse]:
        return await self._repo.list_projects_fast()

    async def get_root_projects(self) -> list[ProjectResponse]:
        return await self._repo.get_root_projects()

    async def get_project(self, id: int) -> ProjectDetailResponse:
        return await self._repo.get_project(id)

    async def get_project_children(self, id: int) -> ProjectChildrenResponse:
        return await self._repo.get_project_children(id)

    async def delete_project(self, id: int) -> None:
        await self._repo.delete_project(id)

    async def get_active_tree(self, min_priority: int | None) -> list[ActiveTreeNode]:
        projects = await self._repo.get_active_projects()
        tasks = await self._repo.get_unfinished_tasks(min_priority)
        kids: dict[int, list[ActiveTreeNode]] = {p.id: [] for p in projects}
        nodes = {
            p.id: ActiveTreeNode.model_construct(
                id=p.id, type="project", name=p.name, due_at=p.due_at, children=kids[p.id]
            )
            for p in projects
        }
        orphans: list[ActiveTreeNode] = []
        for t in tasks:
            node = ActiveTreeNode.model_construct(
                id=t.id,
                type="task",
                name=t.name,
                description=t.description,
                due_at=t.due_at,
                started_at=t.started_at,
                task_type=t.task_type,
                recurrence=t.recurrence,
                priority=t.priority,
                depends_on=t.depends_on,
                blocks=t.blocks,
                blocked=t.blocked,
            )
            if t.project_id is None:
                orphans.append(node)
            elif t.project_id in kids:
                kids[t.project_id].append(node)

        # Each sub-project is put in front of its parent's children, so they come before the
        # tasks, in reverse name order. Projects under an inactive parent are roots.
        roots: list[ActiveTreeNode] = []
        for p in projects:
            if p.parent_id is not None and p.parent_id in kids:
                kids[p.parent_id].insert(0, nodes[p.id])
            else:
                roots.append(nodes[p.id])
        return roots + orphans

    # Tasks

    async def create_task(self, req: CreateTaskRequest) -> TaskResponse:
        task_type = req.task_type or "standard"
        priority = DEFAULT_PRIORITY
        if req.priority is not None:
            priority = req.priority
        elif req.project_id is not None:
            # Without a priority of its own, a task takes its project's.
            priority = (await self._repo.get_project(req.project_id)).priority
        task = await self._repo.create_task(
            req.project_id,
            req.name,
            req.description,
            req.due_at,
            task_type,
            req.recurrence,
            priority,
            req.estimate_hours,
        )
        if req.depends_on:
            await self._repo.replace_task_dependencies(task.id, req.depends_on)
        return await self._with_dependencies(task)

    async def update_task(self, id: int, req: UpdateTaskRequest) -> TaskResponse:
        values = _set(req, "name", "description", "project_id", "task_type", "priority")
        values |= _sent(req, "due_at", "started_at", "finished_at", "estimate_hours")
        if req.task_type is not None and req.task_type != "recurring":
            values["recurrence"] = None
        if req.recurrence is not None:
            values["recurrence"] = req.recurrence
        task = await self._repo.update_task(id, values)
        if req.depends_on is not None:
            await self._repo.replace_task_dependencies(task.id, req.depends_on)
        if req.blocks is not None:
            await self._repo.replace_task_blocks(task.id, req.blocks)
        return await self._with_dependencies(task)

    async def _with_dependencies(self, task: TaskResponse) -> TaskResponse:
        depends_on, blocks, blocked = await self._repo.get_task_dependencies(task.id)
        return task.model_copy(
            update={"depends_on": depends_on, "blocks": blocks, "blocked": blocked}
        )

    async def get_task(self, id: int) -> TaskFullResponse:
        return await self._repo.get_task(id)

    async def get_task_time_entries(self, id: int) -> TaskTimeEntriesResponse:
        return await self._repo.get_task_time_entries(id)

    async def list_tasks_fast(self) -> list[TaskFastResponse]:
        return await self._repo.list_tasks_fast()

    async def delete_task(self, id: int) -> None:
        await self._repo.delete_task(id)

    async def get_tasks_by_due_date(self, min_priority: int | None) -> list[TaskByDueDateResponse]:
        rows = await self._repo.get_tasks_by_due_date()
        # Urgency runs over the whole set: a hidden low-priority task can still head a chain.
        priority = urgency.chain_priorities(rows)
        await self._apply_urgency(rows, priority)
        urgency.apply_dependent_hours(rows)
        for row, p in zip(rows, priority, strict=True):
            row.effective_priority = p
        return [r for r in rows if min_priority is None or r.effective_priority <= min_priority]

    async def get_due_soon_slack(self) -> DueSoonSlackResponse:
        rows = await self._repo.get_tasks_by_due_date()
        minutes = await self._apply_urgency(rows, urgency.chain_priorities(rows))
        return DueSoonSlackResponse(slack_minutes=minutes)

    async def _apply_urgency(self, rows: list[TaskByDueDateResponse], priority: list[int]) -> int:
        """Fill the urgency fields and return today's slack in minutes: the free hours nothing
        claims, or, when work does not fit even starting today, minus the hours that overflow."""
        if self._capacity is None or self._plan is None:
            return 0
        now = self._clock()
        today = now.date()
        dues = [r.due_at or r.project_due_at for r in rows]
        dated = {r.id: i for i, r in enumerate(rows) if dues[i] is not None}
        estimated = [r.id for r in rows if r.id in dated and urgency.is_estimated(r)]
        # Nothing to schedule: none of today is claimed, so all that is left of it is free.
        last_due = max((d for d in dues if d is not None), default=today) if estimated else today

        free: dict[date, Decimal] = {}
        if last_due >= today:
            series = await self._capacity.free_busy_range(today, last_due + timedelta(days=1))
            for i, day in enumerate(series):
                free[day.date] = min(day.free_hours, urgency.day_cap(day.capacity_hours, i))
        if free.get(today, Decimal(0)) > 0:
            left = urgency.hours((self._midnight(today + timedelta(days=1)) - now).total_seconds())
            free[today] = max(min(free[today], left), Decimal(0))

        planned = await self._plan.planned_hours_by_task(estimated, now)
        rem = {
            i: urgency.remaining_hours(rows[i], planned.get(rows[i].id, Decimal(0)))
            for i in dated.values()
        }
        due_days = [d or date.min for d in dues]

        scheduled = [r.model_copy() for r in rows]
        left_free, overflow = urgency.backfill(
            scheduled, due_days, dated, rem, priority, free, today, False
        )
        if overflow > 0:
            # The week does not fit: rerun with the highest priority claiming first, so the
            # shortage lands on the lowest, but keep the first run's work order.
            work_order = [r.work_order for r in scheduled]
            scheduled = [r.model_copy() for r in rows]
            left_free, overflow = urgency.backfill(
                scheduled, due_days, dated, rem, priority, free, today, True
            )
            for row, order in zip(scheduled, work_order, strict=True):
                row.work_order = order
        for i, row in enumerate(scheduled):
            rows[i] = row
        slack = max(left_free.get(today, Decimal(0)), Decimal(0)) - overflow
        return int((slack * 60).quantize(Decimal(1), ROUND_HALF_UP))

    # Todos

    async def create_todo(self, req: CreateTodoRequest) -> TodoResponse:
        return await self._repo.create_todo(req.task_id, req.name)

    async def update_todo(self, id: int, req: UpdateTodoRequest) -> TodoResponse:
        return await self._repo.update_todo(id, _set(req, "task_id", "name", "is_done"))

    async def delete_todo(self, id: int) -> None:
        await self._repo.delete_todo(id)

    # Time entries

    async def create_time_entry(self, req: CreateTimeEntryRequest) -> TimeEntryResponse:
        assert req.started_at is not None  # noqa: S101 - the router requires it
        return await self._repo.create_time_entry(
            req.task_id, req.started_at, req.finished_at, req.comment
        )

    async def update_time_entry(self, id: int, req: UpdateTimeEntryRequest) -> TimeEntryResponse:
        values = _set(req, "task_id", "started_at", "comment") | _sent(req, "finished_at")
        return await self._repo.update_time_entry(id, values)

    async def delete_time_entry(self, id: int) -> None:
        await self._repo.delete_time_entry(id)

    async def get_active_time_entry(self) -> ActiveTimeEntryResponse:
        return await self._repo.get_active_time_entry()

    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse:
        now = self._clock()
        today = now.date()
        week_start = today - timedelta(days=today.weekday())
        summary = await self._repo.get_time_entry_summary(
            self._midnight(today), self._midnight(week_start)
        )
        pace = budget.calc_pace(now, summary.week, summary.today)
        return summary.model_copy(
            update={
                "weekly_target_seconds": budget.WEEKLY_TASK_TARGET_SECONDS,
                "pace": pace,
                "daily_target_seconds": budget.daily_target_seconds(pace),
            }
        )

    async def get_time_entry_history(
        self, frequency: str, start_at: str, end_at: str
    ) -> history.Response:
        trunc = history.valid_frequency(frequency)
        start, end = history.parse_date_range(self._loc, frequency, start_at, end_at, self._now)
        data = await self._repo.get_time_entry_history(trunc, self._loc.key, start, end)
        return history.Response(start_at=start, end_at=end, data=data)

    async def get_time_entries_by_date_range(
        self, start: date, end: date | None
    ) -> list[TimeEntryWithTaskResponse]:
        last = end if end is not None else self._clock().date()
        return await self._repo.get_time_entries_by_date_range(
            self._midnight(start), self._midnight(last + timedelta(days=1))
        )
