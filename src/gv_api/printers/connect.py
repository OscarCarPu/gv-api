"""Prusa Connect (unofficial): print history, file metadata and the chamber, read with the owner's
Prusa account. The login is the web flow (OAuth with PKCE); its refresh token is kept in
PRUSA_CONNECT_TOKEN_FILE as {"refresh_token": "..."} so a session survives restarts.
"""

import asyncio
import base64
import hashlib
import html
import json
import logging
import os
import posixpath
import re
import secrets
import time
from collections.abc import Awaitable, Callable
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, quote, urlencode, urljoin, urlsplit

import httpx

from gv_api.core.http import ApiError
from gv_api.printers.config import Printer
from gv_api.printers.link import first_text, items, num, obj, text
from gv_api.printers.meta import done_fraction, parse_print_meta, scale_used
from gv_api.printers.schemas import Chamber, PrintHistoryEntry, PrintMeta

log = logging.getLogger("gv_api")

DEFAULT_BASE = "https://connect.prusa3d.com"
DEFAULT_ACCOUNT = "https://account.prusa3d.com"
CLIENT_ID = "MRHTlZhZqkNrrQ6FUPtjyusAz8nc59ErHXP8XkS4"
SCOPE = "basic_info user_operations email_lists openid connect"
REDIRECT_URI = DEFAULT_BASE + "/login/auth-callback"
TIMEOUT = 15.0
TOKEN_MARGIN = 60.0
HISTORY_LIMIT = 10
FILES_PAGE = 100
FILES_PAGES = 10
CHAMBER_TTL = 10.0

_CSRF = re.compile(r'name="csrfmiddlewaretoken"[^>]*value="([^"]*)"')
_NEXT = re.compile(r'name="next"[^>]*value="([^"]*)"')

UNEXPECTED = "Prusa Connect returned something unexpected"

AskCode = Callable[[], Awaitable[str]]


class LoginError(Exception):
    pass


VERIFICATION_NEEDED = "Prusa wants a code sent by email, run `make prusa-login` in gv-api"


def _pkce_pair() -> tuple[str, str]:
    verifier = base64.urlsafe_b64encode(secrets.token_bytes(64)).rstrip(b"=").decode()
    digest = hashlib.sha256(verifier.encode()).digest()
    return verifier, base64.urlsafe_b64encode(digest).rstrip(b"=").decode()


def _location(res: httpx.Response) -> str | None:
    loc = res.headers.get("location")
    return urljoin(str(res.url), loc) if loc else None


def _form(body: str, error: str, **fields: str) -> dict[str, str]:
    csrf = _CSRF.search(body)
    if csrf is None:
        raise LoginError(error)
    form = {"csrfmiddlewaretoken": html.unescape(csrf[1]), **fields}
    if nxt := _NEXT.search(body):
        form["next"] = html.unescape(nxt[1])
    return form


def _json(data: bytes) -> Any:
    try:
        return json.loads(data)
    except ValueError:
        raise ApiError(502, UNEXPECTED) from None


