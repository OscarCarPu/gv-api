"""Replace the home database with the one restored from EC2, and report what home wrote since
the dump both started from. Works on local dump files; never touches S3."""

import asyncio
import gzip
import logging
import zlib
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.backup import PgDump
from gv_api.db.engine import create_engine
from gv_api.report import Report, build

log = logging.getLogger("gv_api")

CHUNK = 1 << 16

LastAlive = Callable[[AsyncEngine], Awaitable[datetime | None]]


class BadDumpError(Exception):
    def __init__(self) -> None:
        super().__init__("dump is not a readable gzip file or is empty")


@dataclass
class Request:
    id: str  # the failback dump's name: keys the pre-restore dump and the done marker
    base: Path
    ec2: Path


@dataclass
class Outcome:
    report: Report | None = None  # None when the restore had already been done
    last_alive: datetime | None = None
    swapped: bool = False
    already_done: bool = False
    pre_restore: Path | None = None


class Runner:
    def __init__(
        self,
        admin: AsyncEngine,
        url: Callable[[str], str],
        home: str,
        dir: Path,
        ignore: dict[str, list[str]] | None = None,
        skip: tuple[str, ...] = (),
        last_alive: LastAlive | None = None,
        release: Callable[[], Awaitable[None]] | None = None,
        psql: str = "psql",
    ) -> None:
        """`admin` is connected to another database than home, so home can be renamed; `url`
        builds the connection string of a database on the same server; `release` closes what
        the app holds open on home, right before the swap."""
        self._admin = admin
        self._url = url
        self._home = home
        self._dir = dir
        self._ignore = ignore
        self._skip = skip
        self._last_alive = last_alive
        self._release = release
        self._psql = psql

    @property
    def _base_db(self) -> str:
        return self._home + "_restore_base"

    @property
    def _ec2_db(self) -> str:
        return self._home + "_restore_ec2"

    @property
    def _old_db(self) -> str:
        return self._home + "_pre_restore"

    async def run(self, req: Request) -> Outcome:
        """Load both dumps into scratch databases, report what home holds that EC2's does not,
        and swap EC2's database into place. Starts from scratch every time, so it can be run
        again after a crash. A report that cannot be built is recorded, never fatal."""
        marker = self._dir / f"swapped-{req.id}"
        if await asyncio.to_thread(marker.exists):
            return Outcome(swapped=True, already_done=True)
        for dump in (req.base, req.ec2):
            await asyncio.to_thread(verify, dump)

        out = Outcome()
        out.pre_restore = await self._pre_restore(req.id)
        await self._load(self._base_db, req.base)
        await self._load(self._ec2_db, req.ec2)
        await self._inspect(out)
        if self._release is not None:
            await self._release()
        await self._swap()
        await asyncio.to_thread(marker.touch, 0o600)
        out.swapped = True
        return out

    async def _inspect(self, out: Outcome) -> None:
        engines = [
            create_engine(self._url(db), pool_size=1)
            for db in (self._base_db, self._home, self._ec2_db)
        ]
        base, home, ec2 = engines
        try:
            if self._last_alive is not None:
                try:
                    out.last_alive = await self._last_alive(home)
                except Exception as err:
                    log.warning(
                        "restore: cannot read home's last heartbeat", extra={"error": str(err)}
                    )
            try:
                out.report = await build(base, home, ec2, self._ignore, self._skip)
            except Exception as err:
                log.error("restore: cannot build the report", extra={"error": str(err)})
                out.report = Report.failed(datetime.now(UTC), err)
        finally:
            for engine in engines:
                await engine.dispose()

    async def _pre_restore(self, id: str) -> Path:
        """A dump of home before anything changes, kept beside the markers."""
        await asyncio.to_thread(self._dir.mkdir, mode=0o700, parents=True, exist_ok=True)
        path = self._dir / f"pre-restore-{id}.sql.gz"
        if await asyncio.to_thread(path.exists):
            return path
        tmp = path.with_name(path.name + ".part")
        try:
            raw = await asyncio.to_thread(tmp.open, "wb")
            with raw, gzip.GzipFile(fileobj=raw, mode="wb") as gz:
                await PgDump(self._url(self._home)).dump(gz)
            await asyncio.to_thread(verify, tmp)
            await asyncio.to_thread(tmp.rename, path)
        finally:
            await asyncio.to_thread(tmp.unlink, missing_ok=True)
        return path

    async def _load(self, db: str, dump: Path) -> None:
        await self._recreate(db)
        proc = await asyncio.create_subprocess_exec(
            self._psql,
            "-X",
            "-q",
            "-v",
            "ON_ERROR_STOP=1",
            "--dbname",
            self._url(db),
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.DEVNULL,
            stderr=asyncio.subprocess.PIPE,
        )
        assert proc.stdin is not None and proc.stderr is not None  # noqa: S101 - piped above
        stderr = asyncio.create_task(proc.stderr.read())
        source = await asyncio.to_thread(gzip.open, dump, "rb")
        try:
            while chunk := await asyncio.to_thread(source.read, CHUNK):
                proc.stdin.write(chunk)
                await proc.stdin.drain()
        except BrokenPipeError, ConnectionResetError:
            pass  # psql stopped on an error; its exit status says why
        except BaseException:
            proc.kill()
            await proc.wait()
            raise
        finally:
            await asyncio.to_thread(source.close)
        proc.stdin.close()
        if await proc.wait() != 0:
            detail = (await stderr).decode().strip()
            raise RuntimeError(
                f"load {dump.name} into {db}: exit status {proc.returncode}: {detail}"
            )

    async def _admin_exec(self, sql: str, params: dict[str, object] | None = None) -> None:
        async with self._admin.connect() as conn:
            await conn.execute(text(sql), params or {})

    async def _recreate(self, db: str) -> None:
        await self._admin_exec(f'DROP DATABASE IF EXISTS "{db}" WITH (FORCE)')
        await self._admin_exec(f'CREATE DATABASE "{db}"')

    async def _swap(self) -> None:
        """Rename the live database out of the way and EC2's into its place.

        New connections to home are refused first: the app's pool reconnects as soon as its
        sessions end, and a rename fails while any session is open. A failure puts everything
        back."""
        home, ec2, old = f'"{self._home}"', f'"{self._ec2_db}"', f'"{self._old_db}"'

        await self._admin_exec(f"ALTER DATABASE {home} ALLOW_CONNECTIONS false")
        try:
            await self._admin_exec(
                "SELECT pg_terminate_backend(pid) FROM pg_stat_activity"
                " WHERE datname = ANY(:dbs) AND pid <> pg_backend_pid()",
                {"dbs": [self._home, self._ec2_db, self._base_db]},
            )
            await self._admin_exec(f"DROP DATABASE IF EXISTS {old} WITH (FORCE)")
            await self._admin_exec(f"ALTER DATABASE {home} RENAME TO {old}")
        except BaseException:
            await self._admin_exec(f"ALTER DATABASE {home} ALLOW_CONNECTIONS true")
            raise
        try:
            await self._admin_exec(f"ALTER DATABASE {ec2} RENAME TO {home}")
        except BaseException:
            await self._admin_exec(f"ALTER DATABASE {old} RENAME TO {home}")
            await self._admin_exec(f"ALTER DATABASE {home} ALLOW_CONNECTIONS true")
            raise
        await self._admin_exec(f"ALTER DATABASE {old} ALLOW_CONNECTIONS true")
        await self._admin_exec(f'DROP DATABASE IF EXISTS "{self._base_db}" WITH (FORCE)')


def verify(dump: Path) -> None:
    """Read the whole gzip stream, so a truncated download is caught before anything changes."""
    total = 0
    try:
        with gzip.open(dump, "rb") as f:
            while chunk := f.read(CHUNK):
                total += len(chunk)
    except (OSError, EOFError, zlib.error) as err:
        raise BadDumpError from err
    if total == 0:
        raise BadDumpError
