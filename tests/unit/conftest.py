"""Shared unit-test fixtures."""

from collections.abc import AsyncIterator, Callable, Coroutine
from pathlib import Path
from typing import Any

import httpx
import pytest

from gv_api.printers import Printer, PrintersService, Settings

HOST = "http://printer.test"


@pytest.fixture
async def make_printers(tmp_path: Path) -> AsyncIterator[Callable[..., PrintersService]]:
    made: list[PrintersService] = []

    def build(
        fake: Callable[[httpx.Request], Coroutine[None, None, httpx.Response]] | None = None,
        *,
        settings: dict[str, Any] | None = None,
        **printer: Any,
    ) -> PrintersService:
        """`printer` overrides the printer's fields and `settings` the service's."""
        p = Printer(**{"id": "core-one", "name": "n", "model": "m", "host": HOST, **printer})
        defaults: dict[str, Any] = {
            "recordings_dir": tmp_path / "recordings",
            "uploads_dir": tmp_path / "uploads",
            "max_bytes": 1 << 30,
            "sign_key": b"secret",
            "ffmpeg": str(tmp_path / "no-ffmpeg"),
        }
        settings = Settings(printers=[p], **{**defaults, **(settings or {})})
        svc = PrintersService(settings, httpx.MockTransport(fake) if fake else None)
        made.append(svc)
        return svc

    yield build
    for svc in made:
        await svc.close()
