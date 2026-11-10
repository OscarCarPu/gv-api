"""CORS for the browser clients: allowed origins, methods and headers, 300 s preflight cache."""

from starlette.datastructures import Headers, MutableHeaders
from starlette.types import ASGIApp, Message, Receive, Scope, Send

BROWSER_HEADERS = (
    "Content-Type",
    "Authorization",
    "X-Request-ID",
    "X-Device-ID",  # stable per-browser UUID from gv-web; unread, but sent on every call
    "X-File-Name",
    "X-Upload-Id",
    "X-Upload-Offset",
    "X-Upload-Total",
)
METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD")
EXPOSED = ("Content-Length", "X-Request-ID")
MAX_AGE = 300


def canonical(key: str) -> str:
    """Header name in canonical form: content-type -> Content-Type."""
    return "-".join(part[:1].upper() + part[1:].lower() for part in key.split("-"))


def parse_header_list(raw: str) -> list[str]:
    out: list[str] = []
    buf: list[str] = []
    upper = True
    for i, ch in enumerate(raw):
        if "a" <= ch <= "z":
            buf.append(ch.upper() if upper else ch)
        elif "A" <= ch <= "Z":
            buf.append(ch if upper else ch.lower())
        elif ch in "-_." or ch.isdigit():
            buf.append(ch)
        if ch in " ," or i == len(raw) - 1:
            if buf:
                out.append("".join(buf))
                buf = []
                upper = True
        else:
            upper = ch == "-"
    return out


class CORSMiddleware:
    def __init__(self, app: ASGIApp, allowed_origins: list[str]) -> None:
        self.app = app
        self.origins = {o.lower() for o in allowed_origins}
        self.all_origins = "*" in self.origins
        self.wildcards = [tuple(o.split("*", 1)) for o in self.origins if "*" in o and o != "*"]
        self.headers = {canonical(h) for h in (*BROWSER_HEADERS, "Origin")}
        self.exposed = ", ".join(canonical(h) for h in EXPOSED)

    def origin_allowed(self, origin: str) -> bool:
        if self.all_origins:
            return True
        origin = origin.lower()
        if origin in self.origins:
            return True
        return any(
            len(origin) >= len(a) + len(b) and origin.startswith(a) and origin.endswith(b)
            for a, b in self.wildcards
        )

    @staticmethod
    def method_allowed(method: str) -> bool:
        method = method.upper()
        return method == "OPTIONS" or method in METHODS

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        req = Headers(scope=scope)
        has_origin = "origin" in req
        origin = req.get("origin", "")

        if scope["method"] == "OPTIONS" and req.get("access-control-request-method") and has_origin:
            await self.preflight(req, origin, send)
            return

        extra: list[tuple[str, str]] = [("Vary", "Origin")]
        if has_origin and self.origin_allowed(origin) and self.method_allowed(scope["method"]):
            extra += [
                ("Access-Control-Allow-Origin", "*" if self.all_origins else origin),
                ("Access-Control-Expose-Headers", self.exposed),
            ]

        async def send_with_cors(message: Message) -> None:
            if message["type"] == "http.response.start":
                headers = MutableHeaders(scope=message)
                for key, value in extra:
                    if key == "Vary":
                        headers.append(key, value)
                    else:
                        headers[key] = value
            await send(message)

        await self.app(scope, receive, send_with_cors)

    async def preflight(self, req: Headers, origin: str, send: Send) -> None:
        headers: list[tuple[str, str]] = [
            ("Vary", "Origin"),
            ("Vary", "Access-Control-Request-Method"),
            ("Vary", "Access-Control-Request-Headers"),
        ]
        method = req.get("access-control-request-method", "")
        requested = parse_header_list(req.get("access-control-request-headers", ""))
        if (
            self.origin_allowed(origin)
            and self.method_allowed(method)
            and all(canonical(h) in self.headers for h in requested)
        ):
            headers.append(("Access-Control-Allow-Origin", "*" if self.all_origins else origin))
            headers.append(("Access-Control-Allow-Methods", method.upper()))
            if requested:
                headers.append(("Access-Control-Allow-Headers", ", ".join(requested)))
            headers.append(("Access-Control-Max-Age", str(MAX_AGE)))
        raw = [(k.lower().encode(), v.encode()) for k, v in headers]
        raw.append((b"content-length", b"0"))
        await send({"type": "http.response.start", "status": 200, "headers": raw})
        await send({"type": "http.response.body", "body": b""})
