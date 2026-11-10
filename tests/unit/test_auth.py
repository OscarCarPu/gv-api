"""Ported from internal/auth/service_test.go and handler_test.go, plus token compatibility."""

import base64
import json
import shutil
import subprocess
import time
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path

import httpx
import jwt
import pyotp
import pytest

from gv_api.auth import AuthService
from gv_api.auth.errors import InvalidCodeError, InvalidPasswordError, InvalidTokenError
from gv_api.auth.service import validate_totp
from gv_api.config import Config
from gv_api.main import build

SECRET = "super-secret-test-key"
TOTP_SECRET = "JBSWY3DPEHPK3PXP"
REPO = Path(__file__).parents[2]


def config(**overrides: str) -> Config:
    values = {
        "database_url": "postgresql://u:p@127.0.0.1:1/db",
        "password": "Abc123..",
        "semiprivate_password": "semi-pass",
        "jwt_secret": SECRET,
        "totp_secret": TOTP_SECRET,
        "allowed_origins": ["http://x"],
        **overrides,
    }
    return Config.model_validate(values)


def segment(token: str, i: int) -> bytes:
    part = token.split(".")[i]
    return base64.urlsafe_b64decode(part + "=" * (-len(part) % 4))


def always(result: bool) -> object:
    return lambda _code, _secret: result


# service


def test_main_password_returns_tmp_token() -> None:
    svc = AuthService(config())
    token, kind = svc.login("Abc123..")
    assert kind == "tmp"
    svc.validate_token(token, "tmp")
    claims = json.loads(segment(token, 1))
    assert claims["exp"] - claims["iat"] == 5 * 60


def test_semiprivate_password_returns_semi_token() -> None:
    svc = AuthService(config())
    token, kind = svc.login("semi-pass")
    assert kind == "semi"
    svc.validate_token(token, "semi")
    claims = json.loads(segment(token, 1))
    assert claims["exp"] - claims["iat"] == 30 * 24 * 3600


@pytest.mark.parametrize("password", ["incorrect password", "", "abc123..", "Abc123.. "])
def test_rejects_incorrect_password(password: str) -> None:
    with pytest.raises(InvalidPasswordError, match=r"^invalid password$"):
        AuthService(config()).login(password)


def test_token_bytes_match_golang_jwt() -> None:
    now = datetime(2026, 11, 10, 12, 0, 0, 750_000, tzinfo=UTC)
    token = AuthService(config(), now=lambda: now).generate_token(timedelta(minutes=5), "tmp")
    assert segment(token, 0) == b'{"alg":"HS256","typ":"JWT"}'
    iat = int(now.timestamp())
    assert segment(token, 1) == f'{{"exp":{iat + 300},"iat":{iat},"kind":"tmp"}}'.encode()


def test_2fa_success() -> None:
    svc = AuthService(config(), totp=always(True))  # type: ignore[arg-type]
    tmp, _ = svc.login("Abc123..")
    full = svc.login_2fa(tmp, "123456")
    svc.validate_token(full, "full")
    claims = json.loads(segment(full, 1))
    assert claims["exp"] - claims["iat"] == 30 * 24 * 3600


def test_2fa_failure() -> None:
    svc = AuthService(config(), totp=always(False))  # type: ignore[arg-type]
    tmp, _ = svc.login("Abc123..")
    with pytest.raises(InvalidCodeError, match=r"^invalid 2fa code$"):
        svc.login_2fa(tmp, "wrong-code")


def test_2fa_needs_a_live_tmp_token() -> None:
    svc = AuthService(config(), totp=always(True))  # type: ignore[arg-type]
    semi, _ = svc.login("semi-pass")
    full = svc.generate_token(timedelta(days=30), "full")
    old = datetime.now(UTC) - timedelta(minutes=6)
    expired, _ = AuthService(config(), now=lambda: old).login("Abc123..")
    for token in (semi, full, expired, "bad-token", ""):
        with pytest.raises(InvalidTokenError, match=r"^invalid token$"):
            svc.login_2fa(token, "123456")


