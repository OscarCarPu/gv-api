"""CORS settings for the browser clients (spec http-platform)."""

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
