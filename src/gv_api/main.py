"""Composition root: builds every service and the app, like cmd/api/main.go (design D4)."""

import logging
import signal
import threading
from collections.abc import AsyncGenerator, Iterator
from contextlib import asynccontextmanager, contextmanager
from dataclasses import dataclass, field

import uvicorn
from fastapi import FastAPI
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.workers import Workers
from gv_api.db.engine import create_engine, ping

log = logging.getLogger("gv_api")

GRACEFUL_SHUTDOWN_SECONDS = 10


@dataclass
class Container:
    cfg: Config
    engine: AsyncEngine
    workers: Workers = field(default_factory=Workers)


def build(cfg: Config) -> FastAPI:
    container = Container(cfg=cfg, engine=create_engine(cfg.database_url))

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncGenerator[None]:
        container.workers.start()
        try:
            yield
        finally:
            log.info("shutting down server")
            await container.workers.stop()
            await container.engine.dispose()

    app = create_app(cfg, ping=lambda: ping(container.engine), lifespan=lifespan)
    app.state.container = container
    return app


class _Server(uvicorn.Server):
    @contextmanager
    def capture_signals(self) -> Iterator[None]:
        # uvicorn re-raises the signal after shutdown; the Go service exited 0 on SIGTERM.
        if threading.current_thread() is not threading.main_thread():
            yield
            return
        original = {
            sig: signal.signal(sig, self.handle_exit) for sig in (signal.SIGINT, signal.SIGTERM)
        }
        try:
            yield
        finally:
            for sig, handler in original.items():
                signal.signal(sig, handler)


def run_server(app: FastAPI, cfg: Config) -> None:
    if cfg.on_failover:
        log.warning(
            "running on the backup server: printers and uptime answer 503",
            extra={"failover_side": cfg.failover_side},
        )
    server = _Server(
        uvicorn.Config(
            app,
            host="0.0.0.0",  # noqa: S104 (container port)
            port=int(cfg.port),
            log_config=None,
            access_log=False,
            timeout_graceful_shutdown=GRACEFUL_SHUTDOWN_SECONDS,
            server_header=False,
        )
    )
    log.info("server starting", extra={"port": cfg.port})
    server.run()
    log.info("server stopped")
