from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, parse_body, parse_id, query
from gv_api.habits.errors import NotFoundError
from gv_api.habits.schemas import CreateHabitRequest, LogUpsertRequest, UpdateHabitRequest
from gv_api.habits.service import FREQUENCIES, HabitsService

router = full_router()
Svc = Annotated[HabitsService, Depends(service(HabitsService))]

_BAD_FREQUENCY = "frequency must be daily, weekly, or monthly"


def validate_habit_fields(
    name: str, frequency: str | None, target_min: float | None, target_max: float | None
) -> str:
    """The checks shared by create and update; a frequency of None is not checked."""
    if not name:
        return "name is required"
    if len(name) > 40:
        return "name must be at most 40 characters"
    if frequency is not None and frequency not in FREQUENCIES:
        return _BAD_FREQUENCY
    if target_min is not None and target_min < 0:
        return "target_min must be >= 0"
    if target_max is not None and target_max < 0:
        return "target_max must be >= 0"
    if target_min is not None and target_max is not None and target_min > target_max:
        return "target_min must be <= target_max"
    return ""


@router.get("/habits")
async def get_daily(request: Request, svc: Svc) -> Response:
    try:
        habits = await svc.get_daily_view(query(request, "date"))
    except Exception as err:
        return internal_error(err, "Failed to get daily habits")
    return respond(200, habits or None)  # an empty view is null, not []


@router.post("/habits")
async def create_habit(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateHabitRequest, "Invalid Body")
    if msg := validate_habit_fields(req.name, req.frequency, req.target_min, req.target_max):
        return error(400, msg)
    try:
        habit = await svc.create_habit(req)
    except Exception as err:
        return internal_error(err, "Failed to create habit")
    return respond(201, habit)


@router.post("/habits/log")
async def upsert_log(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, LogUpsertRequest, "Invalid Body")
    try:
        await svc.log_habit(req)
    except Exception as err:
        return internal_error(err, "Failed to log habit")
    return respond(200, {"status": "ok"})


@router.put("/habits/{id}")
async def update_habit(id: str, request: Request, svc: Svc) -> Response:
    habit_id = parse_id(id, "habit")
    req = await parse_body(request, UpdateHabitRequest, "Invalid Body")
    if msg := validate_habit_fields(req.name, req.frequency, req.target_min, req.target_max):
        return error(400, msg)
    try:
        habit = await svc.update_habit(habit_id, req)
    except NotFoundError:
        return error(404, "habit not found")
    except Exception as err:
        return internal_error(err, "Failed to update habit")
    return respond(200, habit)


@router.delete("/habits/{id}")
async def delete_habit(id: str, svc: Svc) -> Response:
    habit_id = parse_id(id, "habit")
    try:
        await svc.delete_habit(habit_id)
    except Exception as err:
        return internal_error(err, "Failed to delete habit")
    return Response(status_code=204)


@router.get("/habits/{id}/history")
async def get_history(id: str, request: Request, svc: Svc) -> Response:
    habit_id = parse_id(id, "habit")
    frequency = query(request, "frequency")
    if frequency and frequency not in FREQUENCIES:
        return error(400, _BAD_FREQUENCY)
    try:
        data = await svc.get_history(
            habit_id, frequency, query(request, "start_at"), query(request, "end_at")
        )
    except Exception as err:
        return internal_error(err, "Failed to get habit history")
    return respond(200, data)
