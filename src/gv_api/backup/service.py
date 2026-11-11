import asyncio
import gzip
import logging
import os
import re
import tempfile
from collections.abc import Awaitable, Callable
from contextlib import AbstractAsyncContextManager, suppress
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import BinaryIO, Protocol

from gv_api.backup.errors import InvalidNameError, NotFoundError
from gv_api.backup.schemas import Backup
from gv_api.core.dates import Clock, utcnow

log = logging.getLogger("gv_api")

HOURLY, DAILY = "hourly", "daily"
NAME = re.compile(r"gv-db-(\d{8}T\d{6}Z)\.sql\.gz")
STAMP = "%Y%m%dT%H%M%SZ"


class Dumper(Protocol):
    async def dump(self, out: gzip.GzipFile) -> None: ...


class Locker(Protocol):
    def hold(self) -> AbstractAsyncContextManager[None]: ...


class Uploader(Protocol):
    async def upload(self, key: str, file: Path) -> None: ...


class BackupService:
    """Gzipped dumps filed as hourly/ and daily/ (the day's first, as a hard link), pruned by
    age, and copied to S3 when configured."""

    def __init__(
        self,
        dumper: Dumper,
        locker: Locker,
        uploader: Uploader | None,
        dir: str,
        keep_hourly: timedelta,
        keep_daily: timedelta,
        now: Clock = utcnow,
    ) -> None:
        self._dumper = dumper
        self._locker = locker
        self._uploader = uploader
        self._dir = Path(dir)
        self.keep_hourly = keep_hourly
        self.keep_daily = keep_daily
        self._now = now

    async def run(self) -> Backup:
        """Take a backup, unless one is running anywhere (RunningError). A second run within
        the same second returns the first one."""
        async with self._locker.hold():
            for d in (HOURLY, DAILY):
                await asyncio.to_thread((self._dir / d).mkdir, parents=True, exist_ok=True)
            name = f"gv-db-{self._now().astimezone(UTC):{STAMP}}.sql.gz"
            existing = self._stat(HOURLY, name)
            if existing is not None:
                return existing
            await self._write(name)
            backup = self._stat(HOURLY, name)
            assert backup is not None  # noqa: S101 - just written

            daily = False
            try:
                daily = self._promote(backup)
            except OSError as err:
                log.warning("backup: creating daily copy", extra={"error": str(err)})
            if self._uploader is not None:
                try:
                    await self._upload(name, daily)
                except Exception as err:
                    log.error("backup: s3 upload failed", extra={"error": str(err)})
            try:
                self._prune()
            except OSError as err:
                log.warning("backup: pruning old backups", extra={"error": str(err)})
            return backup

    async def _write(self, name: str) -> None:
        """Dump into a temp file and rename it into place, so a half-written dump never shows."""
        tmp, raw, gz = await asyncio.to_thread(_open_temp, self._dir / HOURLY)
        try:
            await self._dumper.dump(gz)
            await asyncio.to_thread(_finish, raw, gz)
            await asyncio.to_thread(tmp.rename, self._path(HOURLY, name))
        except BaseException:
            await asyncio.to_thread(_discard, tmp, raw, gz)
            raise

    def _promote(self, backup: Backup) -> bool:
        """Hard-link the day's first backup into daily/."""
        day = f"{backup.created_at:%Y%m%d}"
        if any((self._dir / DAILY).glob(f"gv-db-{day}T*Z.sql.gz")):
            return False
        os.link(self._path(HOURLY, backup.name), self._path(DAILY, backup.name))
        return True

    async def _upload(self, name: str, daily: bool) -> None:
        assert self._uploader is not None  # noqa: S101 - checked by the caller
        await self._uploader.upload(f"{HOURLY}/{name}", self._path(HOURLY, name))
        if daily:
            await self._uploader.upload(f"{DAILY}/{name}", self._path(DAILY, name))

    def _prune(self) -> None:
        """Remove backups older than each tier keeps, always keeping the newest."""
        now = self._now()
        for d, keep in ((HOURLY, self.keep_hourly), (DAILY, self.keep_daily)):
            for i, b in enumerate(self._list(d)):
                if i > 0 and now - b.created_at >= keep:
                    self._path(d, b.name).unlink()

    def backups(self) -> list[Backup]:
        """Every backup once, newest first."""
        return self._list(HOURLY, DAILY)

    def _list(self, *dirs: str) -> list[Backup]:
        found: dict[str, Backup] = {}
        for d in dirs:
            folder = self._dir / d
            if not folder.is_dir():
                continue
            for entry in folder.iterdir():
                if entry.name not in found and NAME.fullmatch(entry.name) and entry.is_file():
                    backup = self._stat(d, entry.name)
                    if backup is not None:
                        found[entry.name] = backup
        return sorted(found.values(), key=lambda b: b.name, reverse=True)

    def _stat(self, dir: str, name: str) -> Backup | None:
        try:
            size = self._path(dir, name).stat().st_size
        except FileNotFoundError:
            return None
        m = NAME.fullmatch(name)
        assert m is not None  # noqa: S101 - only matching names are filed
        created = datetime.strptime(m[1], STAMP).replace(tzinfo=UTC)
        return Backup(name=name, size=size, created_at=created)

    def path(self, name: str) -> Path:
        """Where a backup is, for download."""
        if not NAME.fullmatch(name):
            raise InvalidNameError
        for d in (HOURLY, DAILY):
            if (p := self._path(d, name)).is_file():
                return p
        raise NotFoundError

    def _path(self, dir: str, name: str) -> Path:
        return self._dir / dir / name


def _open_temp(folder: Path) -> tuple[Path, BinaryIO, gzip.GzipFile]:
    fd, name = tempfile.mkstemp(prefix=".gv-db-", suffix=".tmp", dir=folder)
    raw = os.fdopen(fd, "wb")
    return Path(name), raw, gzip.GzipFile(fileobj=raw, mode="wb")


def _finish(raw: BinaryIO, gz: gzip.GzipFile) -> None:
    gz.close()
    raw.flush()
    os.fsync(raw.fileno())
    raw.close()


def _discard(tmp: Path, raw: BinaryIO, gz: gzip.GzipFile) -> None:
    with suppress(OSError, ValueError):
        gz.close()
    raw.close()
    tmp.unlink(missing_ok=True)


Gate = Callable[[], Awaitable[bool]]


class BackupWorker:
    """Takes a backup at every wall-clock multiple of the interval (UTC); 0 disables it."""

    def __init__(
        self,
        svc: BackupService,
        interval: timedelta,
        gate: Gate | None = None,
        now: Clock = utcnow,
    ) -> None:
        self._svc = svc
        self._interval = interval
        self._now = now
        self.gate = gate

    def delay(self) -> float:
        """Seconds until the next multiple of the interval."""
        every = self._interval.total_seconds()
        return every - self._now().timestamp() % every

    async def run(self) -> None:
        if self._interval <= timedelta(0):
            log.info("backup: schedule disabled")
            return
        log.info(
            "backup: schedule starting",
            extra={
                "interval": str(self._interval),
                "keep_hourly": str(self._svc.keep_hourly),
                "keep_daily": str(self._svc.keep_daily),
            },
        )
        while True:
            await asyncio.sleep(self.delay())
            await self.tick()

    async def tick(self) -> None:
        if self.gate is not None and not await self.gate():
            log.info("backup: skipped, a failover is in progress")
            return
        try:
            backup = await self._svc.run()
        except Exception as err:
            log.error("backup: scheduled backup failed", extra={"error": str(err)})
            return
        log.info("backup: done", extra={"file": backup.name, "size": backup.size})
