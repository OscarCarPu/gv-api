"""The house 3D printers (Domotics): PrusaLink telemetry, job control and files, Prusa Connect
history, the camera, and recordings made on this server."""

import asyncio
import contextlib
import logging
import secrets
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from pathlib import Path

import anyio
import httpx

from gv_api.core.http import ApiError
from gv_api.printers.camera import Camera
from gv_api.printers.config import Printer, Settings
from gv_api.printers.connect import ConnectClient
from gv_api.printers.link import (
    Link,
    LinkError,
    boolean,
    download_path,
    file_extension,
    first,
    first_text,
    integer,
    sanitize_file_name,
    text,
    upstream_message,
)
from gv_api.printers.media import Signer
from gv_api.printers.recordings import Recorder
from gv_api.printers.schemas import PublicPrinter, RecordingsView, Telemetry
from gv_api.printers.uploads import DOWNLOADING, FORWARDING, MAX_ENTRIES, Uploads

log = logging.getLogger("gv_api")

AUTO_RECORD_POLL = 5.0

QUEUE_FULL = ApiError(503, "Too many transfers queued for this printer. Try again shortly.")

Job = Callable[[], Awaitable[None]]


def new_upload_id() -> str:
    return secrets.token_hex(16)


def rename_target(display: str, requested: str) -> str:
    """The new name for a file shown as `display`; the extension is kept or added."""
    req = requested.strip()
    if not req or "/" in req or "\\" in req:
        raise ApiError(400, "Invalid file name")
    ext = file_extension(display)
    got = file_extension(req)
    if not got:
        req += ext
    elif got.lower() != ext.lower():
        raise ApiError(400, f"Keep the {ext} extension")
    name = sanitize_file_name(req)
    if not name:
        raise ApiError(400, "Invalid file name")
    if name.lower() == display.lower():
        raise ApiError(400, "That is already the file's name")
    return name


@dataclass
class Unit:
    """One printer and everything that talks to it."""

    printer: Printer
    link: Link
    connect: ConnectClient
    camera: Camera
    recorder: Recorder
    jobs: asyncio.Queue[Job] = field(default_factory=lambda: asyncio.Queue(MAX_ENTRIES))
    worker: asyncio.Task[None] | None = None

    async def work(self) -> None:
        while True:
            job = await self.jobs.get()
            try:
                await job()
            except Exception:
                log.exception("printers: transfer failed", extra={"printer": self.printer.id})


@dataclass
class AutoRecorder:
    """Decides, one reading at a time, when to start and stop a recording on its own."""

    printing: bool = False
    known: bool = False
    started: bool = False

    def observe(self, t: Telemetry) -> tuple[bool, bool]:
        """(start, stop). The first reading only sets the baseline, so a restart mid-print does
        not record half a print, and an offline reading changes nothing, so a dropped poll does
        not cut a recording short."""
        if not t.online:
            return False, False
        now = (t.state or "").upper() in ("PRINTING", "PAUSED")
        was, known = self.printing, self.known
        self.printing, self.known = now, True
        if not known:
            return False, False
        return now and not was, not now and was and self.started


