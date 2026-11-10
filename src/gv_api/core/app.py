"""The FastAPI app and its routing conventions (design D9)."""

import asyncio
import logging
from collections.abc import Awaitable, Callable, Sequence
from contextlib import AbstractAsyncContextManager
from typing import Any

from fastapi import APIRouter, FastAPI
from starlette.middleware.cors import CORSMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Match

from gv_api.config import Config
from gv_api.core import cors
from gv_api.core.http import ApiError, error, error_code
from gv_api.core.middleware import RecoverMiddleware, RequestIDMiddleware
from gv_api.core.routing import tier_router

log = logging.getLogger("gv_api")

Ping = Callable[[], Awaitable[object]]
Lifespan = Callable[[FastAPI], AbstractAsyncContextManager[None]]


_METHODS = ("DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT")


async def method_not_allowed(request: Request, exc: Exception) -> Response:
    """405 listing every method the path supports; Starlette's lists only the first route's."""
    allowed = [
        method
        for method in _METHODS
        if any(
            route.matches({**request.scope, "method": method})[0] is Match.FULL
            for route in request.app.router.routes
        )
    ]
    return JSONResponse(
        {"detail": "Method Not Allowed"}, status_code=405, headers={"Allow": ", ".join(allowed)}
    )


async def api_error(_: Request, exc: Exception) -> Response:
    if not isinstance(exc, ApiError):
        return Response(status_code=500)
    if exc.code is not None:
        return error_code(exc.status, exc.code, exc.message)
    return error(exc.status, exc.message)


def health_router(ping: Ping, timeout: float) -> APIRouter:
    router = tier_router("public")

    @router.get("/health")
    async def health() -> Response:
        try:
            async with asyncio.timeout(timeout):
                await ping()
        except Exception as err:
            log.error("health check: database unreachable", extra={"error": str(err)})
            return Response(status_code=503)
        return Response(status_code=200)

    return router


def create_app(
    cfg: Config,
    *,
    ping: Ping,
    routers: Sequence[APIRouter] = (),
    lifespan: Lifespan | None = None,
    middleware: Sequence[tuple[type[Any], dict[str, Any]]] = (),
) -> FastAPI:
    """`middleware` sits between CORS and the routes, outermost first (the failover lock)."""
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
    app.router.redirect_slashes = False
    app.add_exception_handler(405, method_not_allowed)
    app.add_exception_handler(ApiError, api_error)

    app.include_router(health_router(ping, cfg.health_timeout.total_seconds()))
    for router in routers:
        app.include_router(router)

    # add_middleware wraps: the last added is the outermost.
    app.add_middleware(RecoverMiddleware)
    for cls, options in reversed(middleware):
        app.add_middleware(cls, **options)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=cfg.allowed_origins,
        allow_methods=cors.METHODS,
        allow_headers=cors.BROWSER_HEADERS,
        expose_headers=cors.EXPOSED,
        max_age=cors.MAX_AGE,
    )
    app.add_middleware(RequestIDMiddleware)
    return app
