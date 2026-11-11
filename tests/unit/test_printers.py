"""Printers: PrusaLink over a fake printer, uploads and renames, recordings and signed links."""

import asyncio
import hashlib
import hmac
import json
import os
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
from pathlib import Path
from types import SimpleNamespace
from typing import Any
from urllib.parse import parse_qs, urlsplit

import httpx
import pytest

from gv_api import printers
from gv_api.auth import AuthService
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.deps import ON_FAILOVER
from gv_api.core.http import ApiError
from gv_api.printers import Printer, PrintersService, Settings
from gv_api.printers import link as link_module
from gv_api.printers.link import Link, LinkError, sanitize_file_name
from gv_api.printers.media import Signer
from gv_api.printers.meta import done_fraction, parse_print_meta, round_to, scale_used
from gv_api.printers.recordings import format_time_left, parse_recording_name, recording_name
from gv_api.printers.schemas import Telemetry
from gv_api.printers.service import AutoRecorder, rename_target
from gv_api.printers.uploads import ChunkError, Uploads

from .test_tasks_router import AUTH, CFG, ping

HOST = "http://printer.test"
SEMI = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "semi")}
REC = "2026-08-06T14-32-05.mp4"


def respond_json(data: Any, status: int = 200) -> httpx.Response:
    return httpx.Response(status, json=data)


class FakePrusa:
    """PrusaLink on a USB drive with one print file, A.BGC shown as a.bgcode."""

    def __init__(self) -> None:
        self.calls: list[str] = []
        self.job: dict[str, Any] | None = None
        self.status: dict[str, Any] = {"printer": {"state": "IDLE", "temp_nozzle": 21.5}}
        self.children: list[dict[str, Any]] = [
            {"name": "A.BGC", "display_name": "a.bgcode", "type": "PRINT_FILE"},
            {"name": "dir", "type": "FOLDER"},
            {"name": "notes.txt"},
        ]
        self.info: dict[str, Any] = {
            "size": 42,
            "m_timestamp": 10,
            "refs": {"download": "/usb/A.BGC"},
        }
        self.command_status = 201
        self.stop_status = 204
        self.download: bytes = b"x" * 42
        self.download_status = 200
        self.gate: asyncio.Event | None = None
        self.active = self.peak = 0

    async def __call__(self, request: httpx.Request) -> httpx.Response:
        path, method = request.url.path, request.method
        if path == "/api/v1/storage":
            usb = {"name": "USB", "path": "/usb/", "available": True, "read_only": False}
            return respond_json({"storage_list": [{**usb, "free_space": 100, "total_space": 200}]})
        if path == "/api/v1/status":
            return respond_json(self.status)
        if path == "/api/printer":
            return respond_json({"telemetry": {"material": "PETG"}})
        if path == "/api/v1/job":
            return httpx.Response(204) if self.job is None else respond_json(self.job)
        if path.startswith("/api/v1/job/"):
            self.calls.append(f"{method} {path}")
            return httpx.Response(self.stop_status)
        if path.startswith("/api/v1/files/usb/"):
            return await self._files(request, path, method)
        if path.startswith("/usb/"):
            self.calls.append(f"{method} {path}")
            return httpx.Response(self.download_status, content=self.download)
        return httpx.Response(404)

    async def _files(self, request: httpx.Request, path: str, method: str) -> httpx.Response:
        self.calls.append(f"{method} {path}")
        if method == "PUT":
            self.calls.append("body " + request.content.decode())
            if self.gate is not None:
                self.active += 1
                self.peak = max(self.peak, self.active)
                await self.gate.wait()
                self.active -= 1
            return httpx.Response(self.command_status)
        if method != "GET":
            return httpx.Response(self.command_status)
        if path == "/api/v1/files/usb/":
            return respond_json({"children": self.children})
        return respond_json(self.info)


@asynccontextmanager
async def client(svc: PrintersService, cfg: Config = CFG) -> AsyncIterator[httpx.AsyncClient]:
    app = create_app(cfg, ping=ping, routers=[printers.router, printers.public_router])
    services = {AuthService: AuthService(cfg), PrintersService: svc}
    app.state.container = SimpleNamespace(services=services, cfg=cfg)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t", headers=SEMI) as c:
        yield c


async def eventually(check: Callable[[], Awaitable[bool]], seconds: float = 3) -> None:
    async with asyncio.timeout(seconds):
        while not await check():  # noqa: ASYNC110 - polls the API as a client would
            await asyncio.sleep(0.02)


# names, metadata and formats


def test_sanitize_file_name() -> None:
    cases = {
        "part.bgcode": "part.bgcode",
        "a/b/../part.gcode": "part.gcode",
        "c:\\x\\LIGHTH~1.BGC": "LIGHTH~1.BGC",
        "..": "",
        ".hidden.gcode": "",
        "notes.txt": "",
        "bad\x00.gcode": "",
        "": "",
        "x" * 250 + ".gcode": "",
    }
    for raw, want in cases.items():
        assert sanitize_file_name(raw) == want, raw


