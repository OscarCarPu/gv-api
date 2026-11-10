from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import semi_router
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, parse_body, parse_id
from gv_api.rutas.errors import NotFoundError
from gv_api.rutas.schemas import CreateMarkRequest, UpdateMarkRequest
from gv_api.rutas.service import RutasService, parse_date

router = semi_router()
Svc = Annotated[RutasService, Depends(service(RutasService))]


def _date_error(visited_on: str) -> str:
    if not visited_on:
        return "visited_on is required"
    try:
        parse_date(visited_on)
    except ValueError as err:
        return str(err)
    return ""


@router.get("/rutas/marks")
async def list_marks(svc: Svc) -> Response:
    try:
        marks = await svc.list()
    except Exception as err:
        return internal_error(err, "Failed to list marks")
    return respond(200, marks)


@router.get("/rutas/marks/{id}")
async def get_mark(id: str, svc: Svc) -> Response:
    mark_id = parse_id(id, "mark")
    try:
        mark = await svc.get(mark_id)
    except NotFoundError:
        return error(404, "mark not found")
    except Exception as err:
        return internal_error(err, "Failed to get mark")
    return respond(200, mark)


@router.post("/rutas/marks")
async def create_mark(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateMarkRequest, "invalid body")
    req.name = req.name.strip()
    if not req.name:
        return error(400, "name is required")
    if len(req.name.encode()) > 200:
        return error(400, "name too long")
    if msg := _date_error(req.visited_on):
        return error(400, msg)
    try:
        mark = await svc.create(req)
    except Exception as err:
        return internal_error(err, "Failed to create mark")
    return respond(201, mark)


@router.put("/rutas/marks/{id}")
async def update_mark(id: str, request: Request, svc: Svc) -> Response:
    mark_id = parse_id(id, "mark")
    req = await parse_body(request, UpdateMarkRequest, "invalid body")
    if msg := _date_error(req.visited_on):
        return error(400, msg)
    try:
        mark = await svc.update(mark_id, req)
    except NotFoundError:
        return error(404, "mark not found")
    except Exception as err:
        return internal_error(err, "Failed to update mark")
    return respond(200, mark)


@router.delete("/rutas/marks/{id}")
async def delete_mark(id: str, svc: Svc) -> Response:
    mark_id = parse_id(id, "mark")
    try:
        await svc.delete(mark_id)
    except Exception as err:
        return internal_error(err, "Failed to delete mark")
    return Response(status_code=204)
