"""Home's side of an AWS failover: hold the app while the backup server owns the data, then
restore that server's data and keep a report of what home had besides.

The AWS state machine is the authority. Requests only say what to do; each one is checked
against the state in SSM, so a leaked token cannot start a restore or choose a dump."""

import asyncio
import json
import logging
from collections.abc import Awaitable, Callable
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Protocol

from starlette.types import ASGIApp, Receive, Scope, Send

from gv_api.core.http import error_code
from gv_api.failover.aws import NoParamError
from gv_api.report import Report
from gv_api.restore import Outcome, Request

log = logging.getLogger("gv_api")

MERGE_PENDING = "merge_pending"
PENDING_MESSAGE = "Restoring data after the failover. Back in a moment."

STATE_PARAM = "/home-lab-failover/state"
BASE_PARAM = "/home-lab-failover/base"
DUMP_PARAM = "/home-lab-failover/dump"

NORMAL, FAILING_BACK, PENDING_MERGE, HOME_READY = (
    "NORMAL",
    "FAILING_BACK",
    "PENDING_MERGE",
    "HOME_READY",
)

# The app's mode, and the restore's progress ("" before any).
MODE_NORMAL, MODE_HOLD, MODE_MERGE = "normal", "hold", "merge"
MERGE_RUNNING, MERGE_DONE, MERGE_FAILED = "running", "done", "failed"

# Routes the failover itself needs while the app is held.
EXEMPT = ("/health", "/login", "/failover", "/report")


class Params(Protocol):
    async def get(self, name: str) -> str: ...


class Store(Protocol):
    async def download(self, key: str, dest: Path) -> None: ...


class Restorer(Protocol):
    async def run(self, req: Request) -> Outcome: ...


class Outages(Protocol):
    async def record_failover(
        self, last_alive: datetime | None, ec2_up_at: datetime, home_at: datetime
    ) -> None: ...


