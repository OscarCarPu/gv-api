from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth import full_router
from gv_api.core.dates import parse_day
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error, parse_body, parse_id, query
from gv_api.plan.errors import (
    DaysOfWeekRequiredError,
    InvalidTimeRangeError,
    LabelRequiredError,
    LabelTooLongError,
    NotFoundError,
    OverlapError,
    TaskNotFoundError,
)
from gv_api.plan.schemas import (
    CreateCommitmentRequest,
    CreatePlanBlockRequest,
    UpdateCommitmentRequest,
    UpdatePlanBlockRequest,
)
from gv_api.plan.service import PlanService

router = full_router()
Svc = Annotated[PlanService, Depends(service(PlanService))]

_BLOCK_ERRORS = (InvalidTimeRangeError, LabelRequiredError, LabelTooLongError, OverlapError)
_COMMITMENT_ERRORS = (LabelRequiredError, LabelTooLongError, DaysOfWeekRequiredError)


@router.get("/plan/today")
async def get_today(svc: Svc) -> Response:
    try:
        return respond(200, await svc.get_today())
    except Exception as err:
        return internal_error(err, "Failed to get today's plan")


@router.get("/plan/range")
async def get_range(request: Request, svc: Svc) -> Response:
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
    try:
        return respond(200, await svc.get_range(first, end))
    except Exception as err:
        return internal_error(err, "Failed to get plan range")


@router.post("/plan/blocks")
async def create_block(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreatePlanBlockRequest, "Invalid Body")
    try:
        return respond(201, await svc.create(req))
    except (*_BLOCK_ERRORS, TaskNotFoundError) as err:
        return error(400, err.message)
    except Exception as err:
        return internal_error(err, "Failed to create plan block")


@router.delete("/plan/blocks/future")
async def delete_future(svc: Svc) -> Response:
    try:
        await svc.delete_future()
    except Exception as err:
        return internal_error(err, "Failed to delete future plan blocks")
    return Response(status_code=204)


@router.put("/plan/blocks/{id}")
async def update_block(id: str, request: Request, svc: Svc) -> Response:
    block_id = parse_id(id, "plan block")
    req = await parse_body(request, UpdatePlanBlockRequest, "Invalid Body")
    try:
        return respond(200, await svc.update(block_id, req))
    except NotFoundError:
        return error(404, "plan block not found")
    except _BLOCK_ERRORS as err:
        return error(400, err.message)
    except Exception as err:
        return internal_error(err, "Failed to update plan block")


@router.delete("/plan/blocks/{id}")
async def delete_block(id: str, svc: Svc) -> Response:
    block_id = parse_id(id, "plan block")
    try:
        await svc.delete(block_id)
    except Exception as err:
        return internal_error(err, "Failed to delete plan block")
    return Response(status_code=204)


@router.get("/plan/commitments")
async def list_commitments(svc: Svc) -> Response:
    try:
        return respond(200, await svc.list_commitments())
    except Exception as err:
        return internal_error(err, "Failed to list commitments")


@router.post("/plan/commitments")
async def create_commitment(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, CreateCommitmentRequest, "Invalid Body")
    try:
        return respond(201, await svc.create_commitment(req))
    except (*_COMMITMENT_ERRORS, TaskNotFoundError) as err:
        return error(400, err.message)
    except Exception as err:
        return internal_error(err, "Failed to create commitment")


@router.put("/plan/commitments/{id}")
async def update_commitment(id: str, request: Request, svc: Svc) -> Response:
    commitment_id = parse_id(id, "commitment")
    req = await parse_body(request, UpdateCommitmentRequest, "Invalid Body")
    try:
        return respond(200, await svc.update_commitment(commitment_id, req))
    except NotFoundError:
        return error(404, "commitment not found")
    except _COMMITMENT_ERRORS as err:
        return error(400, err.message)
    except Exception as err:
        return internal_error(err, "Failed to update commitment")


@router.delete("/plan/commitments/{id}")
async def delete_commitment(id: str, svc: Svc) -> Response:
    commitment_id = parse_id(id, "commitment")
    try:
        await svc.delete_commitment(commitment_id)
    except Exception as err:
        return internal_error(err, "Failed to delete commitment")
    return Response(status_code=204)
