"""The advisory lock and pg_dump against the test database."""

import gzip
import io
import shutil

import pytest
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.backup import PgDump, PgLocker
from gv_api.backup.errors import RunningError

from .conftest import require_db_url


async def test_one_lock_at_a_time(db: AsyncEngine) -> None:
    locker = PgLocker(db)
    async with locker.hold():
        with pytest.raises(RunningError):
            async with locker.hold():
                pass
    async with locker.hold():
        pass


async def test_pg_dump() -> None:
    if shutil.which("pg_dump") is None:
        pytest.skip("pg_dump not installed")
    buf = io.BytesIO()
    with gzip.GzipFile(fileobj=buf, mode="wb") as out:
        await PgDump(require_db_url()).dump(out)
    assert b"PostgreSQL database dump" in gzip.decompress(buf.getvalue())


async def test_pg_dump_failure() -> None:
    if shutil.which("pg_dump") is None:
        pytest.skip("pg_dump not installed")
    with (
        gzip.GzipFile(fileobj=io.BytesIO(), mode="wb") as out,
        pytest.raises(RuntimeError, match="pg_dump: exit status"),
    ):
        await PgDump("postgresql://nobody@127.0.0.1:1/none").dump(out)
