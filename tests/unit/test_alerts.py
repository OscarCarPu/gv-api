import asyncio
import io
import json
import logging
import socket
import threading
import time
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, HTTPServer

import httpx
import pytest
from fastapi import APIRouter
from starlette.responses import Response

from gv_api.alerts import TelegramHandler, install, telegram
from gv_api.cli import main
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.logging import request_id_var, setup_logging

TOKEN = "123456:SECRET-bot-token"


class Recorder:
    def __init__(self, status: int = 200, fail: Exception | None = None, hang: bool = False):
        self.requests: list[httpx.Request] = []
        self.status, self.fail, self.hang = status, fail, hang

    async def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        if self.hang:
            await asyncio.sleep(30)
        if self.fail:
            raise self.fail
        return httpx.Response(self.status, json={"ok": self.status == 200, "description": "nope"})

    def texts(self) -> list[str]:
        return [json.loads(r.content)["text"] for r in self.requests]


@pytest.fixture
def log() -> Iterator[logging.Logger]:
    logger = logging.getLogger("test.alerts")
    logger.propagate = False
    logger.setLevel(logging.DEBUG)
    yield logger
    logger.handlers.clear()
    logger.propagate = True


def handler_for(log: logging.Logger, rec: Recorder, side: str = "home") -> TelegramHandler:
    h = TelegramHandler(TOKEN, "42", side, transport=httpx.MockTransport(rec))
    log.addHandler(h)
    return h


async def test_exact_message_format(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec, side="aws")
    log.error("backup: scheduled backup failed", extra={"error": "pg_dump: exit status 1"})
    token = request_id_var.set("req-7")
    try:
        for _ in range(3):
            log.error("Failed to get daily habits", extra={"error": "connection refused"})
    finally:
        request_id_var.reset(token)
    await h.flush_batch()

    (req,) = rec.requests
    assert req.url == f"https://api.telegram.org/bot{TOKEN}/sendMessage"
    body = json.loads(req.content)
    assert body["chat_id"] == "42"
    assert body["disable_web_page_preview"] is True
    assert body["text"] == (
        "gv-api aws: 4 errors\n\n"
        "backup: scheduled backup failed\n"
        "error: pg_dump: exit status 1\n\n"
        "Failed to get daily habits (x3)\n"
        "error: connection refused\n"
        "request_id: req-7"
    )


