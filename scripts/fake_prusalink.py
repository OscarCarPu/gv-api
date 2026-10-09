#!/usr/bin/env python3
"""Minimal PrusaLink stand-in for developing printers without the printer.

Implements the slice of the Buddy-firmware API that gv-api talks to, with the same HTTP Digest
challenge shape as the real Core One (MD5, **no qop** — which is what makes gv-api's
send-once upload flow valid). Stdlib only, same as librewolf_drive.py.

    python scripts/fake_prusalink.py

Then point gv-api at it:

    PRUSALINK_HOST=http://127.0.0.1:8899
    PRUSALINK_USER=maker
    PRUSALINK_PASSWORD=test1234

Uploaded files land in a temp dir (printed at startup) and are listed back, so upload → print →
delete round-trips for real.

Failure injection, to exercise the UI's error paths (toggle at runtime, no restart):

    curl 'http://127.0.0.1:8899/_fake?mode=507'      # no USB drive
    curl 'http://127.0.0.1:8899/_fake?mode=409'      # always conflict
    curl 'http://127.0.0.1:8899/_fake?mode=offline'  # refuse everything (503)
    curl 'http://127.0.0.1:8899/_fake?mode=ok'       # back to normal
    curl 'http://127.0.0.1:8899/_fake?slow=8'        # throttle uploads to ~8s
    curl 'http://127.0.0.1:8899/_fake?state=PRINTING'
    curl 'http://127.0.0.1:8899/_fake?stale_write=1' # reject the next write as a stale nonce
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import secrets
import shutil
import tempfile
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, unquote, urlparse

HOST = "127.0.0.1"
PORT = int(os.environ.get("FAKE_PRUSALINK_PORT", "8899"))
USER = os.environ.get("FAKE_PRUSALINK_USER", "maker")
PASSWORD = os.environ.get("FAKE_PRUSALINK_PASSWORD", "test1234")
REALM = "Printer API"
STORAGE = "usb"

UPLOAD_DIR = tempfile.mkdtemp(prefix="fake-prusalink-")

# Runtime-tweakable behaviour, driven by /_fake.
STATE = {
    "mode": "ok",  # ok | 507 | 409 | offline
    "slow": 0.0,  # seconds to spread an upload over
    "state": "IDLE",
    "nonce": secrets.token_hex(16),
    # When true, the nonce is rotated immediately after a challenge is issued, so the very next
    # request arrives stale. Self-clearing.
    "stale_once": False,
    # When true, the next otherwise-valid write is rejected as stale. This is the reliable way to
    # exercise authSend's re-challenge-and-replay branch: gv-api resolves storage before a write,
    # and that lookup would otherwise consume stale_once. Self-clearing.
    "stale_write": False,
}


def md5(s: str) -> str:
    return hashlib.md5(s.encode()).hexdigest()


def parse_auth(header: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for m in re.finditer(r'(\w+)=(?:"([^"]*)"|([^,]*))', header.replace("Digest ", "", 1)):
        out[m.group(1).lower()] = (m.group(2) or m.group(3) or "").strip()
    return out


def expected_response(method: str, uri: str, nonce: str) -> str:
    ha1 = md5(f"{USER}:{REALM}:{PASSWORD}")
    ha2 = md5(f"{method}:{uri}")
    return md5(f"{ha1}:{nonce}:{ha2}")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "FakePrusaLink/1.0"

    # ---- plumbing ----

    def log_message(self, format: str, *args) -> None:  # noqa: A002 — signature is the base's
        print(f"  {self.command} {self.path} -> {format % args}".rstrip())

    def _send(self, status: int, body: bytes = b"", ctype: str = "application/json") -> None:
        self.send_response(status)
        if body:
            self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)

    def _json(self, status: int, payload) -> None:
        self._send(status, json.dumps(payload).encode())

    def _challenge(self) -> None:
        """401 with a Digest challenge carrying no qop, exactly like Buddy firmware."""
        body = b'{"message":"Unauthorized"}'
        self.send_response(401)
        self.send_header(
            "WWW-Authenticate",
            f'Digest realm="{REALM}", nonce="{STATE["nonce"]}", algorithm=MD5',
        )
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

        if STATE["stale_once"]:
            STATE["stale_once"] = False
            STATE["nonce"] = secrets.token_hex(16)
            print("    (nonce rotated after challenge — next request will be stale)")

    def _authed(self, path: str) -> bool:
        header = self.headers.get("Authorization")
        if not header or not header.lower().startswith("digest"):
            return False
        parts = parse_auth(header)
        if parts.get("username") != USER:
            return False
        # Accept the current nonce only, so a stale-nonce retry can be exercised by rotating it.
        if parts.get("nonce") != STATE["nonce"]:
            return False
        return parts.get("response") == expected_response(self.command, path, parts["nonce"])

    def _read_body(self) -> bytes:
        length = int(self.headers.get("Content-Length") or 0)
        if length == 0:
            return b""
        if STATE["slow"]:
            # Read in chunks with delays so the browser-side progress bar is observable.
            chunks, read, per = [], 0, max(1, length // 20)
            delay = STATE["slow"] / 20
            while read < length:
                chunk = self.rfile.read(min(per, length - read))
                if not chunk:
                    break
                chunks.append(chunk)
                read += len(chunk)
                time.sleep(delay)
            return b"".join(chunks)
        return self.rfile.read(length)

    # ---- fixtures ----

    @staticmethod
    def _short_name(name: str) -> str:
        """Mimic FAT32 8.3 mangling: NAME~1.EXT, extension truncated to 3 chars.

        This matters: a real .bgcode is listed as LIGHTH~1.BGC, and an allowlist without .bgc
        silently dropped every bgcode file from the UI.
        """
        stem, _, ext = name.rpartition(".")
        stem = stem or name
        return f"{stem[:6].upper()}~1.{ext[:3].upper()}" if ext else stem[:8].upper()

    def _files_payload(self) -> dict:
        # Buddy firmware reports NO per-file size here and spells read-only as "ro".
        children = []
        for name in sorted(os.listdir(UPLOAD_DIR)):
            children.append(
                {
                    "name": self._short_name(name),
                    "display_name": name,
                    "type": "PRINT_FILE",
                    "ro": False,
                    "m_timestamp": int(os.path.getmtime(os.path.join(UPLOAD_DIR, name))),
                }
            )
        return {"name": STORAGE, "type": "FOLDER", "children": children}

    def _resolve(self, requested: str) -> str | None:
        """Map either the real name or its 8.3 short name back to a stored file."""
        for name in os.listdir(UPLOAD_DIR):
            if requested == name or requested == self._short_name(name):
                return name
        return None

    def _file_info(self, name: str) -> dict:
        """The per-file endpoint DOES report size — that is where the UI has to get it."""
        return {
            "name": self._short_name(name),
            "display_name": name,
            "type": "PRINT_FILE",
            "ro": False,
            "size": os.path.getsize(os.path.join(UPLOAD_DIR, name)),
            "m_timestamp": int(os.path.getmtime(os.path.join(UPLOAD_DIR, name))),
        }

    def _storage_payload(self) -> dict:
        available = STATE["mode"] != "507"
        used = sum(
            os.path.getsize(os.path.join(UPLOAD_DIR, f)) for f in os.listdir(UPLOAD_DIR)
        )
        total = 8 * 1024**3
        return {
            "storage_list": [
                {
                    "name": "USB drive",
                    "type": "USB",
                    "path": f"/{STORAGE}",
                    "available": available,
                    "read_only": False,
                }
            ]
        }

    def _status_payload(self) -> dict:
        return {
            "printer": {
                "state": STATE["state"],
                "temp_nozzle": 214.7,
                "target_nozzle": 215.0,
                "temp_bed": 59.8,
                "target_bed": 60.0,
                "axis_z": 4.2,
                "speed": 100,
                "flow": 100,
                "fan_hotend": 8400,
                "fan_print": 3100,
            }
        }

    # ---- control plane ----

    def _fake_control(self, query: dict[str, list[str]]) -> None:
        if "mode" in query:
            STATE["mode"] = query["mode"][0]
        if "slow" in query:
            STATE["slow"] = float(query["slow"][0])
        if "state" in query:
            STATE["state"] = query["state"][0]
        if "rotate_nonce" in query:
            STATE["nonce"] = secrets.token_hex(16)
        if "stale_once" in query:
            STATE["stale_once"] = query["stale_once"][0] != "0"
        if "stale_write" in query:
            STATE["stale_write"] = query["stale_write"][0] != "0"
        self._json(200, {k: v for k, v in STATE.items() if k != "nonce"})

    # ---- verbs ----

    def _dispatch(self) -> None:
        parsed = urlparse(self.path)
        path = parsed.path
        query = parse_qs(parsed.query)

        if path == "/_fake":
            self._fake_control(query)
            return

        if STATE["mode"] == "offline":
            self._send(503, b'{"message":"offline"}')
            return

        if not self._authed(path):
            # Drain any body so the connection stays usable for the retry.
            self._read_body()
            self._challenge()
            return

        if STATE["stale_write"] and self.command != "GET":
            STATE["stale_write"] = False
            self._read_body()
            STATE["nonce"] = secrets.token_hex(16)
            print("    (rejecting this write as stale — expect a re-challenged replay)")
            self._challenge()
            return

        # --- reads ---
        if self.command == "GET":
            if path == "/api/v1/status":
                self._json(200, self._status_payload())
            elif path == "/api/v1/job":
                if STATE["state"] == "PRINTING":
                    self._json(
                        200,
                        {
                            "id": 1,
                            "progress": 42.0,
                            "time_remaining": 1830,
                            "time_printing": 900,
                            "file": {"name": "job.bgcode", "display_name": "job.bgcode"},
                        },
                    )
                else:
                    self._send(204)
            elif path == "/api/printer":
                self._json(200, {"telemetry": {"material": "PLA"}})
            elif path == "/api/v1/storage":
                self._json(200, self._storage_payload())
            elif path.rstrip("/") == f"/api/v1/files/{STORAGE}":
                self._json(200, self._files_payload())
            elif path.startswith(f"/api/v1/files/{STORAGE}/"):
                requested = os.path.basename(unquote(path))
                stored = self._resolve(requested)
                if stored is None:
                    self._json(404, {"message": "not found"})
                else:
                    self._json(200, self._file_info(stored))
            else:
                self._json(404, {"message": "not found"})
            return

        # --- writes ---
        m = re.fullmatch(rf"/api/v1/files/{STORAGE}/(.+)", path)
        if not m:
            self._read_body()
            self._json(404, {"message": "not found"})
            return
        requested = os.path.basename(unquote(m.group(1)))
        name = self._resolve(requested) or requested
        target = os.path.join(UPLOAD_DIR, name)

        if self.command == "PUT":
            body = self._read_body()
            if STATE["mode"] == "507":
                self._json(507, {"message": "Insufficient storage"})
                return
            overwrite = self.headers.get("Overwrite") == "?1"
            if (os.path.exists(target) or STATE["mode"] == "409") and not overwrite:
                self._json(409, {"message": "File already exists"})
                return
            with open(target, "wb") as fh:
                fh.write(body)
            print(f"    stored {name} ({len(body)} bytes) in {UPLOAD_DIR}")
            self._json(201, {"message": "created"})
            return

        if self.command == "POST":
            self._read_body()
            if not os.path.exists(target):
                self._json(404, {"message": "not found"})
            elif STATE["mode"] == "409" or STATE["state"] == "PRINTING":
                self._json(409, {"message": "Already printing"})
            else:
                STATE["state"] = "PRINTING"
                print(f"    printing {name}")
                self._send(204)
            return

        if self.command == "DELETE":
            if not os.path.exists(target):
                self._json(404, {"message": "not found"})
            elif STATE["mode"] == "409":
                self._json(409, {"message": "File is printing"})
            else:
                os.remove(target)
                print(f"    deleted {name}")
                self._send(204)
            return

        self._json(405, {"message": "method not allowed"})

    do_GET = _dispatch
    do_PUT = _dispatch
    do_POST = _dispatch
    do_DELETE = _dispatch


def main() -> None:
    print(f"fake PrusaLink on http://{HOST}:{PORT}")
    print(f"  user/password : {USER} / {PASSWORD}")
    print(f"  uploads       : {UPLOAD_DIR}")
    print(f"  control       : curl 'http://{HOST}:{PORT}/_fake?mode=507'")
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nstopping")
    finally:
        server.server_close()
        shutil.rmtree(UPLOAD_DIR, ignore_errors=True)


if __name__ == "__main__":
    main()
