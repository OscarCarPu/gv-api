"""BackupService over fake dumper, lock and uploader; the worker; the /backups router."""

import gzip
import json
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
from pathlib import Path
from types import SimpleNamespace
from typing import Any

import httpx
import pytest

from gv_api import backup
from gv_api.auth import AuthService
from gv_api.backup import BackupService, BackupWorker
from gv_api.backup.errors import RunningError
from gv_api.backup.schemas import Backup
from gv_api.core.app import create_app

from .test_tasks_router import AUTH, CFG, FULL, ping

NOW = datetime(2026, 9, 26, 10, 30, 15, tzinfo=UTC)
DAY = timedelta(days=1)


class Dumper:
    def __init__(self, out: bytes = b"x", err: Exception | None = None) -> None:
        self.out = out
        self.err = err

    async def dump(self, out: gzip.GzipFile) -> None:
        out.write(self.out)
        if self.err is not None:
            raise self.err


class Locker:
    def __init__(self, busy: bool = False) -> None:
        self.busy = busy

    @asynccontextmanager
    async def hold(self) -> AsyncIterator[None]:
        if self.busy:
            raise RunningError
        yield


class Uploader:
    def __init__(self, err: Exception | None = None) -> None:
        self.keys: list[str] = []
        self.err = err

    async def upload(self, key: str, file: Path) -> None:
        if self.err is not None:
            raise self.err
        self.keys.append(key)


def name_at(at: datetime) -> str:
    return f"gv-db-{at.astimezone(UTC):%Y%m%dT%H%M%SZ}.sql.gz"


def touch(dir: Path, sub: str, name: str, body: bytes = b"x") -> None:
    (dir / sub).mkdir(parents=True, exist_ok=True)
    (dir / sub / name).write_bytes(body)


def names(dir: Path, sub: str) -> list[str]:
    return sorted((p.name for p in (dir / sub).glob("gv-db-*T*Z.sql.gz")), reverse=True)


def svc(
    dir: Path,
    dumper: Dumper | None = None,
    locker: Locker | None = None,
    uploader: Uploader | None = None,
    now: datetime = NOW,
) -> BackupService:
    return BackupService(
        dumper or Dumper(),
        locker or Locker(),
        uploader,
        str(dir),
        2 * DAY,
        30 * DAY,
        lambda: now,
    )


async def test_writes_the_dump_gzipped_into_hourly(tmp_path: Path) -> None:
    b = await svc(tmp_path, Dumper(b"CREATE TABLE x();")).run()
    assert (b.name, b.created_at) == (name_at(NOW), NOW.replace(microsecond=0))
    assert gzip.decompress((tmp_path / "hourly" / b.name).read_bytes()) == b"CREATE TABLE x();"
    assert b.size == (tmp_path / "hourly" / b.name).stat().st_size
    assert [p.name for p in (tmp_path / "hourly").iterdir()] == [b.name], "no temp file left"


async def test_only_the_days_first_backup_goes_to_daily(tmp_path: Path) -> None:
    touch(tmp_path, "hourly", name_at(NOW - timedelta(seconds=1)))
    first = await svc(tmp_path).run()
    assert names(tmp_path, "daily") == [first.name]
    assert (tmp_path / "daily" / first.name).stat().st_ino == (
        tmp_path / "hourly" / first.name
    ).stat().st_ino, "a hard link"
    await svc(tmp_path, now=NOW + timedelta(hours=1)).run()
    assert names(tmp_path, "daily") == [first.name]
    assert len(svc(tmp_path).backups()) == len(names(tmp_path, "hourly")) == 3


async def test_prunes_by_age_per_folder_and_leaves_other_files(tmp_path: Path) -> None:
    recent = name_at(NOW - timedelta(hours=1))
    stale_hourly = name_at(NOW - 3 * DAY)
    recent_daily = name_at(NOW - 5 * DAY)
    stale_daily = name_at(NOW - 40 * DAY)
    touch(tmp_path, "hourly", recent)
    touch(tmp_path, "hourly", stale_hourly)
    touch(tmp_path, "daily", recent_daily)
    touch(tmp_path, "daily", stale_daily)
    touch(tmp_path, "daily", "gv-db-20260926.sql.gz")

    b = await svc(tmp_path).run()
    assert names(tmp_path, "hourly") == [b.name, recent]
    assert names(tmp_path, "daily") == [b.name, recent_daily]
    assert (tmp_path / "daily" / "gv-db-20260926.sql.gz").exists()
    assert [x.name for x in svc(tmp_path).backups()] == [b.name, recent, recent_daily]


