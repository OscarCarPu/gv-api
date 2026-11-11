"""Recordings of the printer camera: one ffmpeg writing a fragmented MP4 per recording, under
`<recordings dir>/<printer>/<UTC start>.mp4`, with a JPEG poster written once it ends.

Recordings are capped in length and size, and the oldest are pruned when the folder is over
its budget. The overlay shows the clock, the Z height and the time left, read from PrusaLink.
"""

import asyncio
import contextlib
import logging
import os
import re
import tempfile
from collections.abc import Coroutine
from dataclasses import dataclass, field
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any

from gv_api.core.http import ApiError
from gv_api.printers.config import Printer, Settings
from gv_api.printers.link import Link
from gv_api.printers.schemas import Recording, Telemetry

log = logging.getLogger("gv_api")

OVERLAY_POLL = 5.0
STOP_GRACE = 5.0
STOP_KILL = 3.0
START_PROBE = 8.0
STDERR_KEEP = 2000
NAME_FORMAT = "%Y-%m-%dT%H-%M-%S"
DEFAULT_FONT = "/usr/share/fonts/dejavu/DejaVuSansMono-Bold.ttf"

_NAME = re.compile(r"(\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2})\.(mp4|jpg)")
_FILTER_SPECIAL = re.compile(r"([\\':,;\[\]])")


def recording_name(at: datetime) -> str:
    return at.astimezone(UTC).strftime(NAME_FORMAT) + ".mp4"


def parse_recording_name(name: str) -> datetime | None:
    m = _NAME.fullmatch(name)
    if m is None:
        return None
    try:
        return datetime.strptime(m[1], NAME_FORMAT).replace(tzinfo=UTC)
    except ValueError:
        return None


def poster_name(mp4: str) -> str:
    return mp4.removesuffix(".mp4") + ".jpg"


def rfc3339(t: datetime) -> str:
    return t.astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def format_time_left(seconds: float) -> str:
    minutes = int(seconds) // 60
    if minutes < 1:
        return "<1m"
    if hours := minutes // 60:
        return f"{hours}h {minutes % 60:02d}m"
    return f"{minutes}m"


def overlay_text(t: Telemetry | None) -> str:
    z = left = "—"
    if t is not None and t.axis_z is not None:
        z = f"{t.axis_z:.2f} mm"
    if t is not None and t.job is not None and t.job.time_remaining is not None:
        left = format_time_left(t.job.time_remaining)
    return "REC  %{pts:gmtime:0:%H\\:%M\\:%S}\nZ    " + z + "\nLEFT " + left


def filter_escape(value: str) -> str:
    """Escapes a value for an ffmpeg filter option."""
    return _FILTER_SPECIAL.sub(r"\\\1", value)


def write_overlay(path: Path, text: str) -> None:
    """Atomically, so ffmpeg never reloads half a file."""
    tmp = path.with_name(path.name + ".tmp")
    with contextlib.suppress(OSError):
        tmp.write_text(text)
        tmp.replace(path)


@dataclass
class Active:
    name: str
    path: Path
    proc: asyncio.subprocess.Process
    stderr: bytearray = field(default_factory=bytearray)
    done: asyncio.Event = field(default_factory=asyncio.Event)
    stopping: bool = False

    def last_error(self) -> str:
        lines = self.stderr.decode(errors="replace").strip().split("\n")
        return lines[-1].strip()[:200]


