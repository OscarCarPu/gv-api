"""The auth spec's route tiers."""

import time
from datetime import UTC, datetime, timedelta
from types import SimpleNamespace

import httpx
import jwt
import pytest
from starlette.responses import Response

from gv_api.auth import AuthService, full_router, semi_router
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.routing import route_table

SECRET = "super-secret-test-key"
CFG = Config(
    password="p",
    semiprivate_password="s",
    jwt_secret=SECRET,
    totp_secret="t",
    allowed_origins=["http://x"],
)
SVC = AuthService(CFG)
UNAUTHORIZED = (401, '{"error":"Unauthorized"}')


async def ping() -> None:
    return None


def app() -> httpx.AsyncClient:
    full, semi = full_router(), semi_router()

    @full.get("/habits")
    async def habits() -> Response:
        return Response("habits")

    @semi.get("/rutas/marks")
    async def marks() -> Response:
        return Response("marks")

    application = create_app(CFG, ping=ping, routers=[full, semi])
    application.state.container = SimpleNamespace(services={AuthService: SVC})
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=application), base_url="http://t")


def token(kind: str, ttl: timedelta = timedelta(days=30)) -> str:
    return SVC.generate_token(ttl, kind)


async def call(path: str, authorization: str | None) -> tuple[int, str]:
    headers = {} if authorization is None else {"Authorization": authorization}
    async with app() as c:
        resp = await c.get(path, headers=headers)
    return resp.status_code, resp.text


@pytest.mark.parametrize(
    ("path", "kind", "served"),
    [
        ("/habits", "full", True),
        ("/habits", "semi", False),
        ("/habits", "tmp", False),
        ("/rutas/marks", "semi", True),
        ("/rutas/marks", "full", True),
        ("/rutas/marks", "tmp", False),
        ("/rutas/marks", "other", False),
    ],
)
async def test_token_kinds_per_tier(path: str, kind: str, served: bool) -> None:
    status, body = await call(path, f"Bearer {token(kind)}")
    assert (status, body) == ((200, path.split("/")[-1]) if served else UNAUTHORIZED)


def _expired() -> str:
    past = datetime.now(UTC) - timedelta(days=31)
    return AuthService(CFG, now=lambda: past).generate_token(timedelta(days=30), "full")


@pytest.mark.parametrize(
    "authorization",
    [
        None,
        "",
        "Bearer",
        "Bearer ",
        "bearer {full}",
        "Token {full}",
        "{full}",  # missing "Bearer "
        "Bearer  {full}",
        "Bearer garbage",
        "Bearer {expired}",
        "Bearer {none}",
        "Bearer {other_secret}",
    ],
)
@pytest.mark.parametrize("path", ["/habits", "/rutas/marks"])
async def test_rejected_authorization(path: str, authorization: str | None) -> None:
    claims = {"exp": int(time.time()) + 3600, "kind": "full"}
    values = {
        "full": token("full"),
        "expired": _expired(),
        "none": jwt.encode(claims, None, algorithm="none"),
        "other_secret": jwt.encode(claims, "x" * 32, algorithm="HS256"),
    }
    header = None if authorization is None else authorization.format(**values)
    assert await call(path, header) == UNAUTHORIZED


async def test_auth_runs_after_routing() -> None:
    async with app() as c:
        missing = await c.get("/nope")
        wrong_method = await c.post("/habits")
    assert missing.status_code == 404
    assert wrong_method.status_code == 405
    assert wrong_method.headers.get_list("allow") == ["GET"]


def test_tier_tags() -> None:
    full, semi = full_router(), semi_router()

    @full.get("/a")
    async def a() -> None:
        return None

    @semi.get("/b")
    async def b() -> None:
        return None

    table = route_table(create_app(CFG, ping=ping, routers=[full, semi]))
    assert {("GET", "/a", "full"), ("GET", "/b", "semi")} <= table
