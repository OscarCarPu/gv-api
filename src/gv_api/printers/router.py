"""Printer routes: semiprivate, plus three public routes authorized by a signed link."""

import contextlib
from collections.abc import AsyncIterator, Sequence
from pathlib import Path
from typing import Annotated
from urllib.parse import quote, unquote

import anyio
from fastapi import Depends
from pydantic import BaseModel, Field
from pydantic_core import to_json
from starlette.requests import ClientDisconnect, Request
from starlette.responses import FileResponse, Response, StreamingResponse

from gv_api.auth import semi_router
from gv_api.core.deps import home_only, service
from gv_api.core.encoding import respond
from gv_api.core.http import ApiError, Body, error, internal_error, parse_body, query
from gv_api.core.routing import tier_router
from gv_api.printers.camera import FRAME_WAIT, START_WAIT, NoFrameError
from gv_api.printers.link import (
    LinkError,
    chunks,
    download_path,
    first_text,
    sanitize_file_name,
    text,
    upstream_message,
    upstream_status,
)
from gv_api.printers.schemas import FilesMeta, Uploads
from gv_api.printers.service import PrintersService, Unit, new_upload_id
from gv_api.printers.uploads import (
    MAX_UPLOAD_BYTES,
    ChunkError,
    TooLargeError,
    parse_byte_header,
    valid_upload_id,
)

router = semi_router(home_only)
public_router = tier_router("public", [home_only])
Svc = Annotated[PrintersService, Depends(service(PrintersService))]

MAX_CHUNK_BYTES = 100 << 20
NO_STORE = {"Cache-Control": "no-store"}


def send(status: int, data: BaseModel | Sequence[BaseModel]) -> Response:
    """Empty fields are left out: gv-web's printer schemas read them as optional, not nullable."""
    body = to_json(data, by_alias=True, exclude_none=True)
    return Response(body, status_code=status, media_type="application/json")


def fail(err: Exception, fallback: str) -> Response:
    if isinstance(err, ApiError):
        return error(err.status, err.message)
    return error(502, str(err) or fallback)


def unit(svc: PrintersService, printer_id: str) -> Unit:
    un = svc.unit(printer_id)
    if un is None:
        raise ApiError(404, "Printer not found")
    return un


def configured(svc: PrintersService, printer_id: str) -> Unit:
    un = unit(svc, printer_id)
    if not un.printer.host:
        raise ApiError(503, "PrusaLink not configured")
    return un


def with_connect(svc: PrintersService, printer_id: str, message: str) -> Unit:
    un = unit(svc, printer_id)
    if not un.connect.configured():
        raise ApiError(503, message)
    return un


@router.get("/domotics/printers")
async def list_printers(svc: Svc) -> Response:
    return send(200, svc.list())


@router.get("/domotics/printers/{id}/status")
async def status(id: str, svc: Svc) -> Response:
    return send(200, await unit(svc, id).link.status())


@router.delete("/domotics/printers/{id}/job")
async def stop_job(id: str, svc: Svc) -> Response:
    un = configured(svc, id)
    try:
        await un.link.stop_job()
    except Exception as err:
        return fail(err, "Could not stop the print")
    return respond(200, {"stopped": True})


@router.get("/domotics/printers/{id}/history")
async def history(id: str, svc: Svc) -> Response:
    un = with_connect(svc, id, "Prusa Connect not configured")
    try:
        return send(200, await un.connect.history())
    except Exception as err:
        return fail(err, "Could not read the print history")


@router.get("/domotics/printers/{id}/files")
async def files(id: str, svc: Svc) -> Response:
    un = unit(svc, id)
    view = await un.link.files()
    for f in view.files:
        f.download_url = svc.signer.file_url(un.printer.id, f.name, f.display_name)
    return send(200, view)


@router.get("/domotics/printers/{id}/files/meta")
async def files_meta(id: str, svc: Svc) -> Response:
    un = with_connect(svc, id, "Prusa Connect is not configured")
    try:
        return send(200, FilesMeta(files=await un.connect.file_meta()))
    except Exception as err:
        return fail(err, "Could not read file details from Prusa Connect")


@router.get("/domotics/printers/{id}/chamber")
async def chamber(id: str, svc: Svc) -> Response:
    un = with_connect(svc, id, "Prusa Connect is not configured")
    try:
        return send(200, await un.connect.chamber())
    except Exception as err:
        return fail(err, "Could not read the chamber from Prusa Connect")


class RenameBody(Body):
    name: str = ""
    new_name: str = Field("", alias="newName")


@router.post("/domotics/printers/{id}/files/rename")
async def rename(id: str, request: Request, svc: Svc) -> Response:
    un = configured(svc, id)
    body = await parse_body(request, RenameBody, "Invalid request body")
    name = sanitize_file_name(body.name)
    if not name:
        return error(400, "Missing or invalid file name")
    try:
        upload_id, new_name = await svc.rename(un, name, body.new_name)
    except Exception as err:
        return fail(err, "Could not rename the file")
    return respond(202, {"uploadId": upload_id, "name": new_name})