def _forge(header: dict[str, object], claims: dict[str, object], sig: bytes = b"") -> str:
    def enc(b: bytes) -> str:
        return base64.urlsafe_b64encode(b).rstrip(b"=").decode()

    return ".".join([enc(json.dumps(header).encode()), enc(json.dumps(claims).encode()), enc(sig)])


def test_token_validation_rules() -> None:
    svc = AuthService(config())
    exp = int(time.time()) + 3600
    claims = {"exp": exp, "iat": exp - 3600, "kind": "full"}
    assert svc.token_kind(jwt.encode(claims, SECRET, algorithm="HS256")) == "full"
    # any HMAC, as golang-jwt's SigningMethodHMAC check allowed
    assert svc.token_kind(jwt.encode(claims, SECRET, algorithm="HS512")) == "full"
    # no exp is accepted by golang-jwt v5 too; iat and aud are not checked
    assert svc.token_kind(jwt.encode({"kind": "full"}, SECRET, algorithm="HS256")) == "full"
    future_iat = {**claims, "iat": exp + 999, "aud": "x"}
    assert svc.token_kind(jwt.encode(future_iat, SECRET, algorithm="HS256")) == "full"

    rejected = [
        jwt.encode(claims, "another-secret-of-enough-length", algorithm="HS256"),
        jwt.encode({**claims, "exp": int(time.time()) - 1}, SECRET, algorithm="HS256"),
        jwt.encode({**claims, "nbf": exp}, SECRET, algorithm="HS256"),
        _forge({"alg": "none", "typ": "JWT"}, claims),
        _forge({"alg": "RS256", "typ": "JWT"}, claims, b"sig"),
        "not.a.token",
        "",
    ]
    for token in rejected:
        assert svc.token_kind(token) is None, token
    assert svc.token_kind(jwt.encode({"exp": exp}, SECRET, algorithm="HS256")) is None
    assert svc.token_kind(jwt.encode({**claims, "kind": 1}, SECRET, algorithm="HS256")) is None


def test_validate_totp_like_pquerna() -> None:
    totp = pyotp.TOTP(TOTP_SECRET)
    now = int(time.time())
    assert validate_totp(totp.at(now), TOTP_SECRET)
    assert validate_totp(totp.at(now - 30), TOTP_SECRET)
    assert validate_totp(totp.at(now + 30), TOTP_SECRET)
    assert validate_totp(f" {totp.at(now)}\n", TOTP_SECRET)
    assert validate_totp(totp.at(now), TOTP_SECRET.lower())
    two_steps = {totp.at(now - 60), totp.at(now + 60)} - {totp.at(now + d) for d in (-30, 0, 30)}
    assert not any(validate_totp(code, TOTP_SECRET) for code in two_steps)
    assert not validate_totp(totp.at(now)[:5], TOTP_SECRET)
    assert not validate_totp(totp.at(now) + "0", TOTP_SECRET)
    assert not validate_totp("123456", "test-totp-secret")  # not base32


# handlers


def client(svc: AuthService | None = None) -> httpx.AsyncClient:
    app = build(config())
    if svc is not None:
        app.state.container.provide(svc)
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://t")


@pytest.mark.parametrize(("password", "kind"), [("Abc123..", "tmp"), ("semi-pass", "semi")])
async def test_login_returns_token_and_kind(password: str, kind: str) -> None:
    async with client() as c:
        resp = await c.post("/login", json={"password": password})
    assert resp.status_code == 200
    assert resp.headers["content-type"] == "application/json"
    assert resp.text.startswith(f'{{"kind":"{kind}","token":"ey')
    assert resp.text.endswith('"}\n')
    AuthService(config()).validate_token(resp.json()["token"], kind)


@pytest.mark.parametrize(
    ("body", "status", "expected"),
    [
        ("not json", 400, '{"error":"Invalid Request"}\n'),
        ('{"password": 123}', 400, '{"error":"Invalid Request"}\n'),
        ("[]", 400, '{"error":"Invalid Request"}\n'),
        ("", 400, '{"error":"Invalid Request"}\n'),
        ('{"password": "wrong"}', 401, '{"error":"invalid password"}\n'),
        ("null", 401, '{"error":"invalid password"}\n'),
        ('{"password": null, "extra": 1}', 401, '{"error":"invalid password"}\n'),
    ],
)
async def test_login_errors(body: str, status: int, expected: str) -> None:
    async with client() as c:
        resp = await c.post("/login", content=body)
    assert (resp.status_code, resp.text) == (status, expected)


