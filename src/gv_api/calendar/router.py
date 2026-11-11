from collections.abc import Awaitable
from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.calendar.errors import InvalidError, NotFoundError
from gv_api.calendar.schemas import CreateEventRequest, UpdateEventRequest
from gv_api.calendar.service import CalendarService
from gv_api.core.dates import parse_instant
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, parse_body, parse_id, query

router = full_router()
Svc = Annotated[CalendarService, Depends(service(CalendarService))]


async def _answer(call: Awaitable[object], failure: str, status: int = 200) -> Response:
    try:
        result = await call
    except NotFoundError:
        return error(404, "not found")
    except InvalidError as err:
        return error(400, str(err))
    except Exception as err:
        return internal_error(err, failure)
    if result is None:
        return Response(status_code=204)
    return respond(status, result)


@router.get("/calendar/events")
async def list_events(request: Request, svc: Svc) -> Response:
    try:
        start = parse_instant(query(request, "from"))
    except ValueError:
        return error(400, "from is required (RFC3339 or YYYY-MM-DD)")
    try:
        end = parse_instant(query(request, "to"))
    except ValueError:
        return error(400, "to is required (RFC3339 or YYYY-MM-DD)")
    return await _answer(svc.list_events(start, end), "Failed to list events")


@router.post("/calendar/events")
async def create_event(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateEventRequest, "invalid body")
    return await _answer(svc.create_event(req), "Failed to create event", 201)


@router.get("/calendar/events/{id}")
async def get_event(id: str, svc: Svc) -> Response:
    return await _answer(svc.get_event(parse_id(id, "event")), "Failed to get event")


@router.patch("/calendar/events/{id}")
async def update_event(id: str, request: Request, svc: Svc) -> Response:
    event_id = parse_id(id, "event")
    req = await parse_body(request, UpdateEventRequest, "invalid body")
    return await _answer(svc.update_event(event_id, req), "Failed to update event")


@router.delete("/calendar/events/{id}")
async def delete_event(id: str, svc: Svc) -> Response:
    return await _answer(svc.delete_event(parse_id(id, "event")), "Failed to delete event")