def upload_target(svc: PrintersService, printer_id: str, request: Request) -> tuple[Unit, str]:
    un = configured(svc, printer_id)
    raw = request.headers.get("x-file-name", "")
    if not raw:
        raise ApiError(400, "Missing X-File-Name header")
    name = sanitize_file_name(unquote(raw))
    if not name:
        raise ApiError(400, "Unsupported file name. Expected a .bgcode, .gcode, .gco or .g file.")
    return un, name


def stage_failed(err: Exception) -> Response:
    if isinstance(err, ApiError):
        return error(err.status, err.message)
    return internal_error(err, "Could not stage the uploaded file")


@router.put("/domotics/printers/{id}/files")
async def upload(id: str, request: Request, svc: Svc) -> Response:
    un, name = upload_target(svc, id, request)
    upload_id = request.headers.get("x-upload-id", "")
    if not valid_upload_id(upload_id):
        upload_id = new_upload_id()
    try:
        size = await svc.uploads.stage_whole(upload_id, request.stream(), MAX_UPLOAD_BYTES)
    except OSError, TooLargeError, ClientDisconnect:
        return error(400, "Could not read the uploaded file")
    if size == 0:
        svc.uploads.path(upload_id).unlink(missing_ok=True)
        return error(400, "Empty file")
    try:
        svc.forward(un, upload_id, name, query(request, "overwrite") == "1")
    except Exception as err:
        return stage_failed(err)
    return respond(202, {"name": name, "uploadId": upload_id})


async def read_chunk(request: Request) -> bytes | None:
    data = bytearray()
    try:
        async for part in request.stream():
            data += part
            if len(data) > MAX_CHUNK_BYTES:
                return None
    except ClientDisconnect:
        return None
    return bytes(data)


@router.patch("/domotics/printers/{id}/files")
async def upload_chunk(id: str, request: Request, svc: Svc) -> Response:
    un, name = upload_target(svc, id, request)
    upload_id = request.headers.get("x-upload-id", "")
    offset = parse_byte_header(request.headers.get("x-upload-offset", ""))
    total = parse_byte_header(request.headers.get("x-upload-total", ""))
    if not valid_upload_id(upload_id) or offset is None or total is None:
        return error(
            400, "Missing or malformed X-Upload-Id / X-Upload-Offset / X-Upload-Total header"
        )
    chunk = await read_chunk(request)
    if chunk is None:
        return error(400, "Could not read the uploaded file")
    if offset == 0:
        svc.uploads.sweep_staged()
    try:
        received, complete = await svc.uploads.append_chunk(upload_id, offset, total, chunk)
    except ChunkError as err:
        return respond(err.status, {"error": err.message, "received": err.received})
    except OSError:
        return error(500, "Could not stage the uploaded chunk")
    if not complete:
        return respond(200, {"received": received, "complete": False})
    try:
        svc.forward(un, upload_id, name, query(request, "overwrite") == "1")
    except Exception as err:
        return stage_failed(err)
    return respond(
        202, {"name": name, "uploadId": upload_id, "received": received, "complete": True}
    )


async def file_command(
    svc: PrintersService,
    printer_id: str,
    request: Request,
    method: str,
    op: str,
    failure: str,
    extra: dict[str, str] | None = None,
) -> Response:
    un = unit(svc, printer_id)
    name = sanitize_file_name(query(request, "name"))
    if not name:
        return error(400, "Missing or invalid file name")
    try:
        code = await un.link.command(method, name, extra)
    except Exception as err:
        return fail(err, failure)
    if not 200 <= code <= 299:
        return error(upstream_status(code), upstream_message(code, op))
    return respond(200, {"name": name})


@router.post("/domotics/printers/{id}/files")
async def start_print(id: str, request: Request, svc: Svc) -> Response:
    return await file_command(svc, id, request, "POST", "print", "Could not start the print")


@router.delete("/domotics/printers/{id}/files")
async def delete_file(id: str, request: Request, svc: Svc) -> Response:
    return await file_command(
        svc, id, request, "DELETE", "delete", "Could not delete the file", {"Force": "?0"}
    )


@router.get("/domotics/printers/{id}/files/progress")
async def progress(id: str, request: Request, svc: Svc) -> Response:
    if "u" not in request.query_params:
        return send(200, Uploads(uploads=svc.uploads.active(id)))
    upload_id = query(request, "u")
    if not valid_upload_id(upload_id):
        return error(400, "Invalid upload id")
    return send(200, svc.uploads.get(upload_id))


@router.get("/domotics/printers/{id}/camera")
async def camera(id: str, svc: Svc) -> Response:
    un = unit(svc, id)
    try:
        frame, _ = await un.camera.next(0, START_WAIT)
    except NoFrameError:
        return error(503, "No frame available")
    return Response(frame, media_type="image/jpeg", headers=NO_STORE)


@router.get("/domotics/printers/{id}/camera/url")
async def camera_url(id: str, svc: Svc) -> Response:
    un = unit(svc, id)
    un.camera.warm()
    return respond(200, {"url": svc.signer.camera_url(un.printer.id)})


