"""Prusa Connect: the web login, the saved session, history, file metadata and the chamber."""

import io
import json
import sys
from collections.abc import AsyncIterator, Callable
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs

import httpx
import pytest

from gv_api import cli
from gv_api.printers import PrintersService
from gv_api.printers import connect as connect_module
from gv_api.printers.connect import ConnectClient, LoginError, parse_connect_jobs

from .test_printers import client

BASE = "http://connect.test"


class FakeConnect:
    def __init__(self) -> None:
        self.logins = self.refreshes = self.printer_calls = self.access_seq = 0
        self.valid_access = ""
        self.need_code = False
        self.file_pages: list[str] = []

    @staticmethod
    def redirect(location: str, cookie: str = "") -> httpx.Response:
        headers = [("location", location)]
        if cookie:
            headers.append(("set-cookie", cookie + "; Path=/"))
        return httpx.Response(302, headers=headers)

    async def __call__(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        form = {k: v[0] for k, v in parse_qs(request.content.decode()).items()}
        cookies = request.headers.get("cookie", "")
        if path == "/o/authorize/":
            if "sessionid=s" not in cookies:
                return self.redirect("/login/?next=/o/authorize/")
            return self.redirect("https://connect.example/login/auth-callback?code=abc")
        if path == "/login/":
            return self.login(request.method, form)
        if path == "/login/verify/":
            return self.verify(request.method, form)
        if path == "/o/token/":
            return self.token(form)
        if request.headers.get("authorization") != "Bearer " + self.valid_access:
            return httpx.Response(401)
        return self.app(path, request.url.params.get("offset", ""))

    def login(self, method: str, form: dict[str, str]) -> httpx.Response:
        if method == "GET":
            return httpx.Response(
                200,
                text='<form><input name="x"></form><form method="post"><input type="hidden" '
                'name="csrfmiddlewaretoken" value="tok&amp;1"><input type="hidden" name="next" '
                'value="/o/authorize/"></form>',
            )
        expected = {"csrfmiddlewaretoken": "tok&1", "email": "me@x", "password": "pw"}
        if any(form.get(k) != v for k, v in expected.items()):
            return httpx.Response(200)
        if self.need_code:
            return self.redirect("/login/verify/?next=/o/authorize/", "pending=p")
        self.logins += 1
        return self.redirect("/passkeys/offer/", "sessionid=s")

    def verify(self, method: str, form: dict[str, str]) -> httpx.Response:
        if method == "GET":
            return httpx.Response(
                200,
                text='<form method="post"><input type="text" name="otp_token"><input '
                'type="hidden" name="csrfmiddlewaretoken" value="v1"><input type="hidden" '
                'name="next" value="/o/authorize/"></form>',
            )
        if form.get("otp_token") != "123456" or form.get("csrfmiddlewaretoken") != "v1":
            return httpx.Response(200)
        self.logins += 1
        return self.redirect("/o/authorize/", "sessionid=s")

    def token(self, form: dict[str, str]) -> httpx.Response:
        match form.get("grant_type"):
            case "authorization_code":
                if form.get("code") != "abc" or not form.get("code_verifier"):
                    return httpx.Response(400, json={})
            case "refresh_token":
                if form.get("refresh_token") != "refresh-1":
                    return httpx.Response(400, json={})
                self.refreshes += 1
        self.access_seq += 1
        self.valid_access = f"access-{self.access_seq}"
        body = {"access_token": self.valid_access, "refresh_token": "refresh-1", "expires_in": 7200}
        return httpx.Response(200, json=body)

    def app(self, path: str, offset: str) -> httpx.Response:
        if path == "/app/printers":
            return httpx.Response(200, json={"printers": [{"uuid": "uuid-1"}], "pager": {}})
        if path == "/app/printers/uuid-1":
            self.printer_calls += 1
            chamber = {"temp": 43.9, "target_temp": 50.0, "fan_1_rpm": 0}
            return httpx.Response(200, json={"state": "PRINTING", "chamber": chamber})
        if path == "/app/printers/uuid-1/files":
            self.file_pages.append(offset)
            if offset != "0":
                late = {
                    "type": "PRINT_FILE",
                    "path": "/usb/LATE~1.BGC",
                    "meta": {"filament_type": "ASA"},
                }
                return httpx.Response(200, json={"files": [late]})
            meta = {
                "estimated_print_time": 15849,
                "filament_type": "PLA",
                "filament_used_g": 121.74,
                "filament_used_mm": 40817.15,
            }
            files: list[dict[str, Any]] = [
                {"type": "PRINT_FILE", "path": "/usb/A.BGC", "meta": meta},
                {"type": "FIRMWARE", "path": "/usb/FW.BBF", "meta": {"filament_type": "X"}},
            ]
            files += [{"type": "FILE", "path": "/usb/X.TMP"}] * (100 - len(files))
            return httpx.Response(200, json={"files": files})
        if path == "/app/printers/uuid-1/jobs":
            file = {"name": "A.BGC", "display_name": "benchy.bgcode"}
            meta = {"filament_type": "PLA", "filament_used_g": 12.46}
            jobs = [
                {"state": "FIN_OK", "time_printing": 3600.5, "file": {**file, "meta": meta}},
                {"state": "PRINTING", "time_printing": None, "file": None},
            ]
            return httpx.Response(200, json={"jobs": jobs})
        return httpx.Response(404)


Make = Callable[..., PrintersService]


@pytest.fixture
def fake() -> FakeConnect:
    return FakeConnect()


@pytest.fixture
async def connected(
    make_printers: Make, fake: FakeConnect
) -> AsyncIterator[Callable[..., PrintersService]]:
    def build(user: str = "me@x", password: str = "pw", **extra: Any) -> PrintersService:  # noqa: S107
        return make_printers(
            fake,
            host="",
            connect_user=user,
            connect_password=password,
            connect_base=BASE,
            connect_account=BASE,
            **extra,
        )

    yield build


async def history(svc: PrintersService) -> httpx.Response:
    async with client(svc) as c:
        return await c.get("/domotics/printers/core-one/history")


async def test_history_logs_in_and_keeps_the_useful_fields(
    connected: Make, fake: FakeConnect
) -> None:
    svc = connected()
    res = await history(svc)
    assert res.status_code == 200, res.text
    assert res.json() == [
        {
            "state": "FIN_OK",
            "timePrinting": 3600.5,
            "fileName": "benchy.bgcode",
            "material": "PLA",
            "filamentGrams": 12.5,
        },
        {"state": "PRINTING", "fileName": ""},
    ]
    assert (await history(svc)).status_code == 200
    assert fake.logins == 1, "the token is reused"


async def test_a_revoked_token_is_refreshed_without_a_new_login(
    connected: Make, fake: FakeConnect
) -> None:
    svc = connected()
    assert (await history(svc)).status_code == 200
    fake.valid_access = "rotated-elsewhere"
    assert (await history(svc)).status_code == 200
    assert (fake.logins, fake.refreshes) == (1, 1)


async def test_login_failures(connected: Make) -> None:
    res = await history(connected(password="nope"))
    assert res.status_code == 502
    assert "email or password" in res.json()["error"]
    res = await history(connected(user="", password=""))
    assert (res.status_code, res.json()) == (503, {"error": "Prusa Connect not configured"})


async def test_an_emailed_code_is_asked_once_then_the_session_is_kept(
    connected: Make, fake: FakeConnect, tmp_path: Path
) -> None:
    fake.need_code = True
    token_file = tmp_path / "sub" / "connect.json"
    res = await history(connected(connect_token_file=str(token_file)))
    assert res.status_code == 502
    assert "make prusa-login" in res.json()["error"]

    p = connected(connect_token_file=str(token_file)).settings.printers[0]
    transport = httpx.MockTransport(fake)

    async def wrong() -> str:
        return "000000"

    async def right() -> str:
        return "123456"

    with pytest.raises(LoginError, match="did not accept that code"):
        await ConnectClient(p, transport).save_login(wrong)
    assert not token_file.exists()
    await ConnectClient(p, transport).save_login(right)
    assert json.loads(token_file.read_text()) == {"refresh_token": "refresh-1"}
    assert token_file.stat().st_mode & 0o777 == 0o600

    logins = fake.logins
    res = await history(connected(connect_token_file=str(token_file)))
    assert res.status_code == 200, res.text
    assert fake.logins == logins, "the saved refresh token is used, no new login"


async def test_save_login_needs_an_account(connected: Make) -> None:
    p = connected(user="").settings.printers[0]

    async def code() -> str:
        return ""

    with pytest.raises(LoginError, match="PRUSA_CONNECT_USER"):
        await ConnectClient(p).save_login(code)


async def test_file_meta_and_chamber(connected: Make, fake: FakeConnect) -> None:
    async with client(connected()) as c:
        res = await c.get("/domotics/printers/core-one/files/meta")
        assert res.status_code == 200, res.text
        files = res.json()["files"]
        assert files["A.BGC"] == {
            "printSeconds": 15849,
            "material": "PLA",
            "filamentGrams": 121.7,
            "filamentMeters": 40.82,
        }
        assert files["LATE~1.BGC"] == {"material": "ASA"}
        assert "FW.BBF" not in files
        assert fake.file_pages == ["0", "100"], "a full page asks for the next one"

        for _ in range(2):
            res = await c.get("/domotics/printers/core-one/chamber")
            assert res.json() == {"temp": 43.9, "targetTemp": 50}
        assert fake.printer_calls == 1, "the chamber reading is cached briefly"
    async with client(connected(user="")) as c:
        res = await c.get("/domotics/printers/core-one/chamber")
        assert (res.status_code, res.json()) == (503, {"error": "Prusa Connect is not configured"})


async def test_connect_errors(connected: Make, fake: FakeConnect) -> None:
    async def broken(request: httpx.Request) -> httpx.Response:
        if request.url.path.startswith("/app/"):
            return httpx.Response(200, text="not json")
        return await fake(request)

    svc = connected(connect_id="uuid-1")
    svc.unit("core-one").connect._client = httpx.AsyncClient(  # type: ignore[union-attr]
        transport=httpx.MockTransport(broken)
    )
    async with client(svc) as c:
        for path in ("history", "files/meta", "chamber"):
            res = await c.get(f"/domotics/printers/core-one/{path}")
            assert res.json() == {"error": "Prusa Connect returned something unexpected"}

    async def server_error(request: httpx.Request) -> httpx.Response:
        if request.url.path.startswith("/app/"):
            return httpx.Response(500)
        return await fake(request)

    svc = connected()
    svc.unit("core-one").connect._client = httpx.AsyncClient(  # type: ignore[union-attr]
        transport=httpx.MockTransport(server_error)
    )
    res = await history(svc)
    assert res.json() == {"error": "Prusa Connect returned 500"}


def test_parse_connect_jobs_scales_a_stopped_print() -> None:
    jobs = parse_connect_jobs(
        {
            "jobs": [
                {
                    "state": "FIN_STOPPED",
                    "progress": 25,
                    "file": {"name": "A.BGC", "meta": {"filament_used_g": 20}},
                },
                "not a job",
            ]
        }
    )
    assert [(j.file_name, j.filament_grams) for j in jobs] == [("A.BGC", 5.0)]


def test_prusa_login_command(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], tmp_path: Path
) -> None:
    fake = FakeConnect()
    fake.need_code = True
    token_file = tmp_path / "connect.json"
    env = {
        "DATABASE_URL": "postgresql://u:p@127.0.0.1:1/db",
        "PASSWORD": "p",
        "SEMIPRIVATE_PASSWORD": "s",
        "JWT_SECRET": "secret",
        "TOTP_SECRET": "t",
        "ALLOWED_ORIGINS": "http://x",
        "PRUSA_CONNECT_USER": "me@x",
        "PRUSA_CONNECT_PASSWORD": "pw",
        "PRUSA_CONNECT_TOKEN_FILE": str(token_file),
    }
    for k, v in env.items():
        monkeypatch.setenv(k, v)

    class Pointed(ConnectClient):
        def __init__(self, printer: Any, transport: Any = None) -> None:
            from dataclasses import replace

            printer = replace(printer, connect_base=BASE, connect_account=BASE)
            super().__init__(printer, httpx.MockTransport(fake))

    monkeypatch.setattr(connect_module, "ConnectClient", Pointed)
    monkeypatch.setattr(sys, "stdin", io.StringIO("123456\n"))
    assert cli.main(["prusa-login"]) == 0
    out = capsys.readouterr().out
    assert "Prusa sent a code to your email. Code: " in out
    assert "Logged in." in out
    assert token_file.exists()

    monkeypatch.setattr(sys, "stdin", io.StringIO(""))
    token_file.unlink()
    assert cli.main(["prusa-login"]) == 1
    assert "Login failed: no code entered" in capsys.readouterr().err
    assert not token_file.exists()