async def test_error_storm_collapses_into_one_line(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    for i in range(200):
        token = request_id_var.set(f"r{i}")
        log.error("Failed to list tasks", extra={"error": "db down"})
        request_id_var.reset(token)
    await h.flush_batch()
    (text,) = rec.texts()
    assert (
        text
        == "gv-api home: 200 errors\n\nFailed to list tasks (x200)\nerror: db down\nrequest_id: r0"
    )


async def test_same_message_with_other_error_is_its_own_line(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    log.error("restore failed", extra={"error": "a"})
    log.error("restore failed", extra={"error": "b"})
    await h.flush_batch()
    assert rec.texts()[0].count("restore failed\n") == 2


async def test_exception_text_is_the_error(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    try:
        raise RuntimeError("kaboom")
    except RuntimeError:
        log.exception("unhandled error")
    await h.flush_batch()
    assert rec.texts()[0].endswith("unhandled error\nerror: kaboom")


async def test_only_error_and_above(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    log.warning("careful")
    log.info("fine")
    await h.flush_batch()
    assert rec.requests == []
    log.critical("down")
    await h.flush_batch()
    assert rec.texts() == ["gv-api home: 1 error\n\ndown"]


async def test_batch_is_cleared_after_sending(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    log.error("one")
    await h.flush_batch()
    await h.flush_batch()
    log.error("two")
    await h.flush_batch()
    assert rec.texts() == ["gv-api home: 1 error\n\none", "gv-api home: 1 error\n\ntwo"]


async def test_run_flushes_on_the_interval(
    log: logging.Logger, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(telegram, "FLUSH_INTERVAL", 0.05)
    rec = Recorder()
    h = handler_for(log, rec)
    task = asyncio.create_task(h.run())
    try:
        log.error("a")
        log.error("b")
        await asyncio.sleep(0.08)
        assert len(rec.requests) == 1
        await asyncio.sleep(0.05)
        assert len(rec.requests) == 1  # nothing pending, nothing sent
    finally:
        task.cancel()


def test_long_batch_is_cut_at_an_entry_and_says_so() -> None:
    entries = [telegram._Entry(f"error number {i}", "x" * 80, None) for i in range(100)]
    text = telegram.render("home", entries)
    assert len(text) <= 4096
    assert text.startswith("gv-api home: 100 errors\n\nerror number 0\n")
    shown = text.count("error number ")
    assert text.endswith(f"\n\n[truncated: {100 - shown} more not shown]")
    assert text.split("\n\n")[-2].startswith(f"error number {shown - 1}\n")


def test_single_huge_entry_is_cut_and_says_so() -> None:
    entries = [
        telegram._Entry("pg_dump failed", "e" * 10_000, None),
        telegram._Entry("other", None, None),
    ]
    text = telegram.render("home", entries)
    assert len(text) == 4096
    assert text.startswith("gv-api home: 2 errors\n\npg_dump failed\nerror: eee")
    assert text.endswith("eee\n[truncated]\n\n[truncated: 1 more not shown]")


async def test_buffer_is_bounded_and_overflow_is_reported(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    for i in range(telegram.MAX_ENTRIES + 5):
        log.error(f"e{i}")
    log.error("e0")  # a known message still counts after the buffer is full
    await h.flush_batch()
    text = rec.texts()[0]
    assert text.startswith(f"gv-api home: {telegram.MAX_ENTRIES + 6} errors\n\ne0 (x2)\n")
    assert text.endswith("\n\n[+5 more dropped: buffer full]")
    assert len(text) <= 4096


# 4.2: delivery never affects the service and never leaks the token


async def test_unreachable_api_drops_batch_with_warning(
    log: logging.Logger, caplog: pytest.LogCaptureFixture
) -> None:
    rec = Recorder(fail=httpx.ConnectError(f"cannot reach https://api.telegram.org/bot{TOKEN}"))
    h = handler_for(log, rec)
    log.error("a")
    log.error("a")
    with caplog.at_level(logging.WARNING, logger="gv_api.alerts"):
        await h.flush_batch()
    (warn,) = [r for r in caplog.records if r.name == "gv_api.alerts"]
    assert warn.levelno == logging.WARNING
    assert warn.getMessage() == "telegram: alert batch dropped"
    assert warn.__dict__["records"] == 2
    assert TOKEN not in warn.__dict__["error"]
    assert "ConnectError" in warn.__dict__["error"]
    await h.flush_batch()
    assert len(rec.requests) == 1  # the batch was dropped, not retried


async def test_rejected_message_is_a_warning(
    log: logging.Logger, caplog: pytest.LogCaptureFixture
) -> None:
    h = handler_for(log, Recorder(status=400))
    log.error("a")
    with caplog.at_level(logging.WARNING, logger="gv_api.alerts"):
        await h.flush_batch()
    (warn,) = [r for r in caplog.records if r.name == "gv_api.alerts"]
    assert warn.__dict__["status"] == 400


async def test_send_uses_a_5s_timeout(log: logging.Logger) -> None:
    rec = Recorder()
    h = handler_for(log, rec)
    log.error("a")
    await h.flush_batch()
    timeout = rec.requests[0].extensions["timeout"]
    assert timeout == {"connect": 5.0, "read": 5.0, "write": 5.0, "pool": 5.0}


async def test_own_warnings_are_not_alerted() -> None:
    rec = Recorder()
    h = TelegramHandler(TOKEN, "42", "home", transport=httpx.MockTransport(rec))
    root = logging.getLogger()
    root.addHandler(h)
    try:
        logging.getLogger("gv_api.alerts").error("would loop")
    finally:
        root.removeHandler(h)
    await h.flush_batch()
    assert rec.requests == []


async def test_token_never_in_logs() -> None:
    root = logging.getLogger()
    saved = (root.handlers[:], root.level)
    out = io.StringIO()
    setup_logging(out)
    rec = Recorder()
    h = TelegramHandler(TOKEN, "42", "home", transport=httpx.MockTransport(rec))
    root.addHandler(h)
    try:
        logging.getLogger("gv_api").error("boom")
        await h.flush_batch()  # success: httpx would log the URL at INFO
        rec.fail = httpx.ConnectTimeout(f"timed out on /bot{TOKEN}/sendMessage")
        logging.getLogger("gv_api").error("boom again")
        await h.flush_batch()
        rec.fail, rec.status = None, 500
        logging.getLogger("gv_api").error("and again")
        await h.flush_batch()
    finally:
        root.handlers[:], root.level = saved
    logged = out.getvalue()
    assert len(rec.requests) == 3
    assert logged.count("telegram: alert batch dropped") == 2
    assert TOKEN not in logged
    assert all(TOKEN not in t for t in rec.texts())


async def test_hanging_api_leaves_request_latency_unchanged() -> None:
    router = APIRouter()
    errlog = logging.getLogger("gv_api")

    @router.get("/fail")
    async def fail() -> Response:
        errlog.error("Failed to get daily habits", extra={"error": "boom"})
        return Response(status_code=500)

    async def ping() -> None:
        return None

    cfg = Config(
        database_url="x",
        password="p",
        semiprivate_password="s",
        jwt_secret="j",
        totp_secret="t",
        allowed_origins=["http://x"],
    )
    app = create_app(cfg, ping=ping, routers=[router])
    rec = Recorder(hang=True)
    h = TelegramHandler(TOKEN, "42", "home", transport=httpx.MockTransport(rec))
    root = logging.getLogger()
    root.addHandler(h)
    try:
        errlog.error("pending")
        sending = asyncio.create_task(h.flush_batch())
        await asyncio.sleep(0.01)
        assert rec.requests  # the send is now stuck
        client = httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://t")
        async with client:
            start = time.perf_counter()
            for _ in range(20):
                assert (await client.get("/fail")).status_code == 500
            elapsed = time.perf_counter() - start
        sending.cancel()
    finally:
        root.removeHandler(h)
    assert elapsed < 0.5
    entries, _ = h._take()
    assert entries[0].count == 20
    assert entries[0].request_id  # set by the request-id middleware


# 4.3: disabled mode and the fatal-exit flush


def base_env(monkeypatch: pytest.MonkeyPatch, port: int) -> None:
    env = {
        "DATABASE_URL": "postgresql://u:p@127.0.0.1:1/db?sslmode=disable",
        "PASSWORD": "p",
        "SEMIPRIVATE_PASSWORD": "s",
        "JWT_SECRET": "j",
        "TOTP_SECRET": "t",
        "ALLOWED_ORIGINS": "http://x",
        "PORT": str(port),
    }
    for key, value in env.items():
        monkeypatch.setenv(key, value)
    monkeypatch.delenv("TELEGRAM_BOT_TOKEN", raising=False)
    monkeypatch.delenv("TELEGRAM_CHAT_ID", raising=False)


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


@pytest.mark.parametrize("env", [{}, {"TELEGRAM_BOT_TOKEN": TOKEN}, {"TELEGRAM_CHAT_ID": "42"}])
def test_unset_variables_log_one_info_line(
    env: dict[str, str], monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    base_env(monkeypatch, free_port())
    for key, value in env.items():
        monkeypatch.setenv(key, value)
    root_handlers = logging.getLogger().handlers[:]
    assert main(["serve"]) == 1  # the DB is unreachable
    lines = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    off = [line for line in lines if line["msg"].startswith("telegram alerts disabled")]
    assert len(off) == 1
    assert off[0]["level"] == "INFO"
    assert not any(isinstance(h, TelegramHandler) for h in logging.getLogger().handlers)
    logging.getLogger().handlers[:] = root_handlers


def test_install_attaches_handler_when_configured() -> None:
    cfg = Config(telegram_bot_token=TOKEN, telegram_chat_id="42", failover_side="aws")
    h = install(cfg)
    try:
        assert isinstance(h, TelegramHandler)
        assert h in logging.getLogger().handlers
        assert h._side == "aws"
    finally:
        if h is not None:
            logging.getLogger().removeHandler(h)


class _FakeTelegram(BaseHTTPRequestHandler):
    received: list[tuple[str, dict[str, object]]] = []  # noqa: RUF012

    def do_POST(self) -> None:
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.received.append((self.path, body))
        payload = b'{"ok":true}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format: str, *args: object) -> None:
        pass


def test_failing_startup_sends_alert_before_exit(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    server = HTTPServer(("127.0.0.1", 0), _FakeTelegram)
    _FakeTelegram.received = []
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    root_handlers = logging.getLogger().handlers[:]
    try:
        monkeypatch.setattr(telegram, "API_URL", f"http://127.0.0.1:{server.server_port}")
        base_env(monkeypatch, free_port())
        monkeypatch.setenv("TELEGRAM_BOT_TOKEN", TOKEN)
        monkeypatch.setenv("TELEGRAM_CHAT_ID", "42")
        assert main(["serve"]) == 1
    finally:
        server.shutdown()
        logging.getLogger().handlers[:] = root_handlers
    ((path, body),) = _FakeTelegram.received
    assert path == f"/bot{TOKEN}/sendMessage"
    assert body["chat_id"] == "42"
    text = str(body["text"])
    assert text.startswith("gv-api home: ")
    assert "failed to run migrations" in text
    out = capsys.readouterr().out
    assert TOKEN not in out
    assert "telegram alerts disabled" not in out
    assert not any(isinstance(h, TelegramHandler) for h in logging.getLogger().handlers)


def test_exit_flush_is_capped() -> None:
    rec = Recorder(hang=True)
    h = TelegramHandler(TOKEN, "42", "home", transport=httpx.MockTransport(rec))
    h.emit(logging.makeLogRecord({"msg": "x", "levelno": logging.ERROR}))
    start = time.monotonic()
    h.flush_on_exit(timeout=0.2)
    assert time.monotonic() - start < 1
    assert rec.requests
