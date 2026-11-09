import logging
import re

import httpx
import pytest
from fastapi import APIRouter
from starlette.requests import Request
from starlette.responses import Response

from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.core.cors import canonical, parse_header_list
from gv_api.core.http import ApiError, parse_id

ORIGIN = "https://gv.example.com"


def config(**overrides: object) -> Config:
    return Config(
        database_url="postgres://x",
        password="p",
        semiprivate_password="s",
        jwt_secret="j",
        totp_secret="t",
        allowed_origins=[ORIGIN],
        **overrides,  # type: ignore[arg-type]
    )


def habits_router() -> APIRouter:
    router = APIRouter()

    @router.get("/habits")
    async def daily() -> Response:
        return Response(status_code=200)

    @router.post("/habits")
    async def create() -> Response:
        raise ApiError(400, "name is required")

    @router.delete("/habits/{id}")
    async def delete(id: str) -> Response:
        parse_id(id, "habit")
        return Response(status_code=204)

    @router.get("/boom")
    async def boom(request: Request) -> Response:
        raise RuntimeError("kaput")

    return router


async def ok_ping() -> None:
    return None


@pytest.fixture
def client() -> httpx.AsyncClient:
    app = create_app(config(), ping=ok_ping, routers=[habits_router()])
    return httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app, raise_app_exceptions=False), base_url="http://t"
    )


async def test_unknown_path_is_plain_404(client: httpx.AsyncClient) -> None:
    resp = await client.get("/does-not-exist")
    assert resp.status_code == 404
    assert resp.text == "404 page not found\n"
    assert resp.headers["content-type"].startswith("text/plain")


async def test_trailing_slash_is_not_redirected(client: httpx.AsyncClient) -> None:
    resp = await client.get("/habits/")
    assert resp.status_code == 404


async def test_wrong_method_lists_every_method_of_the_path(client: httpx.AsyncClient) -> None:
    resp = await client.patch("/habits")
    assert resp.status_code == 405
    assert resp.content == b""
    assert sorted(resp.headers.get_list("allow")) == ["GET", "POST"]


async def test_head_is_not_added_to_get_routes(client: httpx.AsyncClient) -> None:
    resp = await client.head("/habits")
    assert resp.status_code == 405


@pytest.mark.parametrize("path", ["/docs", "/redoc", "/openapi.json"])
async def test_no_framework_routes(client: httpx.AsyncClient, path: str) -> None:
    assert (await client.get(path)).status_code == 404


async def test_api_error_envelope(client: httpx.AsyncClient) -> None:
    resp = await client.post("/habits")
    assert resp.status_code == 400
    assert resp.json() == {"error": "name is required"}
    assert resp.headers["content-type"] == "application/json"


async def test_invalid_path_id(client: httpx.AsyncClient) -> None:
    resp = await client.delete("/habits/abc")
    assert (resp.status_code, resp.json()) == (400, {"error": "invalid habit id"})


async def test_unhandled_error_is_empty_500_with_request_id(
    client: httpx.AsyncClient, caplog: pytest.LogCaptureFixture
) -> None:
    with caplog.at_level(logging.ERROR):
        resp = await client.get("/boom", headers={"Origin": ORIGIN})
    assert resp.status_code == 500
    assert resp.content == b""
    assert resp.headers["x-request-id"]
    assert resp.headers["access-control-allow-origin"] == ORIGIN
    assert any("kaput" in str(r.__dict__.get("error", r.exc_info)) for r in caplog.records)


async def test_request_id_echoed_or_generated(client: httpx.AsyncClient) -> None:
    echoed = await client.get("/habits", headers={"X-Request-ID": "abc123"})
    assert echoed.headers["x-request-id"] == "abc123"
    generated = (await client.get("/habits")).headers["x-request-id"]
    assert re.fullmatch(r"[0-9a-f]{16}", generated)
    assert (await client.get("/nope")).headers["x-request-id"]


@pytest.mark.parametrize(
    "requested",
    [
        "content-type",
        "authorization",
        "x-device-id",
        "x-request-id",
        "content-type,authorization,x-device-id,x-request-id",
    ],
)
async def test_preflight_from_web_client(client: httpx.AsyncClient, requested: str) -> None:
    resp = await client.options(
        "/tasks/time-entries?start_time=2026-08-19T00:00:00Z",
        headers={
            "Origin": ORIGIN,
            "Access-Control-Request-Method": "GET",
            "Access-Control-Request-Headers": requested,
        },
    )
    assert resp.status_code == 200
    assert resp.headers["access-control-allow-origin"] == ORIGIN
    assert resp.headers["access-control-allow-methods"] == "GET"
    assert resp.headers["access-control-max-age"] == "300"
    assert resp.headers["x-request-id"]


async def test_preflight_from_foreign_origin_has_no_cors_headers(client: httpx.AsyncClient) -> None:
    resp = await client.options(
        "/tasks/tasks/list-fast",
        headers={"Origin": "https://evil.example.com", "Access-Control-Request-Method": "GET"},
    )
    assert resp.status_code == 200
    assert "access-control-allow-origin" not in resp.headers


async def test_preflight_with_unknown_header_is_refused(client: httpx.AsyncClient) -> None:
    resp = await client.options(
        "/habits",
        headers={
            "Origin": ORIGIN,
            "Access-Control-Request-Method": "GET",
            "Access-Control-Request-Headers": "x-evil",
        },
    )
    assert "access-control-allow-origin" not in resp.headers


async def test_actual_request_cors_headers(client: httpx.AsyncClient) -> None:
    resp = await client.get("/habits", headers={"Origin": ORIGIN})
    assert resp.headers["access-control-allow-origin"] == ORIGIN
    assert resp.headers["access-control-expose-headers"] == "Content-Length, X-Request-Id"
    assert "Origin" in resp.headers["vary"]
    foreign = await client.get("/habits", headers={"Origin": "https://evil.example.com"})
    assert "access-control-allow-origin" not in foreign.headers


async def test_health_ok_and_down() -> None:
    async def down() -> None:
        raise OSError("connection refused")

    up_app = create_app(config(), ping=ok_ping)
    down_app = create_app(config(), ping=down)
    async with (
        httpx.AsyncClient(transport=httpx.ASGITransport(app=up_app), base_url="http://t") as up,
        httpx.AsyncClient(transport=httpx.ASGITransport(app=down_app), base_url="http://t") as dn,
    ):
        ok = await up.get("/health")
        bad = await dn.get("/health")
    assert (ok.status_code, ok.content) == (200, b"")
    assert (bad.status_code, bad.content) == (503, b"")


async def test_health_times_out() -> None:
    import asyncio

    async def slow() -> None:
        await asyncio.sleep(5)

    app = create_app(config(health_timeout_ms=50), ping=slow)
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://t") as c:
        assert (await c.get("/health")).status_code == 503


def test_header_helpers_match_go() -> None:
    assert canonical("x-request-id") == "X-Request-Id"
    assert parse_header_list("content-type, X-DEVICE-ID,authorization") == [
        "Content-Type",
        "X-Device-Id",
        "Authorization",
    ]