def test_recording_name() -> None:
    at = datetime(2026, 8, 6, 14, 32, 5, tzinfo=UTC)
    assert recording_name(at) == REC
    assert parse_recording_name(REC) == at
    for bad in ("../x.mp4", "2026-08-06T14-32-05.exe", "x.mp4", "2026-13-40T99-99-99.mp4"):
        assert parse_recording_name(bad) is None, bad


def test_format_time_left() -> None:
    assert format_time_left(30) == "<1m"
    assert format_time_left(14 * 60) == "14m"
    assert format_time_left(7 * 3600 + 5 * 60) == "7h 05m"


def test_rename_target() -> None:
    assert rename_target("benchy.bgcode", "boat") == "boat.bgcode"
    assert rename_target("benchy.bgcode", "boat.BGCODE") == "boat.BGCODE"
    assert rename_target("part.gcode", " v2 part ") == "v2 part.gcode"
    for bad in ("", "a/b", "x.gcode", "BENCHY", ".hidden"):
        with pytest.raises(ApiError):
            rename_target("benchy.bgcode", bad)


def test_parse_print_meta() -> None:
    m = parse_print_meta(
        {
            "estimated printing time (normal mode)": "1d 2h 3m",
            "filament_type": "PLA;PETG;PLA",
            "filament used [g]": "10.04, 2.5, 0.00",
        }
    )
    assert (m.print_seconds, m.material, m.filament_grams) == (93780, "PLA, PETG", 12.5)
    assert m.filament_meters is None
    assert parse_print_meta({"estimated_print_time": 90}).print_seconds == 90
    assert parse_print_meta({"print_time": "0", "printing_time": "75"}).print_seconds == 75
    assert parse_print_meta({"filament_used_m": 1.234}).filament_meters == 1.23
    assert parse_print_meta({"filament_used_mm": 40817.15}).filament_meters == 40.82
    empty = parse_print_meta({"filament_type": " ; ", "filament used [g]": "x", "print_time": True})
    assert not empty.has_meta()

    assert round_to(12.25, 1) == 12.3, "half up"
    assert scale_used(10.0, done_fraction("FIN_STOPPED", 50.0, None, None)) == 5.0
    assert scale_used(10.0, done_fraction("FIN_ERROR", None, 900.0, 3600)) == 2.5
    assert scale_used(10.0, done_fraction("FIN_OK", 50.0, 1.0, 10)) == 10.0
    assert scale_used(10.0, done_fraction("PRINTING", 50.0, None, None)) == 10.0
    assert scale_used(10.0, done_fraction("FIN_ERROR", None, None, None)) == 10.0


def test_auto_recorder() -> None:
    def on(state: str) -> Telemetry:
        return Telemetry(configured=True, online=True, state=state)

    a = AutoRecorder()
    assert a.observe(on("IDLE")) == (False, False)
    assert a.observe(on("PRINTING")) == (True, False)
    a.started = True
    assert a.observe(on("PAUSED")) == (False, False), "a pause keeps recording"
    assert a.observe(on("PRINTING")) == (False, False)
    assert a.observe(on("FINISHED")) == (False, True)

    a = AutoRecorder()
    assert a.observe(on("PRINTING")) == (False, False), "a print running at boot is a baseline"
    assert a.observe(on("IDLE")) == (False, False), "it never started this recording"

    a = AutoRecorder()
    a.observe(on("IDLE"))
    a.observe(on("PRINTING"))
    a.started = True
    assert a.observe(Telemetry(configured=True)) == (False, False), "offline changes nothing"
    assert a.observe(on("STOPPED")) == (False, True)


# signed links


def test_signed_media() -> None:
    now = 1_786_000_000.0
    s = Signer(b"secret", lambda: now)
    exp = int(now) + 3600
    sig = s.sign("core-one", REC, exp)
    assert s.verify("core-one", REC, str(exp), sig)
    assert not s.verify("core-one", "2026-08-06T14-32-06.mp4", str(exp), sig)
    assert not s.verify("core-one", REC, "x", sig)
    old = exp - 7200
    assert not s.verify("core-one", REC, str(old), s.sign("core-one", REC, old)), "expired"
    message = f"printer-media:core-one/{REC}/{exp}".encode()
    assert sig == hmac.new(b"secret", message, hashlib.sha256).hexdigest()

    q = parse_qs(urlsplit(s.media_url("core-one", REC)).query)
    assert int(q["exp"][0]) % 3600 == 0
    assert 11 * 3600 < int(q["exp"][0]) - now <= 12 * 3600
    url = s.file_url("core-one", "A B.BGC", "a+b.bgcode")
    assert "name=A+B.BGC&as=a%2Bb.bgcode&exp=" in url


