import asyncio
import logging
from collections.abc import Callable, Coroutine
from typing import Any

log = logging.getLogger("gv_api")

Job = Callable[[], Coroutine[Any, Any, None]]


class Workers:
    """Background jobs owned by the lifespan: started after startup, cancelled on shutdown."""

    def __init__(self) -> None:
        self._jobs: list[tuple[str, Job]] = []
        self._tasks: list[asyncio.Task[None]] = []

    def add(self, name: str, job: Job) -> None:
        self._jobs.append((name, job))

    def start(self) -> None:
        for name, job in self._jobs:
            self._tasks.append(asyncio.create_task(self._run(name, job), name=name))

    @staticmethod
    async def _run(name: str, job: Job) -> None:
        try:
            await job()
        except asyncio.CancelledError:
            raise
        except Exception:
            log.exception("worker stopped with an error", extra={"worker": name})

    async def stop(self) -> None:
        for task in self._tasks:
            task.cancel()
        await asyncio.gather(*self._tasks, return_exceptions=True)
        self._tasks.clear()
