import io
import json
import logging
from collections.abc import Iterator

import pytest

from gv_api.core.logging import request_id_var, setup_logging


@pytest.fixture
def stream() -> Iterator[io.StringIO]:
    root = logging.getLogger()
    saved = (root.handlers[:], root.level)
    buf = io.StringIO()
    setup_logging(buf)
    yield buf
    root.handlers[:], root.level = saved


def lines(buf: io.StringIO) -> list[dict[str, object]]:
    return [json.loads(line) for line in buf.getvalue().splitlines()]


def test_one_json_object_per_line(stream: io.StringIO) -> None:
    logging.getLogger("gv").info("server starting", extra={"port": "8080"})
    logging.getLogger("gv").warning("careful")
    first, second = lines(stream)
    assert first["msg"] == "server starting"
    assert first["level"] == "INFO"
    assert first["port"] == "8080"
    assert isinstance(first["time"], str) and "T" in first["time"]
    assert second["level"] == "WARN"
    assert "request_id" not in first


def test_request_id_from_context(stream: io.StringIO) -> None:
    token = request_id_var.set("abc123")
    try:
        logging.getLogger("gv").error("Failed to get daily habits", extra={"error": "boom"})
    finally:
        request_id_var.reset(token)
    (line,) = lines(stream)
    assert line["request_id"] == "abc123"
    assert line["error"] == "boom"
    assert line["level"] == "ERROR"


def test_exception_adds_error_and_traceback(stream: io.StringIO) -> None:
    try:
        raise ValueError("bad")
    except ValueError:
        logging.getLogger("gv").exception("unhandled")
    (line,) = lines(stream)
    assert line["error"] == "bad"
    assert "ValueError" in str(line["traceback"])


def test_below_info_is_dropped_and_access_log_disabled(stream: io.StringIO) -> None:
    logging.getLogger("gv").debug("noise")
    logging.getLogger("uvicorn.access").info("GET / 200")
    assert lines(stream) == []
