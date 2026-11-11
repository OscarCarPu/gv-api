"""The camera, recordings and auto-record, against a fake ffmpeg; and signed-link compatibility."""

import asyncio
import importlib
import json
import logging
import sys
from collections.abc import Callable, MutableMapping
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit

import anyio
import pytest

from gv_api import printers
from gv_api.core.app import create_app
from gv_api.printers import PrintersService
from gv_api.printers import recordings as recordings_module
from gv_api.printers import service as service_module
from gv_api.printers.camera import Camera
from gv_api.printers.schemas import Telemetry

from .test_printers import FakePrusa, client, eventually
from .test_tasks_router import CFG, ping

FAKE_FFMPEG = """
import os, sys, time

args = sys.argv[1:]
out = args[-1]
if os.environ.get("FAKE_ARGS"):
    with open(os.environ["FAKE_ARGS"], "a") as f:
        f.write(json.dumps(args) + "\\n")
if "mjpeg" in args:
    for i in range(1, 100000):
        sys.stdout.buffer.write(b"junk\\xff\\xd8frame%d\\xff\\xd9" % i)
        sys.stdout.buffer.flush()
        time.sleep(0.05)
if "-frames:v" in args:
    with open(out, "wb") as f:
        f.write(b"poster")
    sys.exit(0)
if os.environ.get("FAKE_FAIL"):
    sys.stderr.write("first line\\nrtsp: connection refused\\n")
    sys.exit(1)
with open(out, "wb") as f:
    f.write(b"mp4data")
    f.flush()
    sys.stdin.read(1)
    if os.environ.get("FAKE_IGNORE_Q"):
        time.sleep(60)
    f.write(b"end")
"""


@pytest.fixture
def ffmpeg(tmp_path: Path) -> str:
    script = tmp_path / "ffmpeg"
    script.write_text(f"#!{sys.executable}\nimport json\n{FAKE_FFMPEG}")
    script.chmod(0o755)
    return str(script)


Make = Callable[..., PrintersService]


def with_ffmpeg(
    make_printers: Make, ffmpeg: str, overlay: bool = True, **printer: Any
) -> PrintersService:
    settings = {"ffmpeg": ffmpeg, "overlay": overlay}
    return make_printers(FakePrusa(), settings=settings, **printer)


# camera


async def test_camera_frames_and_idle_out(ffmpeg: str) -> None:
    cam = Camera("rtsp://cam", ffmpeg, idle=0.4)
    try:
        frame, seq = await cam.next(0, 5)
        assert frame.startswith(b"\xff\xd8frame")
        assert frame.endswith(b"\xff\xd9")
        newer, seq2 = await cam.next(seq, 5)
        assert seq2 > seq
        assert newer != frame

        async def stopped() -> bool:
            return cam._task is not None and cam._task.done()

        await eventually(stopped)
    finally:
        await cam.close()


async def test_camera_frame_route(make_printers: Make, ffmpeg: str) -> None:
    async with client(with_ffmpeg(make_printers, ffmpeg)) as c:
        res = await c.get("/domotics/printers/core-one/camera")
    assert res.status_code == 200
    assert res.headers["content-type"] == "image/jpeg"
    assert res.headers["cache-control"] == "no-store"
    assert res.content.startswith(b"\xff\xd8")


