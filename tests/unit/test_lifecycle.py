import asyncio
import json
import logging
import os
import signal
import socket
import subprocess
import sys
import textwrap
import threading
import time

import httpx
import pytest

from gv_api.cli import main
from gv_api.config import Config
from gv_api.core.workers import Workers
from gv_api.main import build


async def test_workers_start_and_are_cancelled_on_stop() -> None:
    ticks: list[int] = []
    cancelled = asyncio.Event()

    async def job() -> None:
        try:
            while True:
                ticks.append(1)
                await asyncio.sleep(0.01)
        except asyncio.CancelledError:
            cancelled.set()
            raise

    workers = Workers()
    workers.add("ticker", job)
    workers.start()
    await asyncio.sleep(0.05)
    await workers.stop()
    assert ticks
    assert cancelled.is_set()


async def test_failing_worker_is_logged_not_fatal(caplog: pytest.LogCaptureFixture) -> None:
    async def broken() -> None:
        raise RuntimeError("nope")

    workers = Workers()
    workers.add("broken", broken)
    with caplog.at_level(logging.ERROR):
        workers.start()
        await asyncio.sleep(0.01)
        await workers.stop()
    assert caplog.records[0].__dict__["worker"] == "broken"


async def test_lifespan_runs_and_stops_workers(monkeypatch: pytest.MonkeyPatch) -> None:
    async def no_migrations(*_: object) -> None:
        return None

    monkeypatch.setattr("gv_api.main.migrate", no_migrations)
    cfg = Config(
        database_url="postgresql://u:p@127.0.0.1:1/db?sslmode=disable",
        password="p",
        semiprivate_password="s",
        jwt_secret="j",
        totp_secret="t",
        allowed_origins=["http://x"],
    )
    app = build(cfg)
    running = asyncio.Event()

    async def job() -> None:
        running.set()
        await asyncio.Event().wait()

    app.state.container.workers.add("job", job)
    async with app.router.lifespan_context(app):
        await asyncio.wait_for(running.wait(), 1)
        task = app.state.container.workers._tasks[0]
    assert task.cancelled()


def test_serve_without_config_exits_1(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    for key in ("DATABASE_URL", "JWT_SECRET"):
        monkeypatch.delenv(key, raising=False)
    assert main(["serve"]) == 1
    line = json.loads(capsys.readouterr().out.splitlines()[-1])
    assert line["msg"] == "failed to load config"
    assert "required env vars not set" in line["error"]


SERVER = textwrap.dedent(
    """
    import asyncio, sys
    from fastapi import APIRouter
    from starlette.responses import Response
    from gv_api.config import Config
    from gv_api.core.app import create_app
    from gv_api.core.logging import setup_logging
    from gv_api.main import run_server

    router = APIRouter()

    @router.get("/slow")
    async def slow() -> Response:
        await asyncio.sleep(1.5)
        return Response("done")

    async def ping() -> None:
        return None

    setup_logging()
    cfg = Config(database_url="x", password="p", semiprivate_password="s", jwt_secret="j",
                 totp_secret="t", allowed_origins=["http://x"], port=sys.argv[1])
    run_server(create_app(cfg, ping=ping, routers=[router]), cfg)
    """
)


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def test_sigterm_lets_inflight_request_finish_and_exits_0() -> None:
    port = free_port()
    proc = subprocess.Popen(  # noqa: S603
        [sys.executable, "-c", SERVER, str(port)],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        env={**os.environ},
    )
    try:
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            try:
                httpx.get(f"http://127.0.0.1:{port}/health", timeout=0.2)
                break
            except httpx.HTTPError:
                time.sleep(0.05)
        result: dict[str, object] = {}

        def call() -> None:
            resp = httpx.get(f"http://127.0.0.1:{port}/slow", timeout=10)
            result["status"], result["body"] = resp.status_code, resp.text

        t = threading.Thread(target=call)
        t.start()
        time.sleep(0.3)
        proc.send_signal(signal.SIGTERM)
        t.join(10)
        code = proc.wait(10)
    finally:
        if proc.poll() is None:
            proc.kill()
    out = proc.stdout.read().decode() if proc.stdout else ""
    assert result == {"status": 200, "body": "done"}, out
    assert code == 0, out
    assert '"msg": "server stopped"' in out


def test_serve_exits_non_zero_when_migrations_fail(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    env = {
        "DATABASE_URL": "postgresql://u:p@127.0.0.1:1/db?sslmode=disable",
        "PASSWORD": "p",
        "SEMIPRIVATE_PASSWORD": "s",
        "JWT_SECRET": "j",
        "TOTP_SECRET": "t",
        "ALLOWED_ORIGINS": "http://x",
        "PORT": str(free_port()),
    }
    for key, value in env.items():
        monkeypatch.setenv(key, value)
    assert main(["serve"]) == 1
    msgs = [json.loads(line)["msg"] for line in capsys.readouterr().out.splitlines()]
    assert "failed to run migrations" in msgs
    assert "server stopped" not in msgs