async def test_the_newest_is_kept_however_old(tmp_path: Path) -> None:
    old = name_at(NOW - 100 * DAY)
    touch(tmp_path, "daily", old)
    await svc(tmp_path).run()
    assert old not in names(tmp_path, "daily")
    touch(tmp_path, "hourly", name_at(NOW - 99 * DAY))
    await svc(tmp_path, now=NOW + 10 * DAY).run()
    assert len(names(tmp_path, "daily")) == 2, "a daily copy for each day, the newer kept"


async def test_a_second_run_in_the_same_second_returns_the_existing_one(tmp_path: Path) -> None:
    touch(tmp_path, "hourly", name_at(NOW))
    b = await svc(tmp_path, Dumper(b"new")).run()
    assert (tmp_path / "hourly" / b.name).read_bytes() == b"x"


async def test_a_failed_dump_leaves_no_file_and_deletes_nothing(tmp_path: Path) -> None:
    touch(tmp_path, "hourly", name_at(NOW - 40 * DAY))
    touch(tmp_path, "hourly", name_at(NOW - 41 * DAY))
    with pytest.raises(RuntimeError, match="boom"):
        await svc(tmp_path, Dumper(err=RuntimeError("boom"))).run()
    assert len(list((tmp_path / "hourly").iterdir())) == 2


async def test_running_elsewhere(tmp_path: Path) -> None:
    with pytest.raises(RunningError):
        await svc(tmp_path, locker=Locker(busy=True)).run()


def test_path(tmp_path: Path) -> None:
    in_hourly, only_daily = "gv-db-20260102T000000Z.sql.gz", "gv-db-20260101T000000Z.sql.gz"
    touch(tmp_path, "hourly", in_hourly)
    touch(tmp_path, "daily", in_hourly)
    touch(tmp_path, "daily", only_daily)
    s = svc(tmp_path)
    assert s.path(in_hourly) == tmp_path / "hourly" / in_hourly
    assert s.path(only_daily) == tmp_path / "daily" / only_daily
    with pytest.raises(Exception, match="invalid backup name"):
        s.path("../.env")
    with pytest.raises(Exception, match="backup not found"):
        s.path("gv-db-20260103T000000Z.sql.gz")
    assert svc(tmp_path / "missing").backups() == []


async def test_upload(tmp_path: Path) -> None:
    up = Uploader()
    first = await svc(tmp_path, uploader=up).run()
    assert up.keys == [f"hourly/{first.name}", f"daily/{first.name}"]
    up.keys = []
    second = await svc(tmp_path, uploader=up, now=NOW + timedelta(seconds=1)).run()
    assert up.keys == [f"hourly/{second.name}"]


async def test_a_failed_upload_keeps_the_backup(
    tmp_path: Path, caplog: pytest.LogCaptureFixture
) -> None:
    b = await svc(tmp_path, uploader=Uploader(RuntimeError("denied"))).run()
    assert (tmp_path / "hourly" / b.name).exists()
    assert "backup: s3 upload failed" in caplog.messages


async def test_a_failed_daily_link_and_prune_only_warn(
    tmp_path: Path, caplog: pytest.LogCaptureFixture, monkeypatch: pytest.MonkeyPatch
) -> None:
    def fail(*_: Any) -> None:
        raise OSError("read-only")

    monkeypatch.setattr("gv_api.backup.service.os.link", fail)
    touch(tmp_path, "hourly", name_at(NOW - 3 * DAY))
    monkeypatch.setattr(Path, "unlink", fail)
    assert (await svc(tmp_path).run()).name == name_at(NOW)
    assert caplog.messages[:2] == ["backup: creating daily copy", "backup: pruning old backups"]


# Worker


def test_worker_waits_for_the_next_multiple() -> None:
    worker = BackupWorker(svc(Path("/x")), timedelta(minutes=5), now=lambda: NOW)
    assert worker.delay() == pytest.approx(4 * 60 + 45)


async def test_worker_disabled(caplog: pytest.LogCaptureFixture) -> None:
    caplog.set_level(logging.INFO, "gv_api")
    await BackupWorker(svc(Path("/x")), timedelta(0)).run()
    assert caplog.messages == ["backup: schedule disabled"]


async def test_worker_ticks(tmp_path: Path, caplog: pytest.LogCaptureFixture) -> None:
    caplog.set_level(logging.INFO, "gv_api")
    worker = BackupWorker(svc(tmp_path), timedelta(minutes=5))
    await worker.tick()
    assert names(tmp_path, "hourly") == [name_at(NOW)]

    async def closed() -> bool:
        return False

    worker.gate = closed
    await worker.tick()
    failing = BackupWorker(svc(tmp_path, Dumper(err=RuntimeError("x")), now=NOW + DAY), DAY)
    await failing.tick()
    assert caplog.messages == [
        "backup: done",
        "backup: skipped, a failover is in progress",
        "backup: scheduled backup failed",
    ]


