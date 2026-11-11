"""The report, the restore runner and the whole failover flow against real databases."""

import asyncio
import gzip
import json
import shutil
import time
from collections.abc import AsyncIterator, Awaitable, Callable
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

import asyncpg
import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.db.engine import create_engine
from gv_api.failover import FailoverController
from gv_api.failover.controller import Params, Store
from gv_api.failover.setup import database_url
from gv_api.report import build
from gv_api.restore import BadDumpError, Request, Runner

from .conftest import require_db_url

pytestmark = pytest.mark.skipif(
    shutil.which("psql") is None or shutil.which("pg_dump") is None,
    reason="psql and pg_dump are needed",
)


class Server:
    """Scratch databases on the test server, dropped afterwards."""

    def __init__(self, admin: AsyncEngine) -> None:
        self.admin = admin
        self.url = require_db_url()
        self.created: list[str] = []
        self.engines: list[AsyncEngine] = []

    def name(self, prefix: str) -> str:
        return f"{prefix}_{time.time_ns()}"

    async def create(self, db: str, sql: str = "") -> AsyncEngine:
        await self.drop(db)
        async with self.admin.connect() as conn:
            await conn.execute(text(f'CREATE DATABASE "{db}"'))
        self.created += [db, db + "_restore_base", db + "_restore_ec2", db + "_pre_restore"]
        engine = self.engine(db)
        if sql:
            await self.run(db, sql)
        return engine

    def engine(self, db: str) -> AsyncEngine:
        engine = create_engine(database_url(self.url, db), pool_size=1)
        self.engines.append(engine)
        return engine

    async def run(self, db: str, sql: str) -> None:
        conn = await asyncpg.connect(database_url(self.url, db).split("?")[0])
        try:
            await conn.execute(sql)
        finally:
            await conn.close()

    async def names(self, db: str) -> list[str]:
        conn = await asyncpg.connect(database_url(self.url, db).split("?")[0])
        try:
            return [r[0] for r in await conn.fetch("SELECT name FROM parents ORDER BY id")]
        finally:
            await conn.close()

    async def drop(self, db: str) -> None:
        async with self.admin.connect() as conn:
            await conn.execute(text(f'DROP DATABASE IF EXISTS "{db}" WITH (FORCE)'))

    async def close(self) -> None:
        for engine in self.engines:
            await engine.dispose()
        for db in self.created:
            await self.drop(db)


@pytest.fixture
async def server(engine: AsyncEngine) -> AsyncIterator[Server]:
    s = Server(engine)
    yield s
    await s.close()


# Report

MINI = """
CREATE TABLE parents  (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE children (id SERIAL PRIMARY KEY,
                       parent_id INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE,
                       label TEXT NOT NULL);
CREATE TABLE wallets  (id SERIAL PRIMARY KEY, name TEXT NOT NULL, total NUMERIC NOT NULL DEFAULT 0);
CREATE TABLE entries  (id SERIAL PRIMARY KEY, wallet_id INT NOT NULL REFERENCES wallets(id),
                       amount NUMERIC NOT NULL);
CREATE TABLE notes    (text TEXT);
CREATE FUNCTION add_to_wallet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN UPDATE wallets SET total = total + NEW.amount WHERE id = NEW.wallet_id; RETURN NULL; END $$;
CREATE TRIGGER entries_total AFTER INSERT ON entries FOR EACH ROW EXECUTE FUNCTION add_to_wallet();
INSERT INTO parents (name) VALUES ('one'), ('two'), ('three'), ('four');
INSERT INTO children (parent_id, label) VALUES (1, 'c1');
INSERT INTO wallets (name) VALUES ('bank');
"""


async def report(server: Server, home_sql: str, ec2_sql: str, skip: tuple[str, ...] = ()) -> Any:
    dbs = []
    for side, extra in (("base", ""), ("home", home_sql), ("ec2", ec2_sql)):
        dbs.append(await server.create(server.name(f"report_{side}"), MINI + extra))
    return await build(*dbs, ignore={"wallets": ["total"]}, skip=skip)


def found(rep: Any, table: str, op: str) -> list[Any]:
    return [i for i in rep.items if i.table == table and i.op == op]


