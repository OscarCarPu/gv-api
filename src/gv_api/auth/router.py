from typing import Annotated

from fastapi import Depends
from starlette.requests import Request
from starlette.responses import Response

from gv_api.auth.errors import AuthError
from gv_api.auth.service import AuthService
from gv_api.core.deps import service
from gv_api.core.encoding import respond
from gv_api.core.http import Body, error, parse_body
from gv_api.core.routing import tier_router

router = tier_router("public")
Svc = Annotated[AuthService, Depends(service(AuthService))]


class LoginRequest(Body):
    password: str = ""


class Login2FARequest(Body):
    token: str = ""
    code: str = ""


@router.post("/login")
async def login(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, LoginRequest, "Invalid Request")
    try:
        token, kind = svc.login(req.password)
    except AuthError as err:
        return error(401, err.message)
    return respond(200, {"kind": kind, "token": token})


@router.post("/login/2fa")
async def login_2fa(request: Request, svc: Svc) -> Response:
    req = await parse_body(request, Login2FARequest, "Invalid Request")
    try:
        token = svc.login_2fa(req.token, req.code)
    except AuthError as err:
        return error(401, err.message)
    return respond(200, {"token": token})
