from collections.abc import Awaitable, Callable
from functools import cache
from typing import Any, cast

from fastapi import params
from starlette.requests import Request

from gv_api.core.http import ApiError


@cache
def service[T](cls: type[T]) -> Callable[[Request], Awaitable[T]]:
    """Dependency returning the composition root's instance of `cls`."""

    # async so FastAPI calls it inline instead of in the threadpool
    async def get(request: Request) -> T:
        services: dict[type[Any], Any] = request.app.state.container.services
        return cast("T", services[cls])

    return get


ON_FAILOVER = "Not available while running on the backup server. Back when home is restored."


async def _home_only(request: Request) -> None:
    cfg = getattr(request.app.state.container, "cfg", None)
    if cfg is not None and cfg.on_failover:
        raise ApiError(503, ON_FAILOVER, "unavailable_on_failover")


# Printers and uptime need the home network: on the AWS side they answer 503.
home_only = params.Depends(_home_only)
