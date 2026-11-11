import re
from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import semi_router
from gv_api.core.dates import parse_instant
from gv_api.core.deps import home_only, service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, query
from gv_api.uptime.errors import InvalidRangeError
from gv_api.uptime.service import UptimeService

router = semi_router(home_only)
Svc = Annotated[UptimeService, Depends(service(UptimeService))]

_DIGITS = re.compile(r"\d+")


@router.get("/domotics/uptime")
async def overview(svc: Svc) -> Response:
    try:
        return respond(200, await svc.overview())
    except Exception as err:
        return internal_error(err, "Failed to read uptime")


@router.get("/domotics/uptime/windows")
async def windows(request: Request, svc: Svc) -> Response:
    bounds = []
    for name in ("from", "to"):
        raw = query(request, name)
        try:
            bounds.append(parse_instant(raw) if raw else None)
        except ValueError:
            return error(400, f"invalid {name}: must be RFC 3339 or YYYY-MM-DD")
    limit = None
    if raw := query(request, "limit"):
        if not _DIGITS.fullmatch(raw) or int(raw) == 0:
            return error(400, "limit must be a positive integer")
        limit = int(raw)
    try:
        return respond(200, await svc.windows(bounds[0], bounds[1], limit))
    except InvalidRangeError:
        return error(400, "from must be before to")
    except Exception as err:
        return internal_error(err, "Failed to read uptime windows")
