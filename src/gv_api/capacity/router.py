from datetime import timedelta
from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.capacity.service import CapacityService
from gv_api.core.dates import parse_day
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, query

router = full_router()
Svc = Annotated[CapacityService, Depends(service(CapacityService))]

MAX_SPAN = timedelta(days=90)


@router.get("/capacity/free-busy")
async def get_free_busy(request: Request, svc: Svc) -> Response:
    try:
        first = parse_day(query(request, "from"))
    except ValueError:
        return error(400, "invalid or missing from")
    try:
        end = parse_day(query(request, "to"))
    except ValueError:
        return error(400, "invalid or missing to")
    if end <= first:
        return error(400, "to must be after from")
    if end - first > MAX_SPAN:
        return error(400, "range is longer than 90 days")
    try:
        days = await svc.free_busy_range(first, end)
    except Exception as err:
        return internal_error(err, "Failed to get free/busy range")
    return respond(200, {"from": first, "to": end, "days": days})
