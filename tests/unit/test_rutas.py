"""From internal/rutas/service_test.go and handler_test.go."""

from dataclasses import dataclass, field
from datetime import date, timedelta
from types import SimpleNamespace
from typing import Any

import httpx
import pytest

from gv_api import rutas
from gv_api.auth import AuthService
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.rutas import RutasService
from gv_api.rutas.errors import NotFoundError
from gv_api.rutas.schemas import ConcelloMark, CreateMarkRequest, UpdateMarkRequest

CFG = Config(
    password="p",
    semiprivate_password="s",
    jwt_secret="secret",
    totp_secret="t",
    allowed_origins=["http://x"],
)
AUTH = AuthService(CFG)
SEMI = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "semi")}


def mark(id: int = 1, name: str = "Arzúa", visited_on: date = date(2026, 6, 11)) -> ConcelloMark:
    return ConcelloMark(id=id, name=name, visited_on=visited_on, description="")


@dataclass
class FakeRepo:
    marks: dict[int, ConcelloMark] = field(default_factory=dict)
    calls: list[tuple[object, ...]] = field(default_factory=list)
    fail: Exception | None = None

    def _call(self, *args: object) -> None:
        self.calls.append(args)
        if self.fail is not None:
            raise self.fail

    async def list(self) -> list[ConcelloMark]:
        self._call("list")
        return list(self.marks.values())

    async def get(self, id: int) -> ConcelloMark:
        self._call("get", id)
        if id not in self.marks:
            raise NotFoundError
        return self.marks[id]

    async def create(self, name: str, visited_on: date, description: str) -> ConcelloMark:
        self._call("create", name, visited_on, description)
        return ConcelloMark(id=1, name=name, visited_on=visited_on, description=description)

    async def update(self, id: int, visited_on: date, description: str) -> ConcelloMark:
        self._call("update", id, visited_on, description)
        if id not in self.marks:
            raise NotFoundError
        old = self.marks[id]
        return ConcelloMark(id=id, name=old.name, visited_on=visited_on, description=description)

    async def delete(self, id: int) -> None:
        self._call("delete", id)


# Service


async def test_service_list() -> None:
    repo = FakeRepo(marks={1: mark()})
    assert await RutasService(repo).list() == [mark()]


async def test_service_get_not_found() -> None:
    with pytest.raises(NotFoundError):
        await RutasService(FakeRepo()).get(7)


async def test_service_create_parses_the_date() -> None:
    repo = FakeRepo()
    req = CreateMarkRequest(name="Melide", visited_on="2026-06-11", description="d")
    created = await RutasService(repo).create(req)
    assert repo.calls == [("create", "Melide", date(2026, 6, 11), "d")]
    assert created.name == "Melide"


@pytest.mark.parametrize("raw", ["11/06/2026", "2026-6-11", ""])
async def test_service_rejects_other_date_formats(raw: str) -> None:
    repo = FakeRepo()
    with pytest.raises(ValueError, match=r'invalid date ".*": must be YYYY-MM-DD'):
        await RutasService(repo).create(CreateMarkRequest(name="X", visited_on=raw))
    assert repo.calls == []


async def test_service_update_error_propagates() -> None:
    repo = FakeRepo(fail=RuntimeError("db error"))
    with pytest.raises(RuntimeError, match="db error"):
        await RutasService(repo).update(3, UpdateMarkRequest(visited_on="2026-06-11"))


async def test_service_delete() -> None:
    repo = FakeRepo()
    await RutasService(repo).delete(4)
    assert repo.calls == [("delete", 4)]


# Router


async def ping() -> None:
    return None


async def call(
    repo: FakeRepo, method: str, path: str, body: str | None = None, headers: Any = SEMI
) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[rutas.router])
    app.state.container = SimpleNamespace(
        services={AuthService: AUTH, RutasService: RutasService(repo)}
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.request(method, path, content=body, headers=headers)
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


async def test_list() -> None:
    repo = FakeRepo(marks={1: mark()})
    assert await call(repo, "GET", "/rutas/marks") == (
        200,
        [{"id": 1, "name": "Arzúa", "visited_on": "2026-06-11T00:00:00Z", "description": ""}],
    )


async def test_empty_list_is_an_empty_array() -> None:
    assert await call(FakeRepo(), "GET", "/rutas/marks") == (200, [])


async def test_full_token_is_accepted_and_no_token_is_not() -> None:
    full = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "full")}
    assert (await call(FakeRepo(), "GET", "/rutas/marks", headers=full))[0] == 200
    assert await call(FakeRepo(), "GET", "/rutas/marks", headers={}) == (
        401,
        {"error": "Unauthorized"},
    )