async def test_nothing_changed_on_home_is_an_empty_report(server: Server) -> None:
    rep = await report(
        server,
        "",
        "INSERT INTO parents (name) VALUES ('only-ec2');"
        " UPDATE parents SET name = 'ec2' WHERE id = 1",
    )
    assert rep.items == []


async def test_every_kind_of_change_home_made(server: Server) -> None:
    rep = await report(
        server,
        """INSERT INTO parents (name) VALUES ('home-new');
           UPDATE parents SET name = 'home-two' WHERE id = 2;
           DELETE FROM parents WHERE id = 3""",
        "",
    )
    assert len(rep.items) == 3
    [inserted] = found(rep, "parents", "insert")
    assert (inserted.home["name"], inserted.base, inserted.ec2, inserted.key) == (
        "home-new",
        None,
        None,
        "[5]",
    )
    [updated] = found(rep, "parents", "update")
    assert (updated.base["name"], updated.home["name"], updated.ec2["name"]) == (
        "two",
        "home-two",
        "two",
    )
    [deleted] = found(rep, "parents", "delete")
    assert (deleted.home, deleted.ec2["name"]) == (None, "three")


async def test_what_ec2_already_holds_is_left_out(server: Server) -> None:
    same = "UPDATE parents SET name = 'same' WHERE id = 2; DELETE FROM parents WHERE id = 3"
    assert (await report(server, same, same)).items == []


async def test_a_row_both_sides_edited_shows_ec2s_version(server: Server) -> None:
    rep = await report(
        server,
        "UPDATE parents SET name = 'home' WHERE id = 2",
        "UPDATE parents SET name = 'ec2' WHERE id = 2",
    )
    [item] = rep.items
    assert (item.home["name"], item.ec2["name"]) == ("home", "ec2")


async def test_a_change_names_the_row_it_points_at(server: Server) -> None:
    rep = await report(server, "INSERT INTO children (parent_id, label) VALUES (2, 'c2')", "")
    [item] = found(rep, "children", "insert")
    assert (item.related["parent_id"].table, item.related["parent_id"].row["name"]) == (
        "parents",
        "two",
    )


async def test_columns_a_trigger_derives_are_not_compared(server: Server) -> None:
    rep = await report(server, "INSERT INTO entries (wallet_id, amount) VALUES (1, 50)", "")
    assert len(found(rep, "entries", "insert")) == 1
    assert found(rep, "wallets", "update") == []


async def test_keyless_and_skipped_tables(server: Server) -> None:
    rep = await report(
        server, "INSERT INTO notes VALUES ('lost'); UPDATE parents SET name = 'x'", "", ("parents",)
    )
    assert (rep.skipped, rep.items) == (["notes"], [])


# Restore

SCHEMA = """
CREATE TABLE parents  (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE children (id SERIAL PRIMARY KEY,
                       parent_id INT NOT NULL REFERENCES parents(id) ON DELETE CASCADE,
                       label TEXT NOT NULL);
INSERT INTO parents (name) VALUES ('one'), ('two'), ('three');
INSERT INTO children (parent_id, label) VALUES (1, 'c1'), (2, 'c2');
"""
HOME_CHANGES = """
INSERT INTO parents (name) VALUES ('home-new');
UPDATE parents SET name = 'home-edit' WHERE id = 1;
UPDATE parents SET name = 'home-clash' WHERE id = 2;
"""
EC2_CHANGES = """
INSERT INTO parents (name) VALUES ('ec2-new');
UPDATE parents SET name = 'ec2-clash' WHERE id = 2;
"""
HOME_NAMES = ["home-edit", "home-clash", "three", "home-new"]
EC2_NAMES = ["one", "ec2-clash", "three", "ec2-new"]


def gz(path: Path, sql: str) -> Path:
    path.write_bytes(gzip.compress(sql.encode()))
    return path