async def test_no_camera_frame(make_printers: Make, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(importlib.import_module("gv_api.printers.router"), "START_WAIT", 0.2)
    async with client(make_printers(FakePrusa())) as c:
        res = await c.get("/domotics/printers/core-one/camera")
    assert (res.status_code, res.json()) == (503, {"error": "No frame available"})


async def test_mjpeg_stream(make_printers: Make, ffmpeg: str) -> None:
    svc = with_ffmpeg(make_printers, ffmpeg)
    app = create_app(CFG, ping=ping, routers=[printers.public_router])
    app.state.container = type("C", (), {"services": {PrintersService: svc}, "cfg": CFG})()
    url = urlsplit(svc.signer.camera_url("core-one"))
    sent: asyncio.Queue[MutableMapping[str, Any]] = asyncio.Queue()

    async def receive() -> dict[str, Any]:
        await asyncio.sleep(3600)
        return {"type": "http.disconnect"}

    async def send(message: MutableMapping[str, Any]) -> None:
        await sent.put(message)

    scope = {
        "type": "http",
        "asgi": {"version": "3.0"},
        "http_version": "1.1",
        "method": "GET",
        "scheme": "http",
        "path": url.path,
        "raw_path": url.path.encode(),
        "query_string": url.query.encode(),
        "root_path": "",
        "headers": [(b"host", b"t")],
        "client": ("t", 1),
        "server": ("t", 80),
    }
    task = asyncio.create_task(app(scope, receive, send))
    try:
        start = await asyncio.wait_for(sent.get(), 5)
        headers = dict(start["headers"])
        assert start["status"] == 200
        assert headers[b"content-type"] == b"multipart/x-mixed-replace; boundary=frame"
        body = b""
        while not body:
            body = (await asyncio.wait_for(sent.get(), 5))["body"]
        assert body.startswith(b"--frame\r\nContent-Type: image/jpeg\r\nContent-Length: ")
        assert b"\r\n\r\n\xff\xd8frame" in body
        assert body.endswith(b"\xff\xd9\r\n")
    finally:
        task.cancel()
        await asyncio.gather(task, return_exceptions=True)


# recordings


async def test_record_start_and_stop(
    make_printers: Make, ffmpeg: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    args_file = tmp_path / "args.jsonl"
    monkeypatch.setenv("FAKE_ARGS", str(args_file))
    svc = with_ffmpeg(make_printers, ffmpeg, rtsp="rtsp://cam/live", host="")
    async with client(svc) as c:
        res = await c.post("/domotics/printers/core-one/recordings?action=start")
        assert res.status_code == 201, res.text
        rec = res.json()
        assert rec["recording"]
        assert rec["sizeBytes"] > 0
        assert "url" not in rec
        res = await c.post("/domotics/printers/core-one/recordings?action=start")
        want = {"error": "This printer is already recording"}
        assert (res.status_code, res.json()) == (409, want)
        view = (await c.get("/domotics/printers/core-one/recordings")).json()
        assert [r["recording"] for r in view["recordings"]] == [True]

        res = await c.post("/domotics/printers/core-one/recordings?action=stop")
        assert res.status_code == 200
        [done] = res.json()["recordings"]
        assert (done["name"], done["recording"]) == (rec["name"], False)

        async def has_poster() -> bool:
            view = (await c.get("/domotics/printers/core-one/recordings")).json()
            return "posterUrl" in view["recordings"][0]

        await eventually(has_poster)
    folder = svc.settings.recordings_dir / "core-one"
    assert (folder / rec["name"]).read_bytes() == b"mp4dataend"
    record_args = json.loads(args_file.read_text().splitlines()[0])
    assert record_args[record_args.index("-i") + 1] == "rtsp://cam/live"
    assert "libx264" in record_args, "the overlay re-encodes"
    drawtext = record_args[record_args.index("-vf") + 1]
    assert drawtext.startswith("drawtext=font=monospace:textfile=")
    assert record_args[record_args.index("-t") + 1] == str(30 * 60 * 60)
    overlay = drawtext.split("textfile=")[1].split(":reload")[0].replace("\\", "")
    assert not await anyio.Path(overlay).exists(), "the overlay file goes with the recording"


async def test_record_without_overlay_copies_the_stream(
    make_printers: Make, ffmpeg: str, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    args_file = tmp_path / "args.jsonl"
    monkeypatch.setenv("FAKE_ARGS", str(args_file))
    svc = with_ffmpeg(make_printers, ffmpeg, overlay=False, host="")
    un = svc.unit("core-one")
    assert un is not None
    await un.recorder.start()
    await un.recorder.stop()
    args = json.loads(args_file.read_text().splitlines()[0])
    assert args[args.index("-c:v") + 1] == "copy"
    assert "-vf" not in args


async def test_record_start_failure(
    make_printers: Make, ffmpeg: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("FAKE_FAIL", "1")
    svc = with_ffmpeg(make_printers, ffmpeg, host="")
    async with client(svc) as c:
        res = await c.post("/domotics/printers/core-one/recordings?action=start")
    assert (res.status_code, res.json()) == (502, {"error": "rtsp: connection refused"})
    folder = svc.settings.recordings_dir / "core-one"

    async def cleaned() -> bool:
        return not any(folder.iterdir())

    await eventually(cleaned)


async def test_record_without_ffmpeg(make_printers: Make) -> None:
    svc = make_printers(FakePrusa(), host="")
    async with client(svc) as c:
        res = await c.post("/domotics/printers/core-one/recordings?action=start")
    assert res.status_code == 502


async def test_stop_escalates_when_ffmpeg_ignores_q(
    make_printers: Make, ffmpeg: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("FAKE_IGNORE_Q", "1")
    monkeypatch.setattr(recordings_module, "STOP_GRACE", 0.3)
    svc = with_ffmpeg(make_printers, ffmpeg, host="")
    un = svc.unit("core-one")
    assert un is not None
    await un.recorder.start()
    await asyncio.wait_for(un.recorder.stop(), 5)
    assert un.recorder.live_name() == ""


async def test_close_stops_a_recording(make_printers: Make, ffmpeg: str) -> None:
    svc = with_ffmpeg(make_printers, ffmpeg, host="")
    un = svc.unit("core-one")
    assert un is not None
    rec = await un.recorder.start()
    await svc.close()
    assert un.recorder.live_name() == ""
    assert (svc.settings.recordings_dir / "core-one" / rec.name).read_bytes().endswith(b"end")


async def test_auto_record_follows_the_print(
    make_printers: Make,
    ffmpeg: str,
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    monkeypatch.setattr(service_module, "AUTO_RECORD_POLL", 0.02)
    # Without the overlay, whose poller would read the scripted states too.
    svc = with_ffmpeg(make_printers, ffmpeg, overlay=False, rtsp="rtsp://cam")
    un = svc.unit("core-one")
    assert un is not None
    states = iter(["IDLE", "PRINTING", "PRINTING", "PAUSED", "FINISHED"])

    async def status() -> Telemetry:
        state = next(states, "IDLE")
        return Telemetry(configured=True, online=True, state=state)

    un.link.status = status  # type: ignore[method-assign]
    seen: list[str] = []

    async def watch() -> None:
        while True:
            seen.append(un.recorder.live_name())
            await asyncio.sleep(0.005)

    watcher = asyncio.create_task(watch())
    runner = asyncio.create_task(svc.run())
    try:

        async def ended() -> bool:
            return any(seen) and un.recorder.live_name() == ""

        await eventually(ended, 5)
    finally:
        runner.cancel()
        watcher.cancel()
        await asyncio.gather(runner, watcher, return_exceptions=True)
    assert len(svc.recordings(un).recordings) == 1

    states = iter(["IDLE", "PRINTING", "IDLE"])
    await un.recorder.start()
    caplog.set_level(logging.INFO, logger="gv_api")
    runner = asyncio.create_task(svc.run())
    try:

        async def warned() -> bool:
            return "printers: auto-record did not start" in caplog.text

        await eventually(warned)
        await asyncio.sleep(0.1)
        assert un.recorder.live_name() != "", "it does not stop a recording it did not start"
    finally:
        runner.cancel()
        await asyncio.gather(runner, return_exceptions=True)
        await un.recorder.stop()