class FailoverController:
    def __init__(
        self,
        token: str,
        params: Params,
        store: Store,
        restorer: Restorer,
        dir: Path,
        after: Callable[[], Awaitable[None]] | None = None,
        retry: float = 30,
    ) -> None:
        """`dir` keeps the downloaded dumps and the report; `after` runs once a restore swapped
        the database, before the app unlocks; `retry` is how often an unreadable state is read
        again while the app is held."""
        self.token = token
        self._params = params
        self._store = store
        self._restorer = restorer
        self._dir = dir
        self._after = after
        self._retry = retry
        self.mode = MODE_NORMAL
        self.merge = ""
        self.error = ""
        self.report: Report | None = None
        self.last_alive: datetime | None = None
        self.outages: Outages | None = None
        self._tasks: set[asyncio.Task[None]] = set()
        self._merging = False
        self._load_report()

    # State

    async def state(self) -> str:
        try:
            raw = await self._params.get(STATE_PARAM)
        except NoParamError:
            return NORMAL
        state = json.loads(raw)["state"]
        return str(state)

    def set_mode(self, mode: str) -> None:
        if mode == MODE_HOLD and self.merge in (MERGE_DONE, MERGE_FAILED):
            self.merge, self.error = "", ""
        self.mode = mode

    async def start(self) -> None:
        """Read the state once. Anything but NORMAL holds the app, and PENDING_MERGE also
        resumes the restore. An unreadable state holds the app and is read again until it
        works."""
        try:
            state = await self.state()
        except Exception as err:
            log.error("failover state unreadable, holding the app", extra={"error": str(err)})
            self.set_mode(MODE_HOLD)
            self._spawn(self._retry_until_read())
            return
        self._apply(state)

    def _apply(self, state: str) -> None:
        if state == NORMAL:
            self.set_mode(MODE_NORMAL)
        elif state == PENDING_MERGE:
            self.start_merge()
        else:
            self.set_mode(MODE_HOLD)

    async def _retry_until_read(self) -> None:
        while True:
            await asyncio.sleep(self._retry)
            try:
                state = await self.state()
            except Exception:  # noqa: S112 - keep holding until it can be read
                continue
            self._apply(state)
            return

    async def allowed(self) -> bool:
        """Whether a worker may run. Rereads the state, which also catches a failover the push
        never reached. An unreadable state does not block it: without SSM there is no S3
        either, so nothing stale can be uploaded."""
        if self.mode != MODE_NORMAL:
            return False
        try:
            state = await self.state()
        except Exception as err:
            log.warning(
                "failover state unreadable, letting the worker run", extra={"error": str(err)}
            )
            return True
        if state != NORMAL:
            self.set_mode(MODE_HOLD)
            return False
        return True

    async def beating(self) -> bool:
        """The uptime heartbeat writes only while home serves and the state is NORMAL: the
        Lambda keeps it at HOME_READY until the failover record is saved."""
        if self.mode != MODE_NORMAL:
            return False
        try:
            return await self.state() == NORMAL
        except Exception:
            return True

    async def has_dump(self) -> bool:
        """Whether the backup server has uploaded its failback dump."""
        try:
            await self._params.get(DUMP_PARAM)
        except Exception:
            return False
        return True

    # Merge

    def start_merge(self) -> None:
        if self._merging or self.merge == MERGE_DONE:
            return
        self._merging = True
        self.mode, self.merge, self.error = MODE_MERGE, MERGE_RUNNING, ""
        self._spawn(self._run_merge())

    async def _run_merge(self) -> None:
        try:
            out = await self._restore()
            if self._after is not None:
                await self._after()
        except Exception as err:
            log.error("restore failed", extra={"error": str(err)})
            self.merge, self.error = MERGE_FAILED, str(err)
            return
        finally:
            self._merging = False
        if out.report is not None:
            self.report = out.report
            await asyncio.to_thread(self._save_report)
        if not out.already_done:
            self.last_alive = out.last_alive
        self.merge, self.mode = MERGE_DONE, MODE_NORMAL

    async def _restore(self) -> Outcome:
        base_key = await self._params.get(BASE_PARAM)
        dump_key = await self._params.get(DUMP_PARAM)
        downloads = self._dir / "downloads"
        await asyncio.to_thread(downloads.mkdir, mode=0o700, parents=True, exist_ok=True)
        base = downloads / PurePosixPath(base_key).name
        ec2 = downloads / PurePosixPath(dump_key).name
        await self._store.download(base_key, base)
        await self._store.download(dump_key, ec2)
        id = PurePosixPath(dump_key).name.removesuffix(".sql.gz")
        return await self._restorer.run(Request(id=id, base=base, ec2=ec2))

    def _spawn(self, job: Awaitable[None]) -> None:
        task: asyncio.Task[None] = asyncio.ensure_future(job)
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)

    async def stop(self) -> None:
        for task in list(self._tasks):
            task.cancel()
        await asyncio.gather(*self._tasks, return_exceptions=True)

    # Report

    @property
    def _report_file(self) -> Path:
        return self._dir / "report.json"

    def _load_report(self) -> None:
        try:
            raw = self._report_file.read_text()
        except OSError:
            return
        try:
            self.report = Report.model_validate_json(raw)
        except ValueError as err:
            log.warning("ignoring unreadable report file", extra={"error": str(err)})

    def _save_report(self) -> None:
        """Keep the report across a restart."""
        assert self.report is not None  # noqa: S101 - only saved once there is one
        try:
            self._dir.mkdir(mode=0o700, parents=True, exist_ok=True)
            self._report_file.write_text(self.report.model_dump_json())
            self._report_file.chmod(0o600)
        except OSError as err:
            log.warning("cannot save the report", extra={"error": str(err)})

    def dismiss_report(self) -> None:
        self.report = None
        self._report_file.unlink(missing_ok=True)


class HoldLock:
    """While the app is held or merging, 503 on everything but what the failover needs."""

    def __init__(self, app: ASGIApp, controller: FailoverController) -> None:
        self.app = app
        self.controller = controller

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http" or self.controller.mode == MODE_NORMAL or _exempt(scope["path"]):
            await self.app(scope, receive, send)
            return
        response = error_code(503, MERGE_PENDING, PENDING_MESSAGE)
        await response(scope, receive, send)


def _exempt(path: str) -> bool:
    return any(path == p or path.startswith(p + "/") for p in EXEMPT)