class Env:
    def __init__(self, server: Server, tmp: Path, home: str) -> None:
        self.server = server
        self.dir = tmp
        self.home = home
        self.base = gz(tmp / "base.sql.gz", SCHEMA)
        self.ec2 = gz(tmp / "ec2.sql.gz", SCHEMA + EC2_CHANGES)
        self.released = 0

    def runner(
        self, last_alive: Callable[[AsyncEngine], Awaitable[datetime | None]] | None = None
    ) -> Runner:
        async def release() -> None:
            self.released += 1

        return Runner(
            admin=self.server.admin,
            url=lambda db: database_url(self.server.url, db),
            home=self.home,
            dir=self.dir / "failback",
            last_alive=last_alive,
            release=release,
        )

    def request(self) -> Request:
        return Request(id="dump-1", base=self.base, ec2=self.ec2)


async def env(server: Server, tmp: Path, home_sql: str = SCHEMA + HOME_CHANGES) -> Env:
    home = server.name("restore")
    await server.create(home, home_sql)
    return Env(server, tmp, home)


async def test_restores_ec2_and_reports_what_home_had(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    out = await e.runner().run(e.request())
    assert (out.swapped, e.released) == (True, 1)
    assert await server.names(e.home) == EC2_NAMES, "EC2's data as it is"
    assert await server.names(e.home + "_pre_restore") == HOME_NAMES, "the old home is kept"
    assert (tmp_path / "failback" / "swapped-dump-1").exists()
    assert out.report is not None and out.report.error is None
    seen = sorted(f"{i.op} {i.home['name']}" for i in out.report.items)
    assert seen == ["insert home-new", "update home-clash", "update home-edit"]
    assert out.pre_restore is not None
    assert b"home-new" in gzip.decompress(out.pre_restore.read_bytes())
    async with server.admin.connect() as conn:
        scratch = await conn.scalar(
            text("SELECT count(*) FROM pg_database WHERE datname = ANY(:dbs)"),
            {"dbs": [e.home + "_restore_base", e.home + "_restore_ec2"]},
        )
    assert scratch == 0


async def test_swaps_while_the_app_keeps_reconnecting(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    await server.create(e.home + "_pre_restore")
    stop = asyncio.Event()
    connects = 0
    url = database_url(server.url, e.home).split("?")[0]

    async def client() -> None:
        nonlocal connects
        while not stop.is_set():
            try:
                conn = await asyncpg.connect(url)
            except Exception:
                await asyncio.sleep(0.01)
                continue
            connects += 1
            try:
                await conn.execute("SELECT pg_sleep(0.2)")
            except Exception:  # noqa: S110 - terminated by the swap
                pass
            finally:
                await conn.close()

    clients = [asyncio.create_task(client()) for _ in range(8)]
    try:
        out = await e.runner().run(e.request())
    finally:
        stop.set()
        await asyncio.gather(*clients)
    assert out.swapped
    assert await server.names(e.home) == EC2_NAMES
    async with server.admin.connect() as conn:
        rows = await conn.execute(
            text("SELECT datallowconn FROM pg_database WHERE datname = ANY(:dbs)"),
            {"dbs": [e.home, e.home + "_pre_restore"]},
        )
        assert [r[0] for r in rows] == [True, True]
    assert connects > 8, "the clients kept reconnecting"


async def test_an_empty_home_takes_ec2_as_is(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path, "")
    out = await e.runner().run(e.request())
    assert out.report is not None and out.report.items == []
    assert await server.names(e.home) == EC2_NAMES


async def test_a_report_that_cannot_be_built_does_not_stop_it(
    server: Server, tmp_path: Path
) -> None:
    e = await env(server, tmp_path)
    e.base = gz(tmp_path / "base.sql.gz", "CREATE TABLE unrelated (id INT PRIMARY KEY);")
    out = await e.runner().run(e.request())
    assert out.swapped and out.report is not None and out.report.error
    assert await server.names(e.home) == EC2_NAMES


async def test_reads_homes_last_heartbeat_before_the_swap(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    alive = datetime(2026, 8, 1, 10, tzinfo=UTC)
    seen = []

    async def hook(home: AsyncEngine) -> datetime:
        async with home.connect() as conn:
            seen.append(await conn.scalar(text("SELECT name FROM parents WHERE id = 1")))
        return alive

    out = await e.runner(hook).run(e.request())
    assert (out.last_alive, seen) == (alive, ["home-edit"])


async def test_a_failing_heartbeat_read_does_not_stop_it(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)

    async def hook(_: AsyncEngine) -> datetime:
        raise RuntimeError("no table")

    out = await e.runner(hook).run(e.request())
    assert (out.swapped, out.last_alive) == (True, None)


async def test_a_truncated_dump_changes_nothing(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    raw = e.ec2.read_bytes()
    e.ec2.write_bytes(raw[: len(raw) // 2])
    with pytest.raises(BadDumpError):
        await e.runner().run(e.request())
    assert await server.names(e.home) == HOME_NAMES
    assert not (tmp_path / "failback").exists(), "nothing was written"


async def test_a_dump_psql_cannot_load_fails(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    e.ec2 = gz(tmp_path / "ec2.sql.gz", "SELECT * FROM nowhere;")
    with pytest.raises(RuntimeError, match=r"load ec2\.sql\.gz"):
        await e.runner().run(e.request())
    assert await server.names(e.home) == HOME_NAMES


async def test_a_repeated_run_is_already_done(server: Server, tmp_path: Path) -> None:
    e = await env(server, tmp_path)
    await e.runner().run(e.request())
    out = await e.runner().run(e.request())
    assert (out.already_done, out.report) == (True, None)
    assert await server.names(e.home) == EC2_NAMES


# The whole flow: the push, the downloads, the real runner and the swap

FLOW = """
CREATE TABLE parents (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO parents (name) VALUES ('one'), ('two'), ('three');
"""
FLOW_HOME = (
    FLOW
    + """
UPDATE parents SET name = 'home-clash' WHERE id = 2;
INSERT INTO parents (name) VALUES ('home-new');
"""
)
FLOW_EC2 = (
    FLOW
    + """
SELECT setval('parents_id_seq', 1000000);
UPDATE parents SET name = 'ec2-clash' WHERE id = 2;
INSERT INTO parents (name) VALUES ('ec2-new');
"""
)


class Params_:
    def __init__(self, values: dict[str, str]) -> None:
        self.values = values

    async def get(self, name: str) -> str:
        return self.values[name]


class CopyStore:
    def __init__(self, dir: Path) -> None:
        self.dir = dir

    async def download(self, key: str, dest: Path) -> None:
        await asyncio.to_thread(shutil.copy, self.dir / Path(key).name, dest)


async def test_push_to_swap_through_the_real_runner(server: Server, tmp_path: Path) -> None:
    home = server.name("flow")
    home_engine = await server.create(home, FLOW_HOME)
    objects = tmp_path / "objects"
    objects.mkdir()
    gz(objects / "base.sql.gz", FLOW)
    gz(objects / "dump.sql.gz", FLOW_EC2)
    calls = {"release": 0, "after": 0}

    async def release() -> None:
        calls["release"] += 1
        await home_engine.dispose()

    async def after() -> None:
        calls["after"] += 1

    runner = Runner(
        admin=server.admin,
        url=lambda db: database_url(server.url, db),
        home=home,
        dir=tmp_path / "failover",
        release=release,
    )
    params = Params_(
        {
            "/home-lab-failover/state": json.dumps({"state": "FAILING_BACK"}),
            "/home-lab-failover/base": "gv-db/hourly/base.sql.gz",
            "/home-lab-failover/dump": "gv-db/hourly/dump.sql.gz",
        }
    )
    ctl = FailoverController(
        "t",
        cast(Params, params),
        cast(Store, CopyStore(objects)),
        runner,
        tmp_path / "failover",
        after,
    )
    await ctl.start()
    assert ctl.mode == "hold"
    ctl.start_merge()
    for _ in range(1000):
        if ctl.merge != "running":
            break
        await asyncio.sleep(0.01)
    assert (ctl.merge, ctl.error, ctl.mode) == ("done", "", "normal")
    assert calls == {"release": 1, "after": 1}
    assert await server.names(home) == ["one", "ec2-clash", "three", "ec2-new"]
    assert ctl.report is not None
    dumped = ctl.report.model_dump_json()
    assert "home-clash" in dumped and "home-new" in dumped