class PrintersService:
    def __init__(
        self, settings: Settings, transport: httpx.AsyncBaseTransport | None = None
    ) -> None:
        self.settings = settings
        self.signer = Signer(settings.sign_key)
        self.uploads = Uploads(settings.uploads_dir)
        self._units: dict[str, Unit] = {}
        for p in settings.printers:
            link = Link(p, transport)
            self._units[p.id] = Unit(
                printer=p,
                link=link,
                connect=ConnectClient(p, transport),
                camera=Camera(p.rtsp, settings.ffmpeg),
                recorder=Recorder(settings, p, link),
            )

    def list(self) -> list[PublicPrinter]:
        return [PublicPrinter(id=p.id, name=p.name, model=p.model) for p in self.settings.printers]

    def unit(self, printer_id: str) -> Unit | None:
        return self._units.get(printer_id)

    def recordings(self, un: Unit) -> RecordingsView:
        recordings = un.recorder.scan(un.recorder.live_name())
        for r in recordings:
            r.url = self.signer.media_url(un.printer.id, r.name)
            if r.poster:
                r.poster_url = self.signer.media_url(un.printer.id, r.poster)
        return RecordingsView(
            recordings=recordings,
            used_bytes=sum(r.size_bytes for r in recordings),
            max_bytes=self.settings.max_bytes,
        )

    # transfers: one at a time per printer, in arrival order

    def _enqueue(self, un: Unit, upload_id: str, job: Job) -> None:
        try:
            un.jobs.put_nowait(job)
        except asyncio.QueueFull:
            self.uploads.finish(upload_id, QUEUE_FULL)
            raise QUEUE_FULL from None
        if un.worker is None or un.worker.done():
            un.worker = asyncio.create_task(un.work(), name=f"printer-{un.printer.id}")

    def forward(self, un: Unit, upload_id: str, name: str, overwrite: bool) -> None:
        """Queues a fully staged upload for the printer."""
        path = self.uploads.path(upload_id)
        size = self.uploads.staged(upload_id)
        if size == 0:
            raise RuntimeError("nothing staged")
        self.uploads.start(upload_id, un.printer.id, name, "", size)

        async def job() -> None:
            try:
                self.uploads.set_status(upload_id, FORWARDING)
                fail = await self._push(un, upload_id, name, path, size, overwrite)
                self.uploads.finish(upload_id, fail)
            finally:
                path.unlink(missing_ok=True)

        try:
            self._enqueue(un, upload_id, job)
        except ApiError:
            path.unlink(missing_ok=True)
            raise

    async def _push(
        self, un: Unit, upload_id: str, name: str, path: Path, size: int, overwrite: bool
    ) -> ApiError | None:
        def sent(n: int) -> None:
            self.uploads.set_sent(upload_id, n)

        try:
            status = await un.link.upload(name, path, size, overwrite, sent)
        except (LinkError, OSError) as err:
            return ApiError(502, str(err))
        if not 200 <= status <= 299:
            return ApiError(status, upstream_message(status, "upload"))
        return None

    async def rename(self, un: Unit, name: str, requested: str) -> tuple[str, str]:
        """Queues a rename, which PrusaLink cannot do: download, upload as the new name, delete.
        Returns (upload id, new name)."""
        storage, _ = await un.link.storage()
        info = await un.link.file_info(storage, name)
        if first(boolean(info, "ro"), boolean(info, "read_only")):
            raise ApiError(409, "That file is read-only")
        children = await un.link.children(storage)
        display = text(info, "display_name") or next(
            (text(c, "display_name") for c in children if text(c, "name") == name), ""
        )
        display = first_text(display, name) or name
        new_name = rename_target(display, requested)
        taken = {text(c, k).lower() for c in children for k in ("name", "display_name")}
        if new_name.lower() in taken:
            raise ApiError(409, "A file with that name is already on the printer")
        short, long = await un.link.printing_file()
        if (short and short.lower() == name.lower()) or (long and long.lower() == display.lower()):
            raise ApiError(409, "That file is currently printing")

        size = integer(info, "size") or 0
        src = download_path(storage, name, info)
        upload_id = new_upload_id()
        self.uploads.start(upload_id, un.printer.id, new_name, display, size)

        async def job() -> None:
            path = self.uploads.path(upload_id)
            try:
                fail = await self._move(un, upload_id, name, new_name, src, path, size)
                self.uploads.finish(upload_id, fail)
            finally:
                path.unlink(missing_ok=True)

        self._enqueue(un, upload_id, job)
        return upload_id, new_name

    async def _move(
        self, un: Unit, upload_id: str, name: str, new_name: str, src: str, path: Path, size: int
    ) -> ApiError | None:
        self.uploads.set_status(upload_id, DOWNLOADING)
        if fail := await self._fetch(un, upload_id, src, path, size):
            return fail
        staged = self.uploads.staged(upload_id)
        if size > 0 and staged != size:
            return ApiError(502, "The printer sent an incomplete copy of the file")
        self.uploads.set_status(upload_id, FORWARDING, staged)
        if fail := await self._push(un, upload_id, new_name, path, staged, overwrite=False):
            return fail
        try:
            status = await un.link.command("DELETE", name, {"Force": "?0"})
            why = None if 200 <= status <= 299 else upstream_message(status, "delete")
        except LinkError as err:
            why = str(err)
        if why is not None:
            return ApiError(
                502, f"Saved as {new_name}, but the original could not be deleted: {why}"
            )
        return None

    async def _fetch(
        self, un: Unit, upload_id: str, src: str, path: Path, size: int
    ) -> ApiError | None:
        try:
            await anyio.Path(self.uploads.dir).mkdir(parents=True, exist_ok=True)
            f = await anyio.open_file(path, "wb")
        except OSError:
            return ApiError(500, "Could not stage the file")
        written = 0

        async def write(chunk: bytes) -> None:
            nonlocal written
            await f.write(chunk)
            written += len(chunk)
            self.uploads.set_sent(upload_id, written)

        try:
            async with f:
                await un.link.download(src, size, write)
        except ApiError as err:
            return err
        except (LinkError, OSError) as err:
            return ApiError(502, f"Could not read the file from the printer: {err}")
        return None

    # auto-record

    async def run(self) -> None:
        """Records every print on its own, on each printer with PrusaLink and a camera."""
        units = [u for u in self._units.values() if u.printer.host and u.printer.rtsp]
        await asyncio.gather(*(self._watch(u) for u in units))

    async def _watch(self, un: Unit) -> None:
        auto = AutoRecorder()
        while True:
            start, stop = auto.observe(await un.link.status())
            if start:
                try:
                    await un.recorder.start()
                    auto.started = True
                except ApiError as err:
                    log.warning(
                        "printers: auto-record did not start",
                        extra={"printer": un.printer.id, "error": err.message},
                    )
            if stop:
                auto.started = False
                try:
                    await un.recorder.stop()
                except ApiError as err:
                    log.info(
                        "printers: auto-record already stopped",
                        extra={"printer": un.printer.id, "error": err.message},
                    )
            await asyncio.sleep(AUTO_RECORD_POLL)

    async def close(self) -> None:
        for un in self._units.values():
            if un.worker is not None:
                un.worker.cancel()
                with contextlib.suppress(asyncio.CancelledError):
                    await un.worker
            await un.camera.close()
            await un.recorder.close()
            await un.link.aclose()
            await un.connect.aclose()