class Recorder:
    def __init__(self, settings: Settings, printer: Printer, link: Link) -> None:
        self._cfg = settings
        self._printer = printer
        self._link = link
        self._lock = asyncio.Lock()
        self._active: Active | None = None
        self._tasks: set[asyncio.Task[None]] = set()

    @property
    def dir(self) -> Path:
        return self._cfg.recordings_dir / self._printer.id

    def live_name(self) -> str:
        return self._active.name if self._active is not None else ""

    def scan(self, live: str) -> list[Recording]:
        try:
            entries = list(os.scandir(self.dir))
        except OSError:
            entries = []
        posters = {e.name for e in entries if e.name.endswith(".jpg")}
        out = []
        for e in entries:
            started = parse_recording_name(e.name)
            if started is None or not e.name.endswith(".mp4"):
                continue
            try:
                if not e.is_file(follow_symlinks=False):
                    continue
                st = e.stat()
            except OSError:
                continue
            rec = Recording(
                name=e.name,
                started_at=rfc3339(started),
                duration_ms=0,
                size_bytes=st.st_size,
                recording=e.name == live,
            )
            end = datetime.fromtimestamp(st.st_mtime, UTC)
            if rec.recording:
                end = datetime.now(UTC)
            else:
                rec.ended_at = rfc3339(end)
            rec.duration_ms = max(0, int((end - started) / timedelta(milliseconds=1)))
            if poster_name(e.name) in posters:
                rec.poster = poster_name(e.name)
            out.append(rec)
        out.sort(key=lambda r: r.name, reverse=True)
        return out

    def _remove_files(self, name: str) -> None:
        for n in (name, poster_name(name)):
            (self.dir / n).unlink(missing_ok=True)

    def prune(self, live: str) -> None:
        """Delete the oldest finished recordings until the folder fits its budget, always keeping
        the newest finished one."""
        recordings = self.scan(live)
        total = sum(r.size_bytes for r in recordings)
        prunable = [r for r in reversed(recordings) if not r.recording][:-1]
        for r in prunable:
            if total <= self._cfg.max_bytes:
                return
            self._remove_files(r.name)
            total -= r.size_bytes

    def _free_name(self) -> tuple[str, datetime]:
        at = datetime.now(UTC)
        for _ in range(60):
            name = recording_name(at)
            if not (self.dir / name).exists():
                return name, at
            at += timedelta(seconds=1)
        raise ApiError(502, "Could not allocate a recording file name")

    async def start(self) -> Recording:
        async with self._lock:
            if self._active is not None:
                raise ApiError(409, "This printer is already recording")
            try:
                self.dir.mkdir(parents=True, exist_ok=True)
            except OSError as err:
                raise ApiError(502, f"Recordings folder is not writable: {err}") from err
            self.prune("")
            name, at = self._free_name()
            path = self.dir / name
            overlay = None
            if self._cfg.overlay:
                overlay = (
                    Path(tempfile.gettempdir()) / f"gv-recording-overlay-{self._printer.id}.txt"
                )
                write_overlay(overlay, overlay_text(None))
            # fmt: off
            args = [
                "-loglevel", "error", "-rtsp_transport", "tcp", "-i", self._printer.rtsp, "-an",
                *self._video_args(overlay),
                "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+default_base_moof",
                "-t", str(self._cfg.max_minutes * 60), "-fs", str(self._cfg.max_bytes),
                str(path),
            ]
            # fmt: on
            try:
                proc = await asyncio.create_subprocess_exec(
                    self._cfg.ffmpeg,
                    *args,
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.DEVNULL,
                    stderr=asyncio.subprocess.PIPE,
                )
            except OSError as err:
                _remove_overlay(overlay)
                raise ApiError(502, str(err)) from err
            a = Active(name, path, proc)
            self._active = a
            self._spawn(self._supervise(a, overlay))

        deadline = asyncio.get_running_loop().time() + START_PROBE
        while not a.done.is_set() and _size(path) == 0:
            if asyncio.get_running_loop().time() > deadline:
                break
            with contextlib.suppress(TimeoutError):
                await asyncio.wait_for(a.done.wait(), 0.15)
        if a.done.is_set():
            self._remove_files(name)
            raise ApiError(502, a.last_error() or "ffmpeg stopped immediately")
        return Recording(
            name=name,
            started_at=rfc3339(at),
            duration_ms=max(0, int((datetime.now(UTC) - at) / timedelta(milliseconds=1))),
            size_bytes=_size(path),
            recording=True,
        )

    def _spawn(self, coro: Coroutine[Any, Any, None]) -> None:
        task = asyncio.create_task(coro)
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)

    async def _supervise(self, a: Active, overlay: Path | None) -> None:
        poll = self._spawn_overlay(overlay)
        try:
            if a.proc.stderr is not None:
                while chunk := await a.proc.stderr.read(4096):
                    a.stderr += chunk
                    del a.stderr[:-STDERR_KEEP]
            await a.proc.wait()
        finally:
            if poll is not None:
                poll.cancel()
            _remove_overlay(overlay)
            if self._active is a:
                self._active = None
            a.done.set()
        await self._make_poster(a.path)
        self.prune(self.live_name())

    def _spawn_overlay(self, overlay: Path | None) -> asyncio.Task[None] | None:
        if overlay is None:
            return None
        return asyncio.create_task(self._poll_overlay(overlay))

    async def _poll_overlay(self, overlay: Path) -> None:
        while True:
            t = await self._link.status()
            write_overlay(overlay, overlay_text(t if t.online else None))
            await asyncio.sleep(OVERLAY_POLL)

    def _video_args(self, textfile: Path | None) -> list[str]:
        codec = self._cfg.video_codec
        if textfile is None:
            return ["-c:v", codec]
        font = self._cfg.font or DEFAULT_FONT
        face = "fontfile=" + filter_escape(font) if Path(font).exists() else "font=monospace"
        drawtext = ":".join(
            [
                "drawtext=" + face,
                "textfile=" + filter_escape(str(textfile)),
                "reload=25",
                "x=16:y=16:fontsize=26:line_spacing=6:fontcolor=white",
                "box=1:boxcolor=black@0.55:boxborderw=10",
            ]
        )
        if codec == "copy":
            codec = "libx264"
        # fmt: off
        return [
            "-vf", drawtext, "-c:v", codec, "-preset", "ultrafast", "-crf", "26",
            "-g", "50", "-pix_fmt", "yuv420p", "-threads", "2",
        ]
        # fmt: on

    async def _make_poster(self, mp4: Path) -> None:
        out = mp4.with_suffix(".jpg")
        for seek in (["-ss", "1"], []):
            args = ["-loglevel", "error", *seek, "-i", str(mp4)]
            args += ["-frames:v", "1", "-q:v", "5", "-y", str(out)]
            try:
                proc = await asyncio.create_subprocess_exec(
                    self._cfg.ffmpeg,
                    *args,
                    stdin=asyncio.subprocess.DEVNULL,
                    stdout=asyncio.subprocess.DEVNULL,
                    stderr=asyncio.subprocess.DEVNULL,
                )
                await proc.wait()
            except OSError:
                break
            if _size(out) > 0:
                return
        out.unlink(missing_ok=True)

    async def stop(self) -> None:
        a = self._active
        if a is None:
            raise ApiError(409, "This printer is not recording")
        if not a.stopping and a.proc.stdin is not None:
            a.stopping = True
            with contextlib.suppress(OSError):
                a.proc.stdin.write(b"q")
                a.proc.stdin.close()
        for grace, signal in ((STOP_GRACE, a.proc.terminate), (STOP_KILL, a.proc.kill)):
            with contextlib.suppress(TimeoutError):
                await asyncio.wait_for(a.done.wait(), grace)
                return
            with contextlib.suppress(ProcessLookupError):
                signal()
        await a.done.wait()

    def remove(self, name: str) -> None:
        if parse_recording_name(name) is None:
            raise ApiError(400, "Invalid recording name")
        if self.live_name() == name:
            raise ApiError(409, "That recording is still running. Stop it first.")
        if not (self.dir / name).exists():
            raise ApiError(404, "Recording not found")
        self._remove_files(name)

    def open(self, name: str) -> Path | None:
        if parse_recording_name(name) is None:
            return None
        path = self.dir / name
        return path if path.is_file() and not path.is_symlink() else None

    async def close(self) -> None:
        if self._active is not None:
            await self.stop()
        for task in list(self._tasks):
            task.cancel()
        await asyncio.gather(*self._tasks, return_exceptions=True)


def _remove_overlay(overlay: Path | None) -> None:
    if overlay is not None:
        overlay.unlink(missing_ok=True)


def _size(path: Path) -> int:
    try:
        return path.stat().st_size
    except OSError:
        return 0
