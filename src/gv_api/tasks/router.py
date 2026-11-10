from collections.abc import Awaitable
from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.core.dates import parse_day
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, parse_body, parse_id, query
from gv_api.history.periods import FREQUENCIES
from gv_api.tasks.errors import (
    ActiveTimeEntryExistsError,
    NotFoundError,
    ParentNotFoundError,
    ProjectCycleError,
)
from gv_api.tasks.schemas import (
    CreateProjectRequest,
    CreateTaskRequest,
    CreateTimeEntryRequest,
    CreateTodoRequest,
    UpdateProjectRequest,
    UpdateTaskRequest,
    UpdateTimeEntryRequest,
    UpdateTodoRequest,
)
from gv_api.tasks.service import TasksService
from gv_api.tasks.validation import (
    validate_name,
    validate_optional_name,
    validate_priority,
    validate_recurrence_value,
    validate_task_type,
)

router = full_router()
Svc = Annotated[TasksService, Depends(service(TasksService))]


def _first(*messages: str) -> str:
    return next((m for m in messages if m), "")


def _min_priority(request: Request) -> int | None:
    raw = query(request, "min_priority")
    if not raw:
        return None
    try:
        value = int(raw)
    except ValueError:
        value = 0
    if not 1 <= value <= 5:
        raise ValueError("min_priority must be between 1 and 5")
    return value


async def _get(
    call: Awaitable[object], failure: str, not_found: str | None = None, status: int = 200
) -> Response:
    """Await a service call and answer it, mapping a missing row to `not_found`."""
    try:
        result = await call
    except NotFoundError as err:
        if not_found is None:
            return internal_error(err, failure)
        return error(404, not_found)
    except Exception as err:
        return internal_error(err, failure)
    return respond(status, result)


async def _delete(call: Awaitable[None], failure: str, not_found: str) -> Response:
    try:
        await call
    except NotFoundError:
        return error(404, not_found)
    except Exception as err:
        return internal_error(err, failure)
    return Response(status_code=204)


# Tree and projects


@router.get("/tasks/tree")
async def get_active_tree(request: Request, svc: Svc) -> Response:
    try:
        min_priority = _min_priority(request)
    except ValueError as err:
        return error(400, str(err))
    return await _get(svc.get_active_tree(min_priority), "Failed to get active tree")


@router.get("/tasks/projects")
async def get_root_projects(svc: Svc) -> Response:
    return await _get(svc.get_root_projects(), "Failed to get projects")


@router.get("/tasks/projects/list-fast")
async def list_projects_fast(svc: Svc) -> Response:
    return await _get(svc.list_projects_fast(), "Failed to list projects")


@router.get("/tasks/projects/{id}")
async def get_project(id: str, svc: Svc) -> Response:
    project_id = parse_id(id, "project")
    return await _get(svc.get_project(project_id), "Failed to get project", "project not found")


@router.get("/tasks/projects/{id}/children")
async def get_project_children(id: str, svc: Svc) -> Response:
    project_id = parse_id(id, "project")
    return await _get(
        svc.get_project_children(project_id),
        "Failed to get project children",
        "project not found",
    )


@router.get("/tasks/projects/{id}/parent-candidates")
async def list_parent_candidates(id: str, svc: Svc) -> Response:
    project_id = parse_id(id, "project")
    return await _get(
        svc.list_project_parent_candidates(project_id),
        "Failed to list parent candidates",
        "project not found",
    )


@router.post("/tasks/projects")
async def create_project(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateProjectRequest, "Invalid Body")
    if msg := _first(validate_name(req.name), validate_priority(req.priority)):
        return error(400, msg)
    return await _get(svc.create_project(req), "Failed to create project", status=201)


@router.patch("/tasks/projects/{id}")
async def update_project(id: str, request: Request, svc: Svc) -> Response:
    project_id = parse_id(id, "project")
    req = await parse_body(request, UpdateProjectRequest, "Invalid Body")
    if msg := _first(validate_optional_name(req.name), validate_priority(req.priority)):
        return error(400, msg)
    try:
        project = await svc.update_project(project_id, req)
    except NotFoundError:
        return error(404, "project not found")
    except ProjectCycleError as err:
        return error(409, err.message)
    except ParentNotFoundError as err:
        return error(400, err.message)
    except Exception as err:
        return internal_error(err, "Failed to update project")
    return respond(200, project)


@router.delete("/tasks/projects/{id}")
async def delete_project(id: str, svc: Svc) -> Response:
    project_id = parse_id(id, "project")
    return await _delete(
        svc.delete_project(project_id), "Failed to delete project", "project not found"
    )


# Tasks


@router.get("/tasks/tasks/by-due-date")
async def get_tasks_by_due_date(request: Request, svc: Svc) -> Response:
    try:
        min_priority = _min_priority(request)
    except ValueError as err:
        return error(400, str(err))
    return await _get(svc.get_tasks_by_due_date(min_priority), "Failed to get tasks by due date")


@router.get("/tasks/tasks/by-due-date/slack")
async def get_due_soon_slack(svc: Svc) -> Response:
    return await _get(svc.get_due_soon_slack(), "Failed to get due-soon slack")


@router.get("/tasks/tasks/list-fast")
async def list_tasks_fast(svc: Svc) -> Response:
    return await _get(svc.list_tasks_fast(), "Failed to list tasks")


@router.get("/tasks/tasks/{id}")
async def get_task(id: str, svc: Svc) -> Response:
    task_id = parse_id(id, "task")
    return await _get(svc.get_task(task_id), "Failed to get task", "task not found")