# PrusaLink


async def test_status_maps_the_telemetry(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.status = {
        "printer": {"state": "PRINTING", "temp_nozzle": 215, "target_bed": 60.0, "axis_z": 1.2},
        "job": {"progress": 40, "time_remaining": 600},
    }
    fake.job = {"id": 7, "time_printing": 120, "file": {"name": "A.BGC", "meta": {}}}
    async with client(make_printers(fake)) as c:
        body = (await c.get("/domotics/printers/core-one/status")).json()
    assert body == {
        "configured": True,
        "online": True,
        "state": "PRINTING",
        "temps": {"nozzle": 215, "bedTarget": 60},
        "fans": {},
        "axisZ": 1.2,
        "job": {
            "progress": 40,
            "timeRemaining": 600,
            "timePrinting": 120,
            "fileName": "A.BGC",
            "material": "PETG",
        },
    }


async def test_status_offline_and_unconfigured(
    make_printers: Callable[..., PrintersService],
) -> None:
    async def down(_: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused")

    async with client(make_printers(down)) as c:
        body = (await c.get("/domotics/printers/core-one/status")).json()
    assert body == {"configured": True, "online": False, "temps": {}, "fans": {},
                    "error": "connection refused"}  # fmt: skip
    async with client(make_printers(down, host="")) as c:
        body = (await c.get("/domotics/printers/core-one/status")).json()
    assert body == {"configured": False, "online": False, "temps": {}, "fans": {}}


async def test_digest_auth(make_printers: Callable[..., PrintersService]) -> None:
    seen: list[str] = []

    async def digest(request: httpx.Request) -> httpx.Response:
        header = request.headers.get("authorization", "")
        seen.append(header)
        if not header.startswith("Digest "):
            challenge = 'Digest realm="Printer API", nonce="abc", algorithm=MD5'
            return httpx.Response(401, headers={"WWW-Authenticate": challenge})
        md5 = lambda s: hashlib.md5(s.encode()).hexdigest()  # noqa: E731, S324
        ha1, ha2 = md5("maker:Printer API:pw"), md5("GET:" + request.url.raw_path.decode())
        assert f'response="{md5(ha1 + ":abc:" + ha2)}"' in header
        assert "qop" not in header
        return respond_json({"printer": {"state": "IDLE"}})

    link = make_printers(digest, user="maker", password="pw").unit("core-one")
    assert link is not None
    assert (await link.link.get_json("/api/v1/status"))[0]
    assert (await link.link.get_json("/api/v1/status"))[0]
    assert len(seen) == 3, "the challenge is kept for the next request"


async def test_api_key_header(make_printers: Callable[..., PrintersService]) -> None:
    async def keyed(request: httpx.Request) -> httpx.Response:
        assert request.headers["x-api-key"] == "k"
        return respond_json({})

    un = make_printers(keyed, api_key="k", user="ignored").unit("core-one")
    assert un is not None
    await un.link.get_json("/api/v1/status")


async def test_files_download_and_stop_job(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    async with client(make_printers(fake)) as c:
        res = await c.get("/domotics/printers/core-one/files")
        assert res.status_code == 200
        view = res.json()
        assert view["online"]
        assert len(view["files"]) == 1
        f = view["files"][0]
        assert (f["displayName"], f["size"], f["modifiedAt"]) == ("a.bgcode", 42, 10000)
        assert view["storage"] == {
            "name": "USB",
            "available": True,
            "readOnly": False,
            "freeSpace": 100,
            "totalSpace": 200,
        }
        assert "printSeconds" not in f, "no metadata on the printer, Connect is not configured"
        prefix = "/domotics/printers/core-one/files/download?name=A.BGC&as=a.bgcode&exp="
        assert f["downloadUrl"].startswith(prefix)

        res = await c.get(f["downloadUrl"], headers={"Authorization": ""})
        assert res.status_code == 200
        assert res.content == b"x" * 42
        assert res.headers["content-disposition"] == 'attachment; filename="a.bgcode"'
        assert res.headers["content-length"] == "42"
        assert res.headers["cache-control"] == "no-store"

        tampered = f["downloadUrl"].replace("sig=", "sig=0")
        assert (await c.get(tampered)).status_code == 401
        no_name = f["downloadUrl"].replace("name=A.BGC", "name=x.txt")
        assert (await c.get(no_name)).status_code == 401

        fake.download_status = 404
        res = await c.get(f["downloadUrl"])
        assert (res.status_code, res.json()) == (
            404,
            {"error": "The printer rejected the storage path"},
        )

        res = await c.delete("/domotics/printers/core-one/job")
        assert (res.status_code, res.json()) == (409, {"error": "No print is running"})
        fake.job = {"id": 7}
        res = await c.delete("/domotics/printers/core-one/job")
        assert (res.status_code, res.json()) == (200, {"stopped": True})
        assert "DELETE /api/v1/job/7" in fake.calls
        fake.stop_status = 409
        res = await c.delete("/domotics/printers/core-one/job")
        want = {"error": "The printer will not stop the job in its current state"}
        assert (res.status_code, res.json()) == (409, want)
        fake.stop_status = 401
        assert (await c.delete("/domotics/printers/core-one/job")).status_code == 502

        assert (await c.get("/domotics/printers/nope/status")).status_code == 404
        res = await c.get("/domotics/printers")
        assert res.json() == [{"id": "core-one", "name": "n", "model": "m"}]


async def test_download_needs_prusalink(make_printers: Callable[..., PrintersService]) -> None:
    svc = make_printers(FakePrusa(), host="")
    url = svc.signer.file_url("core-one", "A.BGC", "a.bgcode")
    async with client(svc) as c:
        res = await c.get(url)
        assert (res.status_code, res.json()) == (503, {"error": "PrusaLink not configured"})
        res = await c.get("/domotics/printers/core-one/files")
        assert res.json() == {"online": False, "files": [], "error": "PrusaLink not configured"}


async def test_files_fill_details_per_file(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.children = [
        {"name": "A.BGC", "type": "PRINT_FILE", "size": 9, "ro": True},
        {"name": "B.GCO", "display_name": "b.gco"},
    ]
    fake.info = {"size": 7, "read_only": False, "meta": {"filament_type": "PLA"}}
    async with client(make_printers(fake, storage="/usb/")) as c:
        files = (await c.get("/domotics/printers/core-one/files")).json()["files"]
    assert files[0] == {
        "name": "A.BGC",
        "displayName": "A.BGC",
        "size": 9,
        "readOnly": True,
        "material": "PLA",
        "downloadUrl": files[0]["downloadUrl"],
    }
    assert (files[1]["size"], files[1]["readOnly"]) == (7, False)
    assert "GET /api/v1/files/usb/B.GCO" in fake.calls


async def test_files_offline(make_printers: Callable[..., PrintersService]) -> None:
    async def broken(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500)

    async with client(make_printers(broken)) as c:
        body = (await c.get("/domotics/printers/core-one/files")).json()
    assert body == {"online": False, "files": [], "error": "PrusaLink /api/v1/files/usb/ -> 500"}


async def test_start_print_and_delete_file(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    async with client(make_printers(fake)) as c:
        res = await c.post("/domotics/printers/core-one/files?name=A.BGC")
        assert (res.status_code, res.json()) == (200, {"name": "A.BGC"})
        res = await c.delete("/domotics/printers/core-one/files?name=A.BGC")
        assert res.status_code == 200
        assert "POST /api/v1/files/usb/A.BGC" in fake.calls
        assert "DELETE /api/v1/files/usb/A.BGC" in fake.calls
        assert (await c.post("/domotics/printers/core-one/files?name=x.sh")).status_code == 400
        fake.command_status = 409
        res = await c.post("/domotics/printers/core-one/files?name=A.BGC")
        assert res.json() == {"error": "A print job is already running"}
        fake.command_status = 507
        res = await c.delete("/domotics/printers/core-one/files?name=A.BGC")
        assert res.status_code == 507
        assert res.json()["error"].startswith("No USB drive detected")


# uploads


async def test_append_chunk(tmp_path: Path) -> None:
    u = Uploads(tmp_path)
    assert await u.append_chunk("upload-1", 0, 6, b"abc") == (3, False)
    assert await u.append_chunk("upload-1", 0, 6, b"abc") == (3, False), "a resent chunk"
    with pytest.raises(ChunkError) as out_of_order:
        await u.append_chunk("upload-1", 5, 6, b"x")
    assert (out_of_order.value.status, out_of_order.value.received) == (409, 3)
    assert await u.append_chunk("upload-1", 3, 6, b"def") == (6, True)
    assert u.path("upload-1").read_bytes() == b"abcdef"

    for args, status in (
        ((0, 0, b"a"), 400),
        ((0, 600 << 20, b"a"), 413),
        ((0, 6, b""), 400),
        ((4, 6, b"abc"), 400),
    ):
        with pytest.raises(ChunkError) as bad:
            await u.append_chunk("upload-2", *args)  # type: ignore[arg-type]
        assert bad.value.status == status


def test_upload_registry_sweeps_and_caps(tmp_path: Path) -> None:
    u = Uploads(tmp_path)
    for i in range(40):
        u.start(f"upload-{i:03d}", "core-one", "x.gcode", "", 1)
        u.finish(f"upload-{i:03d}", None)
    assert len(u.active("core-one")) == 32
    assert u.get("upload-039").status == "done"
    assert u.get("upload-000").status == "unknown", "the oldest are dropped first"
    u.finish("missing", None)
    u.set_status("missing", "forwarding")

    stale = tmp_path / "old.part"
    stale.write_bytes(b"x")
    os.utime(stale, (0, 0))
    fresh = tmp_path / "new.part"
    fresh.write_bytes(b"x")
    u.sweep_staged()
    assert not stale.exists()
    assert fresh.exists()


async def test_chunked_upload_is_forwarded(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    h = {"X-File-Name": "part.bgcode", "X-Upload-Id": "upload-12345", "X-Upload-Total": "6"}
    async with client(make_printers(fake)) as c:
        res = await c.patch(
            "/domotics/printers/core-one/files",
            content=b"abc",
            headers={**h, "X-Upload-Offset": "0"},
        )
        assert (res.status_code, res.json()) == (200, {"received": 3, "complete": False})
        res = await c.patch(
            "/domotics/printers/core-one/files",
            content=b"z",
            headers={**h, "X-Upload-Offset": "5"},
        )
        assert (res.status_code, res.json()) == (
            409,
            {"error": "Chunk out of order", "received": 3},
        )
        res = await c.patch(
            "/domotics/printers/core-one/files",
            content=b"def",
            headers={**h, "X-Upload-Offset": "3"},
        )
        assert res.status_code == 202
        assert res.json() == {
            "name": "part.bgcode",
            "uploadId": "upload-12345",
            "received": 6,
            "complete": True,
        }

        async def done() -> bool:
            res = await c.get("/domotics/printers/core-one/files/progress?u=upload-12345")
            return bool(res.json()["status"] == "done")

        await eventually(done)
        res = await c.get("/domotics/printers/core-one/files/progress?u=upload-12345")
        assert res.json() == {"sent": 6, "total": 6, "status": "done"}
        res = await c.get("/domotics/printers/core-one/files/progress")
        assert res.json()["uploads"] == [
            {
                "sent": 6,
                "total": 6,
                "status": "done",
                "uploadId": "upload-12345",
                "name": "part.bgcode",
            }
        ]
        bad = await c.patch(
            "/domotics/printers/core-one/files", content=b"a", headers={**h, "X-Upload-Offset": "x"}
        )
        assert bad.status_code == 400
        assert (await c.get("/domotics/printers/core-one/files/progress?u=bad")).status_code == 400
        res = await c.get("/domotics/printers/core-one/files/progress?u=unknown-1")
        assert res.json() == {"sent": 0, "total": 0, "status": "unknown"}
    assert "body abcdef" in fake.calls
    assert "PUT /api/v1/files/usb/part.bgcode" in fake.calls


async def test_upload_rejects_bad_requests(make_printers: Callable[..., PrintersService]) -> None:
    async with client(make_printers(FakePrusa())) as c:
        url = "/domotics/printers/core-one/files"
        res = await c.put(url, content=b"x", headers={"X-File-Name": "evil.sh"})
        assert res.status_code == 400
        assert (await c.put(url, content=b"x")).json() == {"error": "Missing X-File-Name header"}
        res = await c.put(url, content=b"", headers={"X-File-Name": "a.gcode"})
        assert res.json() == {"error": "Empty file"}
    async with client(make_printers(FakePrusa(), host="")) as c:
        res = await c.put(url, content=b"x", headers={"X-File-Name": "a.gcode"})
        assert res.status_code == 503


async def test_upload_failure_is_reported(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.command_status = 409
    async with client(make_printers(fake)) as c:
        h = {"X-File-Name": "a%20b.gcode", "X-Upload-Id": "upload-abcdef"}
        res = await c.put("/domotics/printers/core-one/files", content=b"data", headers=h)
        assert res.json() == {"name": "a b.gcode", "uploadId": "upload-abcdef"}

        async def failed() -> bool:
            res = await c.get("/domotics/printers/core-one/files/progress?u=upload-abcdef")
            return bool(res.json()["status"] == "error")

        await eventually(failed)
        res = await c.get("/domotics/printers/core-one/files/progress?u=upload-abcdef")
        assert res.json() == {
            "sent": 4,
            "total": 4,
            "status": "error",
            "error": "A file with that name is already on the printer",
            "httpStatus": 409,
        }


async def test_uploads_reach_the_printer_one_at_a_time(
    make_printers: Callable[..., PrintersService],
) -> None:
    fake = FakePrusa()
    fake.gate = asyncio.Event()
    async with client(make_printers(fake)) as c:

        async def progress(upload_id: str) -> str:
            res = await c.get(f"/domotics/printers/core-one/files/progress?u={upload_id}")
            return str(res.json()["status"])

        for n in ("one", "two"):
            h = {"X-File-Name": f"{n}.gcode", "X-Upload-Id": f"upload-{n}"}
            res = await c.put("/domotics/printers/core-one/files", content=b"data", headers=h)
            assert res.status_code == 202

        async def forwarding() -> bool:
            return fake.active == 1

        await eventually(forwarding)
        await asyncio.sleep(0.05)
        assert await progress("upload-one") == "forwarding"
        assert await progress("upload-two") == "queued"
        fake.gate.set()

        async def both_done() -> bool:
            return await progress("upload-two") == "done"

        await eventually(both_done)
    puts = [c for c in fake.calls if c.startswith("PUT")]
    assert puts == ["PUT /api/v1/files/usb/one.gcode", "PUT /api/v1/files/usb/two.gcode"]
    assert fake.peak == 1


async def test_a_full_queue_refuses_more(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.gate = asyncio.Event()
    svc = make_printers(fake)
    un = svc.unit("core-one")
    assert un is not None
    for i in range(32):
        upload_id = f"upload-{i:04d}"
        svc.uploads.path(upload_id).parent.mkdir(parents=True, exist_ok=True)
        svc.uploads.path(upload_id).write_bytes(b"x")
        svc.forward(un, upload_id, "a.gcode", overwrite=False)
    svc.uploads.path("upload-full").write_bytes(b"x")
    with pytest.raises(ApiError) as full:
        svc.forward(un, "upload-full", "a.gcode", overwrite=False)
    assert full.value.status == 503
    assert not svc.uploads.path("upload-full").exists()
    assert svc.uploads.get("upload-full").http_status == 503
    with pytest.raises(RuntimeError):
        svc.forward(un, "upload-none", "a.gcode", overwrite=False)
    fake.gate.set()


# rename


async def test_rename_moves_the_file(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    async with client(make_printers(fake)) as c:
        url = "/domotics/printers/core-one/files/rename"
        res = await c.post(url, json={"name": "A.BGC", "newName": "b"})
        assert res.status_code == 202, res.text
        body = res.json()
        assert body["name"] == "b.bgcode"

        async def done() -> bool:
            res = await c.get(f"/domotics/printers/core-one/files/progress?u={body['uploadId']}")
            return bool(res.json()["status"] == "done")

        await eventually(done)
        uploads = (await c.get("/domotics/printers/core-one/files/progress")).json()["uploads"]
        assert uploads[0]["from"] == "a.bgcode"

        assert (await c.post(url, json={"name": "A.BGC", "newName": "A.bgcode"})).status_code == 400
        assert (await c.post(url, json={"name": "x.txt", "newName": "y"})).status_code == 400
        assert (await c.post(url, content=b"{")).json() == {"error": "Invalid request body"}
    for call in (
        "GET /usb/A.BGC",
        "PUT /api/v1/files/usb/b.bgcode",
        "body " + "x" * 42,
        "DELETE /api/v1/files/usb/A.BGC",
    ):
        assert call in fake.calls


async def rename_error(svc: PrintersService, new_name: str = "b") -> tuple[int, Any]:
    async with client(svc) as c:
        res = await c.post(
            "/domotics/printers/core-one/files/rename", json={"name": "A.BGC", "newName": new_name}
        )
    return res.status_code, res.json()


async def test_rename_refusals(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.info = {**fake.info, "ro": True}
    assert await rename_error(make_printers(fake)) == (409, {"error": "That file is read-only"})

    fake = FakePrusa()
    fake.children.append({"name": "B~1.BGC", "display_name": "b.bgcode"})
    want = (409, {"error": "A file with that name is already on the printer"})
    assert await rename_error(make_printers(fake)) == want

    fake = FakePrusa()
    fake.job = {"file": {"name": "A.BGC", "display_name": "a.bgcode"}}
    assert await rename_error(make_printers(fake)) == (
        409,
        {"error": "That file is currently printing"},
    )

    fake = FakePrusa()
    fake.info = {}
    fake.children = []

    async def gone(request: httpx.Request) -> httpx.Response:
        if request.url.path == "/api/v1/files/usb/A.BGC":
            return httpx.Response(404)
        return await fake(request)

    assert await rename_error(make_printers(gone)) == (
        404,
        {"error": "That file is no longer on the printer"},
    )


async def finished_with(svc: PrintersService, upload_id: str) -> dict[str, Any]:
    async def settled() -> bool:
        return svc.uploads.get(upload_id).status in ("done", "error")

    await eventually(settled)
    return svc.uploads.get(upload_id).model_dump(exclude_none=True)


async def test_rename_transfer_failures(make_printers: Callable[..., PrintersService]) -> None:
    fake = FakePrusa()
    fake.download = b"short"
    svc = make_printers(fake)
    un = svc.unit("core-one")
    assert un is not None
    upload_id, _ = await svc.rename(un, "A.BGC", "b")
    got = await finished_with(svc, upload_id)
    assert got["error"] == "The printer sent an incomplete copy of the file"

    fake = FakePrusa()
    fake.download_status = 500
    svc = make_printers(fake)
    upload_id, _ = await svc.rename(svc.unit("core-one"), "A.BGC", "b")  # type: ignore[arg-type]
    assert (await finished_with(svc, upload_id))["httpStatus"] == 500

    fake = FakePrusa()

    async def no_delete(request: httpx.Request) -> httpx.Response:
        if request.method == "DELETE":
            return httpx.Response(409)
        return await fake(request)

    svc = make_printers(no_delete)
    upload_id, _ = await svc.rename(svc.unit("core-one"), "A.BGC", "b")  # type: ignore[arg-type]
    got = await finished_with(svc, upload_id)
    assert got["error"] == (
        "Saved as b.bgcode, but the original could not be deleted: That file is currently printing"
    )

    async def unreachable(request: httpx.Request) -> httpx.Response:
        if request.url.path.startswith("/usb/"):
            raise httpx.ConnectError("refused")
        return await FakePrusa()(request)

    svc = make_printers(unreachable)
    upload_id, _ = await svc.rename(svc.unit("core-one"), "A.BGC", "b")  # type: ignore[arg-type]
    got = await finished_with(svc, upload_id)
    assert got["error"] == "Could not read the file from the printer: refused"


async def test_transfers_stop_only_when_the_printer_stalls(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(link_module, "TRANSFER_IDLE", 0.3)

    async def serve(reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        request_line = await reader.readline()
        while (await reader.readline()).strip():
            pass
        writer.write(b"HTTP/1.1 200 OK\r\nContent-Length: 10\r\nConnection: close\r\n\r\n")
        if b"SLOW" in request_line:
            for _ in range(10):
                writer.write(b"x")
                await writer.drain()
                await asyncio.sleep(0.1)
        else:
            writer.write(b"x")
            await writer.drain()
            await asyncio.sleep(5)
        writer.close()

    server = await asyncio.start_server(serve, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    link = Link(Printer(id="p", host=f"http://127.0.0.1:{port}"))
    got = bytearray()

    async def write(chunk: bytes) -> None:
        got.extend(chunk)

    try:
        loop = asyncio.get_running_loop()
        start = loop.time()
        await link.download("/usb/SLOW.GCO", 10, write)
        assert got == b"x" * 10, "slow but moving is fine"
        assert loop.time() - start > 0.9

        start = loop.time()
        with pytest.raises(LinkError, match="stopped responding mid-transfer"):
            await link.download("/usb/STUCK.GCO", 10, write)
        assert loop.time() - start < 2
    finally:
        await link.aclose()
        server.close()


# recordings and signed media


async def test_recordings_are_signed_and_served(
    make_printers: Callable[..., PrintersService],
) -> None:
    svc = make_printers(FakePrusa())
    folder = svc.settings.recordings_dir / "core-one"
    folder.mkdir(parents=True)
    (folder / REC).write_bytes(b"0123456789")
    (folder / REC.replace(".mp4", ".jpg")).write_bytes(b"jpg")
    (folder / "notes.txt").write_bytes(b"x")
    async with client(svc) as c:
        view = (await c.get("/domotics/printers/core-one/recordings")).json()
        assert (view["usedBytes"], view["maxBytes"]) == (10, 1 << 30)
        [rec] = view["recordings"]
        assert rec["startedAt"] == "2026-08-06T14:32:05Z"
        assert rec["poster"] == REC.replace(".mp4", ".jpg")
        assert not rec["recording"]
        assert "endedAt" in rec

        res = await c.get(rec["url"], headers={"Range": "bytes=2-4", "Authorization": ""})
        assert (res.status_code, res.content) == (206, b"234")
        assert res.headers["content-range"] == "bytes 2-4/10"
        assert res.headers["content-type"] == "video/mp4"
        assert res.headers["cache-control"] == "private, max-age=31536000, immutable"
        res = await c.get(rec["url"], headers={"Range": "bytes=20-30"})
        assert (res.status_code, res.headers["content-range"]) == (416, "bytes */10")
        res = await c.get(rec["url"] + "&download=1")
        assert (res.status_code, res.content) == (200, b"0123456789")
        assert res.headers["content-disposition"] == f'attachment; filename="core-one_{REC}"'
        poster = await c.get(rec["posterUrl"])
        assert (poster.content, poster.headers["content-type"]) == (b"jpg", "image/jpeg")

        unsigned = f"/domotics/printers/core-one/recordings/{REC}"
        assert (await c.get(unsigned)).status_code == 401
        missing = svc.signer.media_url("core-one", "2026-08-07T10-00-00.mp4")
        assert (await c.get(missing)).json() == {"error": "Recording not found"}
        assert (await c.get(svc.signer.media_url("other", REC))).status_code == 404

        (folder / "2026-08-07T10-00-00.mp4").write_bytes(b"x")
        names = f"name={REC}&name=2026-08-07T10-00-00.mp4&name=nope.mp4"
        res = await c.delete(f"/domotics/printers/core-one/recordings?{names}")
        assert res.status_code == 200
        body = res.json()
        assert sorted(body["deleted"]) == [REC, "2026-08-07T10-00-00.mp4"]
        assert body["failed"] == {"nope.mp4": "Invalid recording name"}
        assert not (folder / "2026-08-07T10-00-00.mp4").exists()
        assert not (folder / REC.replace(".mp4", ".jpg")).exists()

        res = await c.delete(f"/domotics/printers/core-one/recordings?name={REC}")
        assert (res.status_code, res.json()) == (404, {"error": "Recording not found"})
        res = await c.delete("/domotics/printers/core-one/recordings")
        assert res.json() == {"error": "Missing recording name"}


async def test_a_live_recording_is_served_up_to_its_size_now(
    make_printers: Callable[..., PrintersService],
) -> None:
    svc = make_printers(FakePrusa())
    folder = svc.settings.recordings_dir / "core-one"
    folder.mkdir(parents=True)
    (folder / REC).write_bytes(b"0123456789")
    un = svc.unit("core-one")
    assert un is not None
    un.recorder.live_name = lambda: REC  # type: ignore[method-assign]
    async with client(svc) as c:
        [rec] = (await c.get("/domotics/printers/core-one/recordings")).json()["recordings"]
        assert rec["recording"]
        assert "endedAt" not in rec
        res = await c.get(rec["url"])
        assert (res.status_code, res.content) == (200, b"0123456789")
        assert res.headers["cache-control"] == "no-store"
        assert res.headers["content-length"] == "10"
        res = await c.delete(f"/domotics/printers/core-one/recordings?name={REC}")
        want = {"error": "That recording is still running. Stop it first."}
        assert (res.status_code, res.json()) == (409, want)


def test_prune_keeps_the_newest_finished(tmp_path: Path) -> None:
    from gv_api.printers.recordings import Recorder

    settings = Settings(recordings_dir=tmp_path, max_bytes=25)
    p = Printer(id="core-one")
    r = Recorder(settings, p, Link(p))
    folder = tmp_path / "core-one"
    folder.mkdir()
    names = [f"2026-08-0{d}T10-00-00.mp4" for d in range(1, 5)]
    for name in names:
        (folder / name).write_bytes(b"x" * 10)
    r.prune(live=names[3])
    assert sorted(f.name for f in folder.iterdir()) == names[2:]


async def test_camera_url_is_signed(make_printers: Callable[..., PrintersService]) -> None:
    async with client(make_printers(FakePrusa())) as c:
        assert (await c.get("/domotics/printers/core-one/camera/stream")).status_code == 401
        res = await c.get("/domotics/printers/core-one/camera/url")
        assert res.status_code == 200
        assert "/domotics/printers/core-one/camera/stream?exp=" in res.json()["url"]


async def test_recording_action_validation(make_printers: Callable[..., PrintersService]) -> None:
    async with client(make_printers(FakePrusa())) as c:
        res = await c.post("/domotics/printers/core-one/recordings?action=pause")
        assert res.json() == {"error": "Unknown action. Expected start or stop."}
        res = await c.post("/domotics/printers/core-one/recordings?action=stop")
        assert (res.status_code, res.json()) == (409, {"error": "This printer is not recording"})


async def test_routes_answer_503_on_the_failover_side(
    make_printers: Callable[..., PrintersService],
) -> None:
    aws = CFG.model_copy(update={"failover_side": "aws"})
    svc = make_printers(FakePrusa())
    async with client(svc, aws) as c:
        for path in ("/domotics/printers/core-one/status", svc.signer.media_url("core-one", REC)):
            res = await c.get(path)
            assert res.status_code == 503
            assert res.json() == {"error": ON_FAILOVER, "code": "unavailable_on_failover"}


async def test_semi_token_required(make_printers: Callable[..., PrintersService]) -> None:
    async with client(make_printers(FakePrusa())) as c:
        res = await c.get("/domotics/printers", headers={"Authorization": ""})
        assert res.status_code == 401


def test_settings_from_config() -> None:
    cfg = CFG.model_copy(update={"prusalink_host": "http://p", "printer_recordings_max_gb": 1.5})
    s = printers.settings(cfg)
    assert s.printers[0].host == "http://p"
    assert s.max_bytes == int(1.5 * (1 << 30))
    assert s.sign_key == b"secret"


def test_json_body_has_no_nulls() -> None:
    from gv_api.printers.router import send

    body = json.loads(send(200, [Telemetry(configured=False)]).body)
    assert body == [{"configured": False, "online": False, "temps": {}, "fans": {}}]