@router.get("/domotics/printers/{id}/recordings")
async def recordings(id: str, svc: Svc) -> Response:
    return send(200, svc.recordings(unit(svc, id)))


@router.post("/domotics/printers/{id}/recordings")
async def recording_action(id: str, request: Request, svc: Svc) -> Response:
    un = unit(svc, id)
    match query(request, "action"):
        case "start":
            try:
                return send(201, await un.recorder.start())
            except Exception as err:
                return fail(err, "Could not start recording")
        case "stop":
            try:
                await un.recorder.stop()
            except Exception as err:
                return fail(err, "Could not stop recording")
            return send(200, svc.recordings(un))
    return error(400, "Unknown action. Expected start or stop.")


@router.delete("/domotics/printers/{id}/recordings")
async def delete_recordings(id: str, request: Request, svc: Svc) -> Response:
    un = unit(svc, id)
    names = request.query_params.getlist("name")
    if not names:
        return error(400, "Missing recording name")
    deleted: list[str] = []
    failed: dict[str, str] = {}
    for name in names:
        try:
            un.recorder.remove(name)
        except ApiError as err:
            if len(names) == 1:
                return fail(err, "Could not delete the recording")
            failed[name] = err.message
            continue
        deleted.append(name)
    return respond(200, {"deleted": deleted, "failed": failed})


# public, signed


def check_signature(svc: PrintersService, request: Request, printer_id: str, name: str) -> None:
    exp, sig = query(request, "exp"), query(request, "sig")
    if not svc.signer.verify(printer_id, name, exp, sig):
        raise ApiError(401, "Unauthorized")


@public_router.get("/domotics/printers/{id}/camera/stream")
async def camera_stream(id: str, request: Request, svc: Svc) -> Response:
    check_signature(svc, request, id, "camera")
    un = unit(svc, id)

    async def frames() -> AsyncIterator[bytes]:
        seq = 0
        while True:
            try:
                frame, seq = await un.camera.next(seq, START_WAIT if seq == 0 else FRAME_WAIT)
            except NoFrameError:
                return
            head = f"--frame\r\nContent-Type: image/jpeg\r\nContent-Length: {len(frame)}\r\n\r\n"
            yield head.encode() + frame + b"\r\n"

    return StreamingResponse(
        frames(), media_type="multipart/x-mixed-replace; boundary=frame", headers=NO_STORE
    )


@public_router.get("/domotics/printers/{id}/recordings/{name}")
async def media(id: str, name: str, request: Request, svc: Svc) -> Response:
    check_signature(svc, request, id, name)
    un = unit(svc, id)
    path = un.recorder.open(name)
    if path is None:
        return error(404, "Recording not found")
    live = un.recorder.live_name() == name
    headers = {
        "Cache-Control": "no-store" if live else "private, max-age=31536000, immutable",
    }
    if query(request, "download") == "1":
        headers["Content-Disposition"] = f'attachment; filename="{id}_{name}"'
    media_type = "image/jpeg" if name.endswith(".jpg") else "video/mp4"
    if live and "range" not in request.headers:
        return growing_file(path, media_type, headers)
    return FileResponse(path, media_type=media_type, headers=headers)


def growing_file(path: Path, media_type: str, headers: dict[str, str]) -> Response:
    """A file still being written, sent up to its size now: more would break Content-Length."""
    size = path.stat().st_size

    async def body() -> AsyncIterator[bytes]:
        left = size
        async with await anyio.open_file(path, "rb") as f:
            while left > 0 and (chunk := await f.read(min(64 << 10, left))):
                left -= len(chunk)
                yield chunk

    headers = {**headers, "Content-Length": str(size), "Accept-Ranges": "bytes"}
    return StreamingResponse(body(), media_type=media_type, headers=headers)


def content_disposition(filename: str) -> str:
    quoted = quote(filename)
    if quoted != filename:
        return f"attachment; filename*=utf-8''{quoted}"
    return f'attachment; filename="{filename}"'


@public_router.get("/domotics/printers/{id}/files/download")
async def download(id: str, request: Request, svc: Svc) -> Response:
    name = sanitize_file_name(query(request, "name"))
    if not name:
        raise ApiError(401, "Unauthorized")
    check_signature(svc, request, id, "file/" + name)
    un = configured(svc, id)
    try:
        storage, _ = await un.link.storage()
        info = await un.link.file_info(storage, name)
        res = await un.link.open_download(download_path(storage, name, info))
    except Exception as err:
        return fail(err, "Could not read the file from the printer")

    async def body() -> AsyncIterator[bytes]:
        # A transfer that fails midway ends the copy short, as a dropped link would.
        try:
            with contextlib.suppress(LinkError):
                async for chunk in chunks(res):
                    yield chunk
        finally:
            await res.aclose()

    filename = first_text(sanitize_file_name(query(request, "as")), text(info, "display_name"))
    headers = {
        "Content-Disposition": content_disposition(filename or name),
        **NO_STORE,
    }
    size = info.get("size")
    if isinstance(size, int) and size > 0:
        headers["Content-Length"] = str(size)
    return StreamingResponse(body(), media_type="application/octet-stream", headers=headers)
