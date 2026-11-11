"""What a backup runs on: pg_dump, the cross-process lock and the S3 copy."""

import asyncio
import gzip
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path
from typing import TYPE_CHECKING

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.backup.errors import RunningError
from gv_api.backup.service import BackupService
from gv_api.config import Config
from gv_api.core import aws

if TYPE_CHECKING:
    from mypy_boto3_s3 import S3Client

CHUNK = 1 << 16


class PgDump:
    def __init__(self, url: str) -> None:
        self._url = url

    async def dump(self, out: gzip.GzipFile) -> None:
        """A plain-SQL dump that recreates every object, written to `out`."""
        proc = await asyncio.create_subprocess_exec(
            "pg_dump",
            "--clean",
            "--if-exists",
            "--no-owner",
            "--no-privileges",
            "--dbname",
            self._url,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        assert proc.stdout is not None and proc.stderr is not None  # noqa: S101 - piped above
        stderr = asyncio.create_task(proc.stderr.read())
        try:
            while chunk := await proc.stdout.read(CHUNK):
                await asyncio.to_thread(out.write, chunk)
        except BaseException:
            proc.kill()
            await proc.wait()
            raise
        code = await proc.wait()
        if code != 0:
            raise RuntimeError(f"pg_dump: exit status {code}: {(await stderr).decode().strip()}")


class PgLocker:
    """pg_try_advisory_lock(hashtext('backup')): one backup at a time across processes."""

    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    @asynccontextmanager
    async def hold(self) -> AsyncIterator[None]:
        async with self._engine.connect() as conn:
            taken = await conn.scalar(text("SELECT pg_try_advisory_lock(hashtext('backup'))"))
            if not taken:
                raise RunningError
            try:
                yield
            finally:
                try:
                    await conn.execute(text("SELECT pg_advisory_unlock(hashtext('backup'))"))
                except Exception:
                    # A session lock dies with its connection.
                    await conn.invalidate()


class S3Uploader:
    def __init__(self, bucket: str) -> None:
        self._bucket = bucket
        self._client: S3Client | None = None

    async def upload(self, key: str, file: Path) -> None:
        await asyncio.to_thread(self._put, key, file)

    def _put(self, key: str, file: Path) -> None:
        if self._client is None:
            self._client = aws.client("s3")
        with file.open("rb") as body:
            self._client.put_object(
                Bucket=self._bucket, Key="gv-db/" + key, Body=body, IfNoneMatch="*"
            )


def backup_service(cfg: Config, engine: AsyncEngine) -> BackupService:
    uploader = S3Uploader(cfg.backup_s3_bucket) if cfg.backup_s3_bucket else None
    return BackupService(
        PgDump(cfg.database_url),
        PgLocker(engine),
        uploader,
        cfg.backup_dir,
        cfg.backup_keep_hourly,
        cfg.backup_keep_daily,
    )