class ConnectClient:
    def __init__(self, printer: Printer, transport: httpx.AsyncBaseTransport | None = None):
        self._user = printer.connect_user
        self._password = printer.connect_password
        self._printer_id = printer.connect_id
        self._base = printer.connect_base.rstrip("/") or DEFAULT_BASE
        self._account = printer.connect_account.rstrip("/") or DEFAULT_ACCOUNT
        self._token_file = printer.connect_token_file
        self._transport = transport
        self._client = httpx.AsyncClient(transport=transport, timeout=TIMEOUT)
        self._lock = asyncio.Lock()
        self._access = ""
        self._expires = 0.0
        self._refresh = ""
        self._chamber: Chamber | None = None
        self._chamber_at = 0.0

    def configured(self) -> bool:
        return bool(self._user and self._password)

    async def aclose(self) -> None:
        await self._client.aclose()

    # session

    async def _token(self) -> str:
        async with self._lock:
            if self._access and time.monotonic() < self._expires:
                return self._access
            tried = {""}
            for refresh in (self._refresh, self._read_refresh()):
                if refresh in tried:
                    continue
                tried.add(refresh)
                form = {"grant_type": "refresh_token", "refresh_token": refresh}
                try:
                    self._store(await self._exchange({**form, "client_id": CLIENT_ID}))
                except LoginError:
                    continue
                return self._access
            self._refresh = ""
            self._store(await self.login(None))
            return self._access

    def _read_refresh(self) -> str:
        if not self._token_file:
            return ""
        try:
            saved = json.loads(Path(self._token_file).read_text())
        except OSError, ValueError:
            return ""
        token = saved.get("refresh_token") if isinstance(saved, dict) else None
        return token if isinstance(token, str) else ""

    def _write_refresh(self) -> None:
        if not self._token_file or not self._refresh:
            return
        path = Path(self._token_file)
        try:
            path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w") as f:
                json.dump({"refresh_token": self._refresh}, f)
        except OSError as err:
            log.warning(
                "printers: could not save the Prusa Connect token", extra={"error": str(err)}
            )

    def _store(self, token: dict[str, Any]) -> None:
        self._access = token["access_token"]
        expires_in = token.get("expires_in")
        expires_in = expires_in if isinstance(expires_in, int) else 0
        self._expires = time.monotonic() + expires_in - TOKEN_MARGIN
        if isinstance(refresh := token.get("refresh_token"), str) and refresh:
            self._refresh = refresh
            self._write_refresh()

    async def _exchange(self, form: dict[str, str]) -> dict[str, Any]:
        try:
            res = await self._client.post(self._account + "/o/token/", data=form)
        except httpx.HTTPError as err:
            raise LoginError(str(err) or "the token endpoint is unreachable") from err
        if res.status_code != 200:
            raise LoginError(f"token endpoint returned {res.status_code}")
        try:
            token = res.json()
        except ValueError:
            token = None
        if not isinstance(token, dict) or not token.get("access_token"):
            raise LoginError("token endpoint returned no access token")
        return token

    async def login(self, ask_code: AskCode | None) -> dict[str, Any]:
        """The web login. Without `ask_code`, a login that wants an emailed code fails."""
        verifier, challenge = _pkce_pair()
        params = {
            "response_type": "code",
            "client_id": CLIENT_ID,
            "code_challenge_method": "S256",
            "code_challenge": challenge,
            "redirect_uri": REDIRECT_URI,
            "scope": SCOPE,
        }
        authorize = self._account + "/o/authorize/?" + urlencode(params)
        async with httpx.AsyncClient(transport=self._transport, timeout=TIMEOUT) as web:
            login_url = _location(await web.get(authorize))
            if login_url is None:
                raise LoginError("Prusa did not ask for a login")
            page = await web.get(login_url)
            form = _form(
                page.text,
                "Prusa login form has changed",
                email=self._user,
                password=self._password,
            )
            res = await web.post(login_url, data=form, headers={"Referer": login_url})
            if res.status_code != 302:
                raise LoginError("Prusa rejected the account email or password")
            verify = _location(res)
            if verify is not None and urlsplit(verify).path.startswith("/login/verify"):
                if ask_code is None:
                    raise LoginError(VERIFICATION_NEEDED)
                await self._verify(web, verify, ask_code)
            back = _location(await web.get(authorize))
            code = parse_qs(urlsplit(back).query).get("code", [""])[0] if back else ""
            if not code:
                raise LoginError("Prusa did not return an authorization code")
        return await self._exchange(
            {
                "grant_type": "authorization_code",
                "code": code,
                "client_id": CLIENT_ID,
                "redirect_uri": REDIRECT_URI,
                "code_verifier": verifier,
            }
        )

    async def _verify(self, web: httpx.AsyncClient, target: str, ask_code: AskCode) -> None:
        page = await web.get(target)
        form = _form(page.text, "Prusa verification form has changed")
        form["otp_token"] = await ask_code()
        res = await web.post(target, data=form, headers={"Referer": target})
        if res.status_code != 302:
            raise LoginError("Prusa did not accept that code")

    async def save_login(self, ask_code: AskCode) -> None:
        if not self.configured():
            raise LoginError("PRUSA_CONNECT_USER and PRUSA_CONNECT_PASSWORD are not set")
        token = await self.login(ask_code)
        async with self._lock:
            self._store(token)

    # reads

    async def _get(self, path: str) -> bytes:
        async with asyncio.timeout(TIMEOUT):
            res = await self._authorized_get(path)
            if res.status_code == 401:
                # Revoked elsewhere (another login rotates it): refresh once and retry.
                self._access = ""
                res = await self._authorized_get(path)
        if res.status_code != 200:
            raise ApiError(502, f"Prusa Connect returned {res.status_code}")
        return res.content

    async def _authorized_get(self, path: str) -> httpx.Response:
        try:
            token = await self._token()
        except (LoginError, httpx.HTTPError) as err:
            raise ApiError(502, f"Could not log in to Prusa Connect: {err}") from err
        headers = {"Authorization": "Bearer " + token, "Accept": "application/json"}
        try:
            return await self._client.get(self._base + path, headers=headers)
        except httpx.HTTPError:
            raise ApiError(502, "Prusa Connect is unreachable") from None

    async def _printer_uuid(self) -> str:
        if self._printer_id:
            return self._printer_id
        printers = items(_json(await self._get("/app/printers")), "printers")
        uuid = text(printers[0], "uuid") if printers else ""
        if not uuid:
            raise ApiError(502, "No printer found on the Prusa Connect account")
        self._printer_id = uuid
        return uuid

    async def history(self) -> list[PrintHistoryEntry]:
        uuid = quote(await self._printer_uuid(), safe="")
        query = urlencode({"limit": HISTORY_LIMIT, "offset": 0})
        data = _json(await self._get(f"/app/printers/{uuid}/jobs?{query}"))
        return parse_connect_jobs(data)[:HISTORY_LIMIT]

    async def file_meta(self) -> dict[str, PrintMeta]:
        uuid = quote(await self._printer_uuid(), safe="")
        out: dict[str, PrintMeta] = {}
        for page in range(FILES_PAGES):
            query = urlencode({"limit": FILES_PAGE, "offset": page * FILES_PAGE})
            data = _json(await self._get(f"/app/printers/{uuid}/files?{query}"))
            files = items(data, "files")
            for f in files:
                if text(f, "type") == "PRINT_FILE" and text(f, "path"):
                    out[posixpath.basename(text(f, "path"))] = parse_print_meta(obj(f, "meta"))
            if len(files) < FILES_PAGE:
                break
        return out

    async def chamber(self) -> Chamber:
        if self._chamber is not None and time.monotonic() - self._chamber_at < CHAMBER_TTL:
            return self._chamber
        uuid = quote(await self._printer_uuid(), safe="")
        c = obj(_json(await self._get(f"/app/printers/{uuid}")), "chamber")
        self._chamber = Chamber(temp=num(c, "temp"), target_temp=num(c, "target_temp"))
        self._chamber_at = time.monotonic()
        return self._chamber


def parse_connect_jobs(data: Any) -> list[PrintHistoryEntry]:
    out = []
    for j in items(data, "jobs"):
        state = text(j, "state")
        entry = PrintHistoryEntry(state=state, time_printing=num(j, "time_printing"), file_name="")
        if isinstance(file := j.get("file"), dict):
            entry.file_name = first_text(text(file, "display_name"), text(file, "name")) or ""
            meta = parse_print_meta(obj(file, "meta"))
            entry.material = meta.material
            done = done_fraction(state, num(j, "progress"), entry.time_printing, meta.print_seconds)
            entry.filament_grams = scale_used(meta.filament_grams, done)
            entry.filament_meters = scale_used(meta.filament_meters, done)
        out.append(entry)
    return out