@router.get("/tasks/tasks/{id}/time-entries")
async def get_task_time_entries(id: str, svc: Svc) -> Response:
    task_id = parse_id(id, "task")
    return await _get(
        svc.get_task_time_entries(task_id), "Failed to get task time entries", "task not found"
    )


def _recurrence_error(task_type: str | None, recurrence: int | None) -> str:
    if task_type == "recurring" and recurrence is None:
        return "recurrence is required when task_type is recurring"
    if task_type != "recurring" and recurrence is not None:
        return "recurrence is only valid when task_type is recurring"
    return validate_recurrence_value(recurrence)


@router.post("/tasks/tasks")
async def create_task(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateTaskRequest, "Invalid Body")
    if msg := _first(
        validate_name(req.name),
        validate_task_type(req.task_type),
        _recurrence_error(req.task_type, req.recurrence),
        validate_priority(req.priority),
    ):
        return error(400, msg)
    return await _get(svc.create_task(req), "Failed to create task", status=201)


@router.patch("/tasks/tasks/{id}")
async def update_task(id: str, request: Request, svc: Svc) -> Response:
    task_id = parse_id(id, "task")
    req = await parse_body(request, UpdateTaskRequest, "Invalid Body")
    if msg := _first(
        validate_optional_name(req.name),
        validate_task_type(req.task_type),
        _recurrence_error(req.task_type, req.recurrence) if req.task_type is not None else "",
        validate_recurrence_value(req.recurrence),
        validate_priority(req.priority),
    ):
        return error(400, msg)
    return await _get(svc.update_task(task_id, req), "Failed to update task", "task not found")


@router.delete("/tasks/tasks/{id}")
async def delete_task(id: str, svc: Svc) -> Response:
    task_id = parse_id(id, "task")
    return await _delete(svc.delete_task(task_id), "Failed to delete task", "task not found")


# Todos


@router.post("/tasks/todos")
async def create_todo(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateTodoRequest, "Invalid Body")
    if req.task_id == 0:
        return error(400, "task_id is required")
    if msg := validate_name(req.name):
        return error(400, msg)
    return await _get(svc.create_todo(req), "Failed to create todo", status=201)


@router.patch("/tasks/todos/{id}")
async def update_todo(id: str, request: Request, svc: Svc) -> Response:
    todo_id = parse_id(id, "todo")
    req = await parse_body(request, UpdateTodoRequest, "Invalid Body")
    if msg := validate_optional_name(req.name):
        return error(400, msg)
    return await _get(svc.update_todo(todo_id, req), "Failed to update todo", "todo not found")


@router.delete("/tasks/todos/{id}")
async def delete_todo(id: str, svc: Svc) -> Response:
    todo_id = parse_id(id, "todo")
    return await _delete(svc.delete_todo(todo_id), "Failed to delete todo", "todo not found")


# Time entries


@router.get("/tasks/time-entries")
async def get_time_entries_by_date_range(request: Request, svc: Svc) -> Response:
    raw_start = query(request, "start_time")
    if not raw_start:
        return error(400, "start_time is required")
    try:
        start = parse_day(raw_start)
    except ValueError:
        return error(400, "invalid start_time format, expected YYYY-MM-DD")
    end = None
    if raw_end := query(request, "end_time"):
        try:
            end = parse_day(raw_end)
        except ValueError:
            return error(400, "invalid end_time format, expected YYYY-MM-DD")
    return await _get(svc.get_time_entries_by_date_range(start, end), "Failed to get time entries")


@router.get("/tasks/time-entries/active")
async def get_active_time_entry(svc: Svc) -> Response:
    return await _get(
        svc.get_active_time_entry(), "Failed to get active time entry", "no active time entry"
    )


@router.get("/tasks/time-entries/summary")
async def get_time_entry_summary(svc: Svc) -> Response:
    return await _get(svc.get_time_entry_summary(), "Failed to get time entry summary")


@router.get("/tasks/time-entries/history")
async def get_time_entry_history(request: Request, svc: Svc) -> Response:
    frequency = query(request, "frequency")
    if not frequency:
        return error(400, "frequency is required")
    if frequency not in FREQUENCIES:
        return error(400, "frequency must be daily, weekly, or monthly")
    return await _get(
        svc.get_time_entry_history(frequency, query(request, "start_at"), query(request, "end_at")),
        "Failed to get time entry history",
    )


@router.post("/tasks/time-entries")
async def create_time_entry(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateTimeEntryRequest, "Invalid Body")
    if req.task_id == 0:
        return error(400, "task_id is required")
    if req.started_at is None:
        return error(400, "started_at is required")
    try:
        entry = await svc.create_time_entry(req)
    except ActiveTimeEntryExistsError as err:
        return error(409, err.message)
    except Exception as err:
        return internal_error(err, "Failed to create time entry")
    return respond(201, entry)


@router.patch("/tasks/time-entries/{id}")
async def update_time_entry(id: str, request: Request, svc: Svc) -> Response:
    entry_id = parse_id(id, "time entry")
    req = await parse_body(request, UpdateTimeEntryRequest, "Invalid Body")
    return await _get(
        svc.update_time_entry(entry_id, req),
        "Failed to update time entry",
        "time entry not found",
    )


@router.delete("/tasks/time-entries/{id}")
async def delete_time_entry(id: str, svc: Svc) -> Response:
    entry_id = parse_id(id, "time entry")
    return await _delete(
        svc.delete_time_entry(entry_id), "Failed to delete time entry", "time entry not found"
    )
