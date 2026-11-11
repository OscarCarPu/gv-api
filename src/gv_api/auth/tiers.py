"""Route tiers (spec auth): every protected route is in a router built here."""

from typing import Annotated

from fastapi import APIRouter, Depends, params
from starlette.requests import Request

from gv_api.auth.service import AuthService
from gv_api.core.deps import service
from gv_api.core.http import ApiError
from gv_api.core.routing import tier_router


def _require(*kinds: str) -> params.Depends:
    async def check(
        request: Request, svc: Annotated[AuthService, Depends(service(AuthService))]
    ) -> None:
        header = request.headers.get("authorization", "")
        token = header.removeprefix("Bearer ")
        if token == header or svc.token_kind(token) not in kinds:
            raise ApiError(401, "Unauthorized")

    return params.Depends(check)


def full_router() -> APIRouter:
    return tier_router("full", [_require("full")])


def semi_router(*extra: params.Depends) -> APIRouter:
    """rutas, printers and uptime: a `semi` or a `full` token."""
    return tier_router("semi", [_require("semi", "full"), *extra])
