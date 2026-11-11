"""PrusaLink: telemetry, job control and the files on the printer, over its local HTTP API."""

import asyncio
import re
import unicodedata
from collections.abc import AsyncIterator, Awaitable, Callable
from pathlib import Path
from typing import Any
from urllib.parse import quote

import anyio
import httpx

from gv_api.core.http import ApiError
from gv_api.printers.config import Printer
from gv_api.printers.meta import parse_print_meta
from gv_api.printers.schemas import Fans, File, FilesView, JobInfo, Storage, Telemetry, Temps

SHORT_TIMEOUT = 4.0
COMMAND_TIMEOUT = 10.0
# A transfer fails once no byte has moved for TRANSFER_IDLE. After a whole upload is sent, the
# printer gets DRAINED_GRACE to answer while it finishes writing to its USB drive.
TRANSFER_IDLE = 90.0
DRAINED_GRACE = 600.0
STALLED = "The printer stopped responding mid-transfer"
TOO_LONG = "The transfer took too long"

ALLOWED_EXTENSIONS = (".bgcode", ".gcode", ".bgc", ".gco", ".g")
MAX_NAME_LENGTH = 255
CHUNK = 64 << 10

ACCEPT_JSON = {"Accept": "application/json"}

Json = dict[str, Any]


class LinkError(Exception):
    """PrusaLink could not be reached, did not answer in time, or answered something unexpected."""


def obj(d: Any, key: str) -> Json:
    v = d.get(key) if isinstance(d, dict) else None
    return v if isinstance(v, dict) else {}


def items(d: Any, key: str) -> list[Json]:
    v = d.get(key) if isinstance(d, dict) else None
    return [x for x in v if isinstance(x, dict)] if isinstance(v, list) else []


def num(d: Json, key: str) -> float | None:
    v = d.get(key)
    return v if isinstance(v, int | float) and not isinstance(v, bool) else None


def integer(d: Json, key: str) -> int | None:
    v = d.get(key)
    return v if isinstance(v, int) and not isinstance(v, bool) else None


def boolean(d: Json, key: str) -> bool | None:
    v = d.get(key)
    return v if isinstance(v, bool) else None


def text(d: Json, key: str) -> str:
    v = d.get(key)
    return v if isinstance(v, str) else ""


def first[T](*values: T | None) -> T | None:
    return next((v for v in values if v is not None), None)


def first_text(*values: str) -> str | None:
    return next((v for v in values if v), None)


def sanitize_file_name(raw: str) -> str:
    """The base name of a printable file, or "" when it is not one."""
    base = re.split(r"[/\\]", raw)[-1].strip()
    if not base or len(base) > MAX_NAME_LENGTH or base.startswith("."):
        return ""
    if any(unicodedata.category(c) == "Cc" for c in base):
        return ""
    return base if base.lower().endswith(ALLOWED_EXTENSIONS) else ""


def file_extension(name: str) -> str:
    lower = name.lower()
    ext = next((e for e in ALLOWED_EXTENSIONS if lower.endswith(e)), "")
    return name[len(name) - len(ext) :] if ext else ""


def file_path(storage: str, name: str) -> str:
    return f"/api/v1/files/{storage}/{quote(name, safe='')}"


def download_path(storage: str, name: str, info: Json) -> str:
    ref = text(obj(info, "refs"), "download")
    return ref if ref.startswith("/") else f"/{storage}/{quote(name, safe='')}"


