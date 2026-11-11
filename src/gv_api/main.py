"""Composition root: builds every service and the app (design D4)."""

import logging
import signal
import threading
from collections.abc import AsyncGenerator, Iterator
from contextlib import asynccontextmanager, contextmanager, suppress
from dataclasses import dataclass, field
from typing import Any

import uvicorn
from fastapi import FastAPI
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api import (
    auth,
    backup,
    calendar,
    capacity,
    finance,
    habits,
    plan,
    rutas,
    tasks,
    uptime,
)
from gv_api.alerts import TelegramHandler
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.workers import Workers
from gv_api.db.engine import create_engine, ping
from gv_api.db.migrate import migrate

log = logging.getLogger("gv_api")

GRACEFUL_SHUTDOWN_SECONDS = 10


@dataclass
class Container:
    cfg: Config
    engine: AsyncEngine
    workers: Workers = field(default_factory=Workers)
    services: dict[type[Any], Any] = field(default_factory=dict)

    def provide(self, *instances: object) -> None:
        for instance in instances:
            self.services[type(instance)] = instance


def build(cfg: Config, alerts: TelegramHandler | None = None) -> FastAPI:
    container = Container(cfg=cfg, engine=create_engine(cfg.database_url))
    container.provide(auth.AuthService(cfg))
    container.provide(habits.HabitsService(habits.HabitsRepository(container.engine), cfg.location))
    task_service = tasks.TasksService(tasks.TasksRepository(container.engine), cfg.location)
    plan_service = plan.PlanService(
        plan.PlanRepository(container.engine), task_service, cfg.location
    )
    capacity_service = capacity.CapacityService(
        cfg.daily_capacity_hours, plan_service, cfg.location
    )
    # tasks and plan depend on each other, so urgency providers are wired with a setter.
    task_service.set_urgency_providers(capacity_service, plan_service)
    container.provide(task_service, plan_service, capacity_service)
    container.provide(
        calendar.CalendarService(calendar.CalendarRepository(container.engine), plan_service)
    )
    container.provide(
        finance.FinanceService(finance.FinanceRepository(container.engine), cfg.location)
    )
    container.provide(rutas.RutasService(rutas.RutasRepository(container.engine)))
    backup_service = backup.backup_service(cfg, container.engine)
    container.provide(backup_service)
    container.workers.add("backup", backup.BackupWorker(backup_service, cfg.backup_interval).run)
    uptime_repo = uptime.UptimeRepository(container.engine)
    container.provide(uptime.UptimeService(uptime_repo))
    if not cfg.on_failover:
        container.workers.add("heartbeat", uptime.Heartbeat(uptime_repo).run)
    if alerts is not None:
        container.workers.add("alerts", alerts.run)

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncGenerator[None]:
        try:
            await migrate(container.engine)
        except Exception as err:
            log.error("failed to run migrations", extra={"error": str(err)})
            await container.engine.dispose()
            raise
        container.workers.start()
        try:
            yield
        finally:
            log.info("shutting down server")
            await container.workers.stop()
            await container.engine.dispose()
            if alerts is not None:
                await alerts.flush_batch()

    app = create_app(
        cfg,
        ping=lambda: ping(container.engine),
        routers=[
            auth.router,
            habits.router,
            tasks.router,
            plan.router,
            capacity.router,
            calendar.router,
            finance.router,
            rutas.router,
            uptime.router,
            backup.router,
        ],
        lifespan=lifespan,
    )
    app.state.container = container
    return app


class _Server(uvicorn.Server):
    @contextmanager
    def capture_signals(self) -> Iterator[None]:
        # uvicorn re-raises the signal after shutdown; a SIGTERM shutdown must exit 0.
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


def run_server(app: FastAPI, cfg: Config) -> int:
    if cfg.on_failover:
        log.warning(
            "running on the backup server: printers and uptime answer 503",
            extra={"failover_side": cfg.failover_side},
        )
    server = _Server(
        uvicorn.Config(
            app,
            # Every interface, IPv4 and IPv6: healthchecks dial localhost (::1).
            host="",
            port=int(cfg.port),
            log_config=None,
            access_log=False,
            timeout_graceful_shutdown=GRACEFUL_SHUTDOWN_SECONDS,
            server_header=False,
        )
    )
    log.info("server starting", extra={"port": cfg.port})
    with suppress(SystemExit):  # uvicorn exits this way when the lifespan fails to start
        server.run()
    if not server.started:
        return 1
    log.info("server stopped")
    return 0
