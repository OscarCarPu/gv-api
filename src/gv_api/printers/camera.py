"""The printer camera: one ffmpeg reading the RTSP feed into JPEG frames while someone watches.

It starts on demand, keeps the latest frame, restarts if the feed drops, and stops after
IDLE seconds without a viewer.
"""

import asyncio
import contextlib
import time

FPS = "5"
IDLE = 5 * 60.0
MAX_BUFFER = 4_000_000
FRAME_WAIT = 5.0
START_WAIT = 20.0
READ_WAIT = 5.0

SOI = b"\xff\xd8"
EOI = b"\xff\xd9"


class NoFrameError(Exception):
    pass


class Camera:
    def __init__(self, url: str, ffmpeg: str = "ffmpeg", idle: float = IDLE) -> None:
        self._url = url
        self._ffmpeg = ffmpeg
        self._idle = idle
        self._latest: bytes | None = None
        self._seq = 0
        self._changed = asyncio.Event()
        self._last = 0.0
        self._task: asyncio.Task[None] | None = None

    def warm(self) -> None:
        self._last = time.monotonic()
        if self._task is None or self._task.done():
            self._task = asyncio.create_task(self._run(), name="camera")

    async def next(self, seq: int, wait: float) -> tuple[bytes, int]:
        """The first frame newer than `seq`, starting the feed if nobody is watching."""
        self.warm()
        try:
            async with asyncio.timeout(wait):
                while self._latest is None or self._seq == seq:
                    await self._changed.wait()
                frame = self._latest
        except TimeoutError:
            raise NoFrameError from None
        return frame, self._seq

    def idle(self) -> bool:
        return time.monotonic() - self._last >= self._idle

    async def close(self) -> None:
        if self._task is not None:
            self._task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._task

    async def _run(self) -> None:
        while True:
            await self._stream()
            self._latest = None
            if self.idle():
                return
            await asyncio.sleep(1)

    def _publish(self, frame: bytes) -> None:
        self._latest = frame
        self._seq += 1
        changed, self._changed = self._changed, asyncio.Event()
        changed.set()

    async def _stream(self) -> None:
        # fmt: off
        args = [
            "-loglevel", "error", "-rtsp_transport", "tcp", "-probesize", "32768",
            "-analyzeduration", "0", "-i", self._url, "-an", "-vf", "fps=" + FPS,
            "-f", "mjpeg", "-q:v", "6", "pipe:1",
        ]
        # fmt: on
        try:
            proc = await asyncio.create_subprocess_exec(
                self._ffmpeg,
                *args,
                stdin=asyncio.subprocess.DEVNULL,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.DEVNULL,
            )
        except OSError:
            return
        stdout = proc.stdout
        if stdout is None:  # always set: stdout is a pipe
            return
        pending = bytearray()
        try:
            while not self.idle():
                try:
                    data = await asyncio.wait_for(stdout.read(64 << 10), READ_WAIT)
                except TimeoutError:
                    continue
                if not data:
                    return
                pending += data
                while (start := pending.find(SOI)) >= 0 and (
                    end := pending.find(EOI, start + 2)
                ) >= 0:
                    self._publish(bytes(pending[start : end + 2]))
                    del pending[: end + 2]
                if len(pending) > MAX_BUFFER:
                    pending.clear()
        finally:
            if proc.returncode is None:
                with contextlib.suppress(ProcessLookupError):
                    proc.kill()
            await proc.wait()