# Router


async def call(service: BackupService, method: str, path: str) -> httpx.Response:
    app = create_app(CFG, ping=ping, routers=[backup.router])
    app.state.container = SimpleNamespace(services={AuthService: AUTH, BackupService: service})
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        return await c.request(method, path, headers=FULL)


async def test_router(tmp_path: Path) -> None:
    touch(tmp_path, "hourly", "gv-db-20260101T000000Z.sql.gz", b"dump")
    resp = await call(svc(tmp_path), "GET", "/backups")
    assert (resp.status_code, resp.json()) == (
        200,
        [
            {
                "name": "gv-db-20260101T000000Z.sql.gz",
                "size": 4,
                "created_at": "2026-01-01T00:00:00Z",
            }
        ],
    )
    resp = await call(svc(tmp_path), "GET", "/backups/gv-db-20260101T000000Z.sql.gz")
    assert (resp.status_code, resp.content) == (200, b"dump")
    assert resp.headers["content-type"] == "application/gzip"
    assert resp.headers["content-disposition"] == (
        'attachment; filename="gv-db-20260101T000000Z.sql.gz"'
    )

    resp = await call(svc(tmp_path), "POST", "/backups")
    assert (resp.status_code, resp.json()["name"]) == (201, name_at(NOW))


async def test_router_errors(tmp_path: Path) -> None:
    resp = await call(svc(tmp_path, locker=Locker(busy=True)), "POST", "/backups")
    assert (resp.status_code, resp.json()) == (409, {"error": "a backup is already running"})
    resp = await call(svc(tmp_path, Dumper(err=RuntimeError("x"))), "POST", "/backups")
    assert (resp.status_code, resp.json()) == (500, {"error": "Backup failed"})
    resp = await call(svc(tmp_path), "GET", "/backups/gv-db-x.sql.gz")
    assert (resp.status_code, resp.json()) == (400, {"error": "invalid backup name"})
    resp = await call(svc(tmp_path), "GET", "/backups/gv-db-20260103T000000Z.sql.gz")
    assert (resp.status_code, resp.json()) == (404, {"error": "backup not found"})
    # Raised directly: a chmod-ed folder still lists for root, as in CI.
    broken = svc(tmp_path)

    def denied() -> list[Backup]:
        raise PermissionError("denied")

    broken.backups = denied  # type: ignore[method-assign]
    resp = await call(broken, "GET", "/backups")
    assert (resp.status_code, resp.json()) == (500, {"error": "Failed to list backups"})


# The backup command


ENV = {
    "DATABASE_URL": "postgresql://u:p@127.0.0.1:1/db?sslmode=disable",
    "PASSWORD": "p",
    "SEMIPRIVATE_PASSWORD": "s",
    "JWT_SECRET": "j",
    "TOTP_SECRET": "t",
    "ALLOWED_ORIGINS": "http://x",
}


def run_backup(monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]) -> list[str]:
    from gv_api.cli import main

    for key, value in ENV.items():
        monkeypatch.setenv(key, value)
    assert main(["backup"]) == 1
    return [json.loads(line)["msg"] for line in capsys.readouterr().out.splitlines()]


def test_backup_command_without_config(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    from gv_api.cli import main

    monkeypatch.delenv("JWT_SECRET", raising=False)
    monkeypatch.delenv("DATABASE_URL", raising=False)
    assert main(["backup"]) == 1


def test_backup_command_without_a_database(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    assert "failed to connect to database" in run_backup(monkeypatch, capsys)


async def _no_ping(_: object) -> None:
    return None


def test_backup_command_reports_a_failed_backup(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    async def fail(self: BackupService) -> None:
        raise RuntimeError("disk full")

    monkeypatch.setattr("gv_api.db.engine.ping", _no_ping)
    monkeypatch.setattr(BackupService, "run", fail)
    assert "backup failed" in run_backup(monkeypatch, capsys)


def test_backup_command_reports_a_bad_setup(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    def broken(*_: object) -> None:
        raise RuntimeError("no credentials")

    monkeypatch.setattr("gv_api.db.engine.ping", _no_ping)
    monkeypatch.setattr("gv_api.backup.backup_service", broken)
    assert "failed to set up backups" in run_backup(monkeypatch, capsys)
