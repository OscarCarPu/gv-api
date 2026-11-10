import json
import logging
import sys
from contextvars import ContextVar
from datetime import datetime
from typing import Any, TextIO

request_id_var: ContextVar[str | None] = ContextVar("request_id", default=None)

_STANDARD = set(logging.makeLogRecord({}).__dict__) | {
    "message",
    "asctime",
    "taskName",
    "color_message",  # uvicorn
}
_LEVELS = {"WARNING": "WARN", "CRITICAL": "ERROR"}


class RequestIDFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        rid = request_id_var.get()
        if rid is not None and not hasattr(record, "request_id"):
            record.request_id = rid
        return True


class JSONFormatter(logging.Formatter):
    """One JSON object per line, shaped like Go's slog JSON handler."""

    def format(self, record: logging.LogRecord) -> str:
        out: dict[str, Any] = {
            "time": datetime.fromtimestamp(record.created).astimezone().isoformat(),
            "level": _LEVELS.get(record.levelname, record.levelname),
            "msg": record.getMessage(),
        }
        for key, value in record.__dict__.items():
            if key not in _STANDARD and not key.startswith("_"):
                out[key] = value
        if record.exc_info and record.exc_info[1] is not None:
            out.setdefault("error", str(record.exc_info[1]))
            out["traceback"] = self.formatException(record.exc_info)
        return json.dumps(out, default=str)


def setup_logging(stream: TextIO | None = None, level: int = logging.INFO) -> None:
    handler = logging.StreamHandler(stream or sys.stdout)
    handler.setFormatter(JSONFormatter())
    handler.addFilter(RequestIDFilter())
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel(level)
    for name in ("uvicorn", "uvicorn.error"):
        logging.getLogger(name).handlers.clear()
        logging.getLogger(name).propagate = True
    logging.getLogger("uvicorn.access").disabled = True
    # httpx logs every request URL at INFO, and the Telegram URL carries the bot token.
    for name in ("httpx", "httpcore"):
        logging.getLogger(name).setLevel(logging.WARNING)
