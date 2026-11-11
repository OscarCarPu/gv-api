from typing import Annotated

from fastapi import Depends
from starlette.responses import FileResponse, Response

from gv_api.auth import full_router
from gv_api.backup.errors import InvalidNameError, NotFoundError, RunningError
from gv_api.backup.service import BackupService
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import error, internal_error

router = full_router()
Svc = Annotated[BackupService, Depends(service(BackupService))]


@router.get("/backups")
async def list_backups(svc: Svc) -> Response:
    try:
        return respond(200, svc.backups())
    except Exception as err:
        return internal_error(err, "Failed to list backups")


@router.post("/backups")
async def create_backup(svc: Svc) -> Response:
    try:
        return respond(201, await svc.run())
    except RunningError as err:
        return error(409, str(err))
    except Exception as err:
        return internal_error(err, "Backup failed")


@router.get("/backups/{name}")
async def download_backup(name: str, svc: Svc) -> Response:
    try:
        path = svc.path(name)
    except InvalidNameError as err:
        return error(400, str(err))
    except NotFoundError as err:
        return error(404, str(err))
    except Exception as err:
        return internal_error(err, "Failed to read backup")
    return FileResponse(path, media_type="application/gzip", filename=path.name)
