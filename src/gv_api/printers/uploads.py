"""Uploads staged on disk before they go to the printer, and the state of each transfer.

A large file arrives in PATCH chunks (`X-Upload-Id`, `X-Upload-Offset`, `X-Upload-Total`): a chunk
already received is acknowledged again, and one that skips ahead is a 409 carrying `received`, so
the client resumes from there. Transfers to a printer then run one at a time (see the service).
"""

import re
import time
from collections.abc import AsyncIterable
from dataclasses import dataclass, field
from pathlib import Path

import anyio

from gv_api.core.http import ApiError
from gv_api.printers.schemas import ActiveUpload, UploadState

MAX_UPLOAD_BYTES = 512 << 20
STALE_STAGING = 3600.0
FORWARDING_TTL = 30 * 60.0
SETTLED_TTL = 2 * 60.0
MAX_ENTRIES = 32

QUEUED = "queued"
DOWNLOADING = "downloading"
FORWARDING = "forwarding"

UPLOAD_ID = re.compile(r"[A-Za-z0-9-]{8,64}")
_BYTES = re.compile(r"[0-9]{1,15}")


def valid_upload_id(raw: str) -> bool:
    return UPLOAD_ID.fullmatch(raw) is not None


def parse_byte_header(raw: str) -> int | None:
    return int(raw) if _BYTES.fullmatch(raw) else None


class ChunkError(Exception):
    def __init__(self, message: str, status: int, received: int = 0) -> None:
        super().__init__(message)
        self.message = message
        self.status = status
        self.received = received


class TooLargeError(Exception):
    pass


@dataclass
class Entry:
    printer_id: str
    name: str
    from_: str
    state: UploadState
    updated: float = field(default_factory=time.monotonic)


class Uploads:
    def __init__(self, directory: Path) -> None:
        self.dir = directory
        self._entries: dict[str, Entry] = {}

    def path(self, upload_id: str) -> Path:
        return self.dir / (upload_id + ".part")

    def staged(self, upload_id: str) -> int:
        try:
            return self.path(upload_id).stat().st_size
        except OSError:
            return 0

    async def stage_whole(self, upload_id: str, body: AsyncIterable[bytes], limit: int) -> int:
        await anyio.Path(self.dir).mkdir(parents=True, exist_ok=True)
        path = self.path(upload_id)
        size = 0
        try:
            async with await anyio.open_file(path, "wb") as f:
                async for chunk in body:
                    size += len(chunk)
                    if size > limit:
                        raise TooLargeError
                    await f.write(chunk)
        except BaseException:
            path.unlink(missing_ok=True)
            raise
        return size

    async def append_chunk(
        self, upload_id: str, offset: int, total: int, chunk: bytes
    ) -> tuple[int, bool]:
        """(bytes received so far, whether the file is complete)."""
        if total <= 0:
            raise ChunkError("Missing upload total", 400)
        if total > MAX_UPLOAD_BYTES:
            raise ChunkError("File is too large", 413)
        if not chunk:
            raise ChunkError("Empty chunk", 400)
        if offset + len(chunk) > total:
            raise ChunkError("Chunk runs past the declared file size", 400)
        await anyio.Path(self.dir).mkdir(parents=True, exist_ok=True)
        have = self.staged(upload_id)
        if offset + len(chunk) <= have:
            return have, have == total
        if offset != have:
            raise ChunkError("Chunk out of order", 409, have)
        async with await anyio.open_file(self.path(upload_id), "ab") as f:
            await f.write(chunk)
        received = have + len(chunk)
        return received, received == total

    def sweep_staged(self) -> None:
        """Drop staged files of uploads abandoned for over an hour."""
        for part in self.dir.glob("*.part"):
            try:
                if time.time() - part.stat().st_mtime > STALE_STAGING:
                    part.unlink()
            except OSError:
                continue

    def _sweep(self) -> None:
        now = time.monotonic()
        for upload_id, e in list(self._entries.items()):
            if e.state.status == QUEUED:
                continue
            ttl = FORWARDING_TTL if e.state.status in (DOWNLOADING, FORWARDING) else SETTLED_TTL
            if now - e.updated > ttl:
                del self._entries[upload_id]
        while len(self._entries) > MAX_ENTRIES:
            oldest = min(self._entries, key=lambda k: self._entries[k].updated)
            del self._entries[oldest]

    def start(self, upload_id: str, printer_id: str, name: str, from_: str, total: int) -> None:
        self._sweep()
        self._entries[upload_id] = Entry(
            printer_id, name, from_, UploadState(total=total, status=QUEUED)
        )

    def set_status(self, upload_id: str, status: str, total: int = 0) -> None:
        if e := self._entries.get(upload_id):
            e.state.status, e.state.sent, e.updated = status, 0, time.monotonic()
            if total > 0:
                e.state.total = total

    def set_sent(self, upload_id: str, sent: int) -> None:
        e = self._entries.get(upload_id)
        if e is not None and e.state.status in (FORWARDING, DOWNLOADING):
            e.state.sent, e.updated = sent, time.monotonic()

    def finish(self, upload_id: str, fail: ApiError | None) -> None:
        e = self._entries.get(upload_id)
        if e is None:
            return
        e.updated = time.monotonic()
        if fail is None:
            e.state.status, e.state.sent = "done", e.state.total
        else:
            e.state.status, e.state.error, e.state.http_status = "error", fail.message, fail.status

    def get(self, upload_id: str) -> UploadState:
        e = self._entries.get(upload_id)
        return e.state if e is not None else UploadState(status="unknown")

    def active(self, printer_id: str) -> list[ActiveUpload]:
        self._sweep()
        mine = sorted(
            ((k, e) for k, e in self._entries.items() if e.printer_id == printer_id),
            key=lambda kv: kv[1].updated,
        )
        return [
            ActiveUpload.model_validate(
                {**dict(e.state), "upload_id": k, "name": e.name, "from": e.from_ or None}
            )
            for k, e in mine
        ]