async def test_login_2fa_with_current_code_returns_full_token() -> None:
    async with client() as c:
        tmp = (await c.post("/login", json={"password": "Abc123.."})).json()["token"]
        code = pyotp.TOTP(TOTP_SECRET).now()
        resp = await c.post("/login/2fa", json={"token": tmp, "code": code})
    assert resp.status_code == 200
    assert list(resp.json()) == ["token"]
    AuthService(config()).validate_token(resp.json()["token"], "full")


async def test_login_2fa_errors() -> None:
    svc = AuthService(config())
    tmp, _ = svc.login("Abc123..")
    semi, _ = svc.login("semi-pass")
    cases = [
        ("not json", 400, '{"error":"Invalid Request"}\n'),
        (json.dumps({"token": "bad-token", "code": "123456"}), 401, '{"error":"invalid token"}\n'),
        (json.dumps({"token": semi, "code": "123456"}), 401, '{"error":"invalid token"}\n'),
        (json.dumps({"token": tmp, "code": "000000x"}), 401, '{"error":"invalid 2fa code"}\n'),
    ]
    async with client() as c:
        for body, status, expected in cases:
            resp = await c.post("/login/2fa", content=body)
            assert (resp.status_code, resp.text) == (status, expected), body


# compatibility with the Go service, while its code is in the tree

GO_HELPER = """package main

import (
\t"fmt"
\t"os"
\t"time"

\t"gv-api/internal/auth"
\t"gv-api/internal/config"
)

func main() {
\tsvc := auth.NewService(&config.Config{JwtSecret: os.Args[2]}, nil)
\tif os.Args[1] == "issue" {
\t\ttok, _ := svc.GenerateToken(time.Hour, os.Args[3])
\t\tfmt.Print(tok)
\t\treturn
\t}
\tif svc.ValidateToken(os.Args[4], os.Args[3]) != nil {
\t\tfmt.Print("invalid")
\t\treturn
\t}
\tfmt.Print("valid")
}
"""


@pytest.fixture(scope="module")
def go_auth(tmp_path_factory: pytest.TempPathFactory) -> Callable[..., str]:
    go = shutil.which("go")
    if go is None or not (REPO / "internal" / "auth").is_dir():
        pytest.skip("Go toolchain or internal/auth not available")
    tmp = tmp_path_factory.mktemp("go")
    (tmp / "main.go").write_text(GO_HELPER)
    # A virtual package inside the module, so it may import internal/; nothing is written to REPO.
    virtual = REPO / "internal" / "auth" / "pycompat" / "main.go"
    (tmp / "overlay.json").write_text(json.dumps({"Replace": {str(virtual): str(tmp / "main.go")}}))
    binary = str(tmp / "goauth")
    overlay = str(tmp / "overlay.json")
    build_cmd = [go, "build", "-overlay", overlay, "-o", binary, "./internal/auth/pycompat"]
    subprocess.run(build_cmd, cwd=REPO, check=True)  # noqa: S603

    def run(*args: str) -> str:
        return subprocess.run(  # noqa: S603
            [binary, *args], capture_output=True, check=True, text=True
        ).stdout

    return run


def test_go_token_is_accepted_and_python_token_by_go(go_auth: Callable[..., str]) -> None:
    run = go_auth
    svc = AuthService(config())
    go_full = run("issue", SECRET, "full")
    assert svc.token_kind(go_full) == "full"
    assert svc.token_kind(run("issue", "other-secret", "full")) is None

    py_full = svc.generate_token(timedelta(days=30), "full")
    assert run("validate", SECRET, "full", py_full) == "valid"
    assert run("validate", SECRET, "semi", py_full) == "invalid"
    py_semi, _ = svc.login("semi-pass")
    assert run("validate", SECRET, "semi", py_semi) == "valid"
    assert run("validate", "other-secret", "full", py_full) == "invalid"
