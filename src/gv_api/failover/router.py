import hmac
import logging
from datetime import UTC, datetime, timedelta
from typing import Annotated

from fastapi import Depends, params
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import ApiError, Body, InTime, error, internal_error, parse_body
from gv_api.core.routing import tier_router
from gv_api.failover.controller import (
    FAILING_BACK,
    HOME_READY,
    MODE_HOLD,
    NORMAL,
    PENDING_MERGE,
    FailoverController,
)

log = logging.getLogger("gv_api")

Ctl = Annotated[FailoverController, Depends(service(FailoverController))]


async def _token(request: Request, ctl: Ctl) -> None:
    got = request.headers.get("x-failover-token", "").encode()
    if not ctl.token or not hmac.compare_digest(got, ctl.token.encode()):
        raise ApiError(401, "invalid failover token")


# What the AWS side calls: the failover token, not a user login.
router = tier_router("failover-token", [params.Depends(_token)])
# The report screen, behind the normal login.
report_router = full_router()


class FailoverBody(Body):
    mode: str = ""


class OutageBody(Body):
    ec2_up_at: InTime | None = None
    home_at: InTime | None = None


def _status(ctl: FailoverController) -> dict[str, str | None]:
    return {"mode": ctl.mode, "merge": ctl.merge, "error": ctl.error or None}


async def _state(ctl: FailoverController) -> str:
    try:
        return await ctl.state()
    except Exception as err:
        log.error("cannot read the failover state", extra={"error": str(err)})
        raise ApiError(500, "cannot read the failover state") from err


@router.post("/failover")
async def failover(request: Request, ctl: Ctl) -> Response:
    body = await parse_body(request, FailoverBody, "invalid body")
    state = await _state(ctl)
    if body.mode == "hold":
        if state == NORMAL:
            return error(409, "the failover state is NORMAL")
        ctl.set_mode(MODE_HOLD)
    elif body.mode == "merge":
        if state not in (FAILING_BACK, PENDING_MERGE):
            return error(409, f"the failover state is {state}")
        if not await ctl.has_dump():
            return error(409, "no failback dump yet")
        ctl.start_merge()
    else:
        return error(400, "mode must be hold or merge")
    return respond(200, _status(ctl))


@router.get("/failover/status")
async def status(ctl: Ctl) -> Response:
    return respond(200, _status(ctl))


@router.post("/failover/outage")
async def outage(request: Request, ctl: Ctl) -> Response:
    body = await parse_body(request, OutageBody, "invalid body")
    ec2_up, home = body.ec2_up_at, body.home_at
    if (
        ec2_up is None
        or home is None
        or ec2_up >= home
        or home > datetime.now(UTC) + timedelta(minutes=1)
    ):
        return error(400, "invalid body")
    state = await _state(ctl)
    if state != HOME_READY:
        return error(409, f"the failover state is {state}")
    if ctl.outages is None:
        return error(503, "uptime is not available")
    try:
        await ctl.outages.record_failover(ctl.last_alive, ec2_up, home)
    except Exception as err:
        return internal_error(err, "cannot record the failover")
    ctl.last_alive = None
    return Response(status_code=200)


@report_router.get("/report")
async def get_report(ctl: Ctl) -> Response:
    return respond(200, _status(ctl) | {"report": ctl.report})


@report_router.delete("/report")
async def dismiss_report(ctl: Ctl) -> Response:
    ctl.dismiss_report()
    return Response(status_code=204)