async def test_list_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    assert await call(repo, "GET", "/rutas/marks") == (500, {"error": "Failed to list marks"})


@pytest.mark.parametrize(
    ("repo", "path", "expected"),
    [
        (FakeRepo(), "/rutas/marks/abc", (400, {"error": "invalid mark id"})),
        (FakeRepo(), "/rutas/marks/0", (400, {"error": "invalid mark id"})),
        (FakeRepo(), "/rutas/marks/7", (404, {"error": "mark not found"})),
        (
            FakeRepo(fail=RuntimeError("db")),
            "/rutas/marks/7",
            (500, {"error": "Failed to get mark"}),
        ),
    ],
)
async def test_get_errors(repo: FakeRepo, path: str, expected: tuple[int, Any]) -> None:
    assert await call(repo, "GET", path) == expected


async def test_get() -> None:
    status, body = await call(FakeRepo(marks={7: mark(7, "Melide")}), "GET", "/rutas/marks/7")
    assert (status, body["name"]) == (200, "Melide")


@pytest.mark.parametrize(
    ("body", "message"),
    [
        ("not json", "invalid body"),
        ('{"name": "   ", "visited_on": "2026-06-11"}', "name is required"),
        ('{"name": "' + "a" * 201 + '", "visited_on": "2026-06-11"}', "name too long"),
        ('{"name": "' + "á" * 101 + '", "visited_on": "2026-06-11"}', "name too long"),
        ('{"name": "Arzúa"}', "visited_on is required"),
        (
            '{"name": "Arzúa", "visited_on": "11/06/2026"}',
            'invalid date "11/06/2026": must be YYYY-MM-DD',
        ),
    ],
)
async def test_create_validation(body: str, message: str) -> None:
    repo = FakeRepo()
    assert await call(repo, "POST", "/rutas/marks", body) == (400, {"error": message})
    assert repo.calls == []


async def test_create_trims_the_name() -> None:
    repo = FakeRepo()
    body = '{"name": "  Arzúa  ", "visited_on": "2026-06-11", "description": null}'
    status, created = await call(repo, "POST", "/rutas/marks", body)
    assert (status, created["name"], created["description"]) == (201, "Arzúa", "")
    assert repo.calls == [("create", "Arzúa", date(2026, 6, 11), "")]


async def test_create_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    body = '{"name": "Arzúa", "visited_on": "2026-06-11"}'
    assert await call(repo, "POST", "/rutas/marks", body) == (
        500,
        {"error": "Failed to create mark"},
    )


@pytest.mark.parametrize(
    ("path", "body", "expected"),
    [
        ("/rutas/marks/abc", "{}", (400, {"error": "invalid mark id"})),
        ("/rutas/marks/3", "[", (400, {"error": "invalid body"})),
        ("/rutas/marks/3", '{"description": "x"}', (400, {"error": "visited_on is required"})),
        ("/rutas/marks/3", '{"visited_on": "2026-06-11"}', (404, {"error": "mark not found"})),
    ],
)
async def test_update_errors(path: str, body: str, expected: tuple[int, Any]) -> None:
    assert await call(FakeRepo(), "PUT", path, body) == expected


async def test_update_keeps_the_name() -> None:
    repo = FakeRepo(marks={3: mark(3, "Melide")})
    status, body = await call(repo, "PUT", "/rutas/marks/3", '{"visited_on": "2026-06-12"}')
    assert (status, body["name"], body["visited_on"]) == (200, "Melide", "2026-06-12T00:00:00Z")
    assert repo.calls == [("update", 3, date(2026, 6, 12), "")]


async def test_update_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    assert await call(repo, "PUT", "/rutas/marks/3", '{"visited_on": "2026-06-11"}') == (
        500,
        {"error": "Failed to update mark"},
    )


async def test_delete() -> None:
    repo = FakeRepo()
    assert await call(repo, "DELETE", "/rutas/marks/9") == (204, "")
    assert repo.calls == [("delete", 9)]


async def test_delete_errors() -> None:
    assert await call(FakeRepo(), "DELETE", "/rutas/marks/x") == (
        400,
        {"error": "invalid mark id"},
    )
    assert await call(FakeRepo(fail=NotFoundError()), "DELETE", "/rutas/marks/9") == (
        500,
        {"error": "Failed to delete mark"},
    )