def transfer_timeout(size: int) -> float:
    return max(3600, size // 4096)


def upstream_status(status: int) -> int:
    """A 401 from the printer is a gateway problem, not the client's credentials."""
    return 502 if status == 401 else status


def upstream_message(status: int, op: str) -> str:
    match status:
        case 409 if op == "upload":
            return "A file with that name is already on the printer"
        case 409 if op == "print":
            return "A print job is already running"
        case 409:
            return "That file is currently printing"
        case 507:
            return (
                "No USB drive detected in the printer. Check that one is inserted and "
                "formatted as FAT32."
            )
        case 404:
            return "The printer rejected the storage path"
        case 401:
            return "PrusaLink rejected the credentials"
    return f"PrusaLink returned {status}"


def stop_message(status: int) -> str:
    match status:
        case 409:
            return "The printer will not stop the job in its current state"
        case 404:
            return "That job had already finished"
        case 401:
            return "PrusaLink rejected the credentials"
    return f"PrusaLink returned {status}"


class FileBody:
    """An upload body that can be sent again (a digest retry re-sends it) and reports progress."""

    def __init__(self, path: Path, on_sent: Callable[[int], None]) -> None:
        self._path = path
        self._on_sent = on_sent

    async def __aiter__(self) -> AsyncIterator[bytes]:
        self._on_sent(0)
        sent = 0
        async with await anyio.open_file(self._path, "rb") as f:
            while chunk := await f.read(CHUNK):
                sent += len(chunk)
                self._on_sent(sent)
                yield chunk


class Link:
    def __init__(self, printer: Printer, transport: httpx.AsyncBaseTransport | None = None):
        self.printer = printer
        headers = {"X-Api-Key": printer.api_key} if printer.api_key else {}
        auth = None
        if printer.user and not printer.api_key:
            auth = httpx.DigestAuth(printer.user, printer.password)
        self._client = httpx.AsyncClient(
            base_url=printer.host,
            headers=headers,
            auth=auth,
            transport=transport,
            timeout=COMMAND_TIMEOUT,
        )

    async def aclose(self) -> None:
        await self._client.aclose()

    async def _request(
        self,
        method: str,
        path: str,
        headers: dict[str, str] | None = None,
        seconds: float = COMMAND_TIMEOUT,
    ) -> httpx.Response:
        try:
            return await self._client.request(method, path, headers=headers, timeout=seconds)
        except httpx.TimeoutException:
            raise LinkError("PrusaLink did not answer in time") from None
        except httpx.HTTPError as err:
            raise LinkError(str(err) or "PrusaLink is unreachable") from err

    async def get_json(self, path: str) -> tuple[bool, Any]:
        """(False, None) when the printer answers 204, as /job does while idle."""
        res = await self._request("GET", path, ACCEPT_JSON, SHORT_TIMEOUT)
        if res.status_code == 204:
            return False, None
        if not res.is_success:
            raise LinkError(f"PrusaLink {path} -> {res.status_code}")
        try:
            return True, res.json()
        except ValueError:
            raise LinkError("PrusaLink returned something unexpected") from None

    async def _attempt(self, path: str) -> tuple[bool, Any] | LinkError:
        try:
            return await self.get_json(path)
        except LinkError as err:
            return err

    async def status(self) -> Telemetry:
        if not self.printer.host:
            return Telemetry(configured=False)
        st, job, legacy = await asyncio.gather(
            self._attempt("/api/v1/status"),
            self._attempt("/api/v1/job"),
            self._attempt("/api/printer"),
        )
        if isinstance(st, LinkError):
            return Telemetry(configured=True, error=str(st))

        raw = st[1] if isinstance(st[1], dict) else {}
        p = obj(raw, "printer")
        t = Telemetry(
            configured=True,
            online=True,
            state=text(p, "state") or None,
            temps=Temps(
                nozzle=num(p, "temp_nozzle"),
                nozzle_target=num(p, "target_nozzle"),
                bed=num(p, "temp_bed"),
                bed_target=num(p, "target_bed"),
                chamber=num(p, "temp_chamber"),
            ),
            fans=Fans(hotend=num(p, "fan_hotend"), print=num(p, "fan_print")),
            axis_z=num(p, "axis_z"),
            speed=num(p, "speed"),
            flow=num(p, "flow"),
        )
        sj = obj(raw, "job")
        j: Json | None = sj if isinstance(raw.get("job"), dict) else None
        if not isinstance(job, LinkError) and job[0]:
            j = obj({"job": job[1]}, "job")
        if j is None:
            return t
        file = obj(j, "file")
        material = ""
        if not isinstance(legacy, LinkError):
            material = text(obj(legacy[1], "telemetry"), "material")
        t.job = JobInfo(
            progress=first(num(j, "progress"), num(sj, "progress")),
            time_remaining=first(num(j, "time_remaining"), num(sj, "time_remaining")),
            time_printing=first(num(j, "time_printing"), num(sj, "time_printing")),
            file_name=first_text(text(file, "display_name"), text(file, "name")),
            material=first_text(text(obj(file, "meta"), "filament_type"), material),
        )
        return t

    async def stop_job(self) -> None:
        res = await self._request("GET", "/api/v1/job", ACCEPT_JSON)
        if res.status_code == 204:
            raise ApiError(409, "No print is running")
        if res.status_code != 200:
            raise LinkError(f"PrusaLink /api/v1/job -> {res.status_code}")
        try:
            job = res.json()
        except ValueError:
            raise LinkError("PrusaLink returned something unexpected") from None
        job_id = integer(job, "id") if isinstance(job, dict) else None
        if job_id is None:
            raise ApiError(409, "No print is running")
        res = await self._request("DELETE", f"/api/v1/job/{job_id}", ACCEPT_JSON)
        if not res.is_success:
            raise ApiError(upstream_status(res.status_code), stop_message(res.status_code))

    async def storage(self) -> tuple[str, Storage | None]:
        """The storage files go to, and its details when the printer lists it."""
        if self.printer.storage:
            return self.printer.storage.strip("/"), None
        try:
            _, raw = await self.get_json("/api/v1/storage")
        except LinkError:
            raw = None
        entries = items(raw, "storage_list")
        name = next(
            (
                text(s, "path").strip("/")
                for s in entries
                if s.get("available") is True and s.get("read_only") is not True and text(s, "path")
            ),
            "usb",
        )
        for s in entries:
            if text(s, "path") and text(s, "path").strip("/") == name:
                return name, Storage(
                    name=text(s, "name") or name,
                    available=s.get("available") is True,
                    read_only=s.get("read_only") is True,
                    free_space=integer(s, "free_space"),
                    total_space=integer(s, "total_space"),
                )
        return name, None

    async def children(self, storage: str) -> list[Json]:
        _, folder = await self.get_json(f"/api/v1/files/{storage}/")
        return items(folder, "children")

    async def files(self) -> FilesView:
        if not self.printer.host:
            return FilesView(online=False, error="PrusaLink not configured")
        storage, info = await self.storage()
        try:
            children = await self.children(storage)
        except LinkError as err:
            return FilesView(online=False, error=str(err))
        files = [
            _file(c)
            for c in children
            if text(c, "type").lower() != "folder" and sanitize_file_name(text(c, "name"))
        ]
        await self._fill(storage, files)
        return FilesView(online=True, storage=info, files=files)

    async def _fill(self, storage: str, files: list[File]) -> None:
        """Ask for each file the folder listing left without a size or metadata."""
        limit = asyncio.Semaphore(4)

        async def one(f: File) -> None:
            async with limit:
                try:
                    ok, info = await self.get_json(file_path(storage, f.name))
                except LinkError:
                    return
            if not ok or not isinstance(info, dict):
                return
            if f.size is None:
                f.size = integer(info, "size")
            if not f.has_meta():
                for key, value in parse_print_meta(obj(info, "meta")):
                    setattr(f, key, value)
            if f.read_only is None:
                f.read_only = first(boolean(info, "ro"), boolean(info, "read_only"))
            if f.modified_at is None and (ts := integer(info, "m_timestamp")) is not None:
                f.modified_at = ts * 1000

        await asyncio.gather(*(one(f) for f in files if f.size is None or not f.has_meta()))

    async def file_info(self, storage: str, name: str) -> Json:
        res = await self._request("GET", file_path(storage, name), ACCEPT_JSON, SHORT_TIMEOUT)
        if res.status_code == 404:
            raise ApiError(404, "That file is no longer on the printer")
        if not res.is_success:
            raise ApiError(
                upstream_status(res.status_code), upstream_message(res.status_code, "read")
            )
        try:
            info = res.json()
        except ValueError:
            info = None
        if not isinstance(info, dict):
            raise ApiError(502, "PrusaLink returned something unexpected")
        return info

    async def command(self, method: str, name: str, extra: dict[str, str] | None = None) -> int:
        storage, _ = await self.storage()
        res = await self._request(
            method, file_path(storage, name), {**ACCEPT_JSON, **(extra or {})}
        )
        return res.status_code

    async def printing_file(self) -> tuple[str, str]:
        ok, job = await self.get_json("/api/v1/job")
        file = obj(job, "file") if ok else {}
        return text(file, "name"), text(file, "display_name")

    async def upload(
        self, name: str, path: Path, size: int, overwrite: bool, on_sent: Callable[[int], None]
    ) -> int:
        storage, _ = await self.storage()
        headers = {
            **ACCEPT_JSON,
            "Content-Type": "application/octet-stream",
            "Content-Length": str(size),
            "Print-After-Upload": "?0",
            "Overwrite": "?1" if overwrite else "?0",
        }
        timeout = httpx.Timeout(COMMAND_TIMEOUT, read=DRAINED_GRACE, write=TRANSFER_IDLE)
        try:
            async with asyncio.timeout(transfer_timeout(size)):
                res = await self._client.put(
                    file_path(storage, name),
                    content=FileBody(path, on_sent),
                    headers=headers,
                    timeout=timeout,
                )
        except httpx.TimeoutException:
            raise LinkError(STALLED) from None
        except TimeoutError:
            raise LinkError(TOO_LONG) from None
        except httpx.HTTPError as err:
            raise LinkError(str(err) or "PrusaLink is unreachable") from err
        return res.status_code

    async def open_download(self, path: str) -> httpx.Response:
        """A streaming GET of a file; the caller reads it with `chunks` and closes it."""
        timeout = httpx.Timeout(COMMAND_TIMEOUT, read=TRANSFER_IDLE)
        request = self._client.build_request("GET", path, timeout=timeout)
        try:
            res = await self._client.send(request, stream=True)
        except httpx.TimeoutException:
            raise LinkError(STALLED) from None
        except httpx.HTTPError as err:
            raise LinkError(str(err) or "PrusaLink is unreachable") from err
        if not res.is_success:
            await res.aclose()
            raise ApiError(
                upstream_status(res.status_code), upstream_message(res.status_code, "download")
            )
        return res

    async def download(
        self, path: str, size: int, write: Callable[[bytes], Awaitable[None]]
    ) -> None:
        try:
            async with asyncio.timeout(transfer_timeout(size)):
                res = await self.open_download(path)
                try:
                    async for chunk in chunks(res):
                        await write(chunk)
                finally:
                    await res.aclose()
        except TimeoutError:
            raise LinkError(TOO_LONG) from None


async def chunks(res: httpx.Response) -> AsyncIterator[bytes]:
    try:
        async for chunk in res.aiter_bytes():
            yield chunk
    except httpx.TimeoutException:
        raise LinkError(STALLED) from None
    except httpx.HTTPError as err:
        raise LinkError(str(err) or "The printer closed the transfer") from err


def _file(c: Json) -> File:
    ts = integer(c, "m_timestamp")
    return File(
        name=text(c, "name"),
        display_name=first_text(text(c, "display_name"), text(c, "name")) or "",
        size=integer(c, "size"),
        read_only=first(boolean(c, "ro"), boolean(c, "read_only")),
        modified_at=ts * 1000 if ts is not None else None,
        **dict(parse_print_meta(obj(c, "meta"))),
    )
