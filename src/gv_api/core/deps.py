from collections.abc import Awaitable, Callable
from functools import cache
from typing import Any, cast

from starlette.requests import Request


@cache
def service[T](cls: type[T]) -> Callable[[Request], Awaitable[T]]:
    """Dependency returning the composition root's instance of `cls` (design D4)."""

    # async so FastAPI calls it inline instead of in the threadpool
    async def get(request: Request) -> T:
        services: dict[type[Any], Any] = request.app.state.container.services
        return cast("T", services[cls])

    return get
