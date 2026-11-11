"""The failover controller and its routes over fake SSM, S3 and restore."""

import asyncio
import json
from datetime import UTC, datetime
from pathlib import Path
from types import SimpleNamespace
from typing import Any, cast

import httpx
import pytest
from fastapi import APIRouter
from starlette.responses import Response

from gv_api import failover
from gv_api.auth import AuthService
from gv_api.core.app import create_app
from gv_api.failover import FailoverController, HoldLock
from gv_api.failover.aws import NoParamError
from gv_api.failover.controller import Outages, Params, Restorer, Store
from gv_api.report import Report
from gv_api.report.schemas import Item
from gv_api.restore import Outcome, Request

from .test_tasks_router import AUTH, CFG, FULL, ping

TOKEN = "secret"
STATE, BASE, DUMP = (
    "/home-lab-failover/state",
    "/home-lab-failover/base",
    "/home-lab-failover/dump",
)
ALIVE = datetime(2026, 8, 1, 10, tzinfo=UTC)
EC2_UP = datetime(2026, 8, 1, 10, 20, tzinfo=UTC)
HOME_AT = datetime(2026, 8, 1, 11, tzinfo=UTC)
OUTAGE = {"ec2_up_at": "2026-08-01T10:20:00Z", "home_at": "2026-08-01T11:00:00Z"}


def state(s: str) -> str:
    return json.dumps({"state": s})


class FakeParams:
    def __init__(self, values: dict[str, str]) -> None:
        self.values = values
        self.err: Exception | None = None

    async def get(self, name: str) -> str:
        if self.err is not None:
            raise self.err
        if name not in self.values:
            raise NoParamError(name)
        return self.values[name]


class FakeStore:
    def __init__(self) -> None:
        self.keys: list[str] = []

    async def download(self, key: str, dest: Path) -> None:
        self.keys.append(key)
        await asyncio.to_thread(dest.write_text, key)


class FakeRestorer:
    def __init__(self, outcome: Any) -> None:
        self.outcome = outcome
        self.runs: list[Request] = []

    async def run(self, req: Request) -> Outcome:
        self.runs.append(req)
        result = self.outcome(len(self.runs))
        if asyncio.iscoroutine(result):
            result = await result
        if isinstance(result, Exception):
            raise result
        return cast(Outcome, result)


class FakeOutages:
    def __init__(self) -> None:
        self.calls: list[tuple[datetime | None, datetime, datetime]] = []

    async def record_failover(
        self, last_alive: datetime | None, ec2_up_at: datetime, home_at: datetime
    ) -> None:
        self.calls.append((last_alive, ec2_up_at, home_at))


def swapped(_: int) -> Outcome:
    return Outcome(swapped=True)


def reporting(_: int) -> Outcome:
    item = Item(table="parents", key="[2]", op="update", base={"name": "two"})
    return Outcome(swapped=True, report=Report(at=ALIVE, items=[item]))


def with_last_alive(_: int) -> Outcome:
    return Outcome(swapped=True, last_alive=ALIVE)


class Fixture:
    def __init__(self, tmp: Path, state_json: str, outcome: Any) -> None:
        values = {BASE: "gv-db/hourly/base.sql.gz", DUMP: "gv-db/hourly/dump.sql.gz"}
        if state_json:
            values[STATE] = state_json
        self.params = FakeParams(values)
        self.store = FakeStore()
        self.restorer = FakeRestorer(outcome)
        self.dir = tmp
        self.after = 0
        self.ctl = self.controller()

    def controller(self) -> FailoverController:
        async def after() -> None:
            self.after += 1

        return FailoverController(
            TOKEN,
            cast(Params, self.params),
            cast(Store, self.store),
            cast(Restorer, self.restorer),
            self.dir,
            after,
            retry=0.01,
        )

    async def do(
        self, method: str, path: str, token: str = TOKEN, body: Any = None
    ) -> httpx.Response:
        stub = APIRouter()

        @stub.get("/habits")
        @stub.post("/login")
        async def ok() -> Response:
            return Response(status_code=200)

        app = create_app(
            CFG,
            ping=ping,
            routers=[stub, failover.router, failover.report_router],
            middleware=[(HoldLock, {"controller": self.ctl})],
        )
        services = {AuthService: AUTH, FailoverController: self.ctl}
        app.state.container = SimpleNamespace(services=services)
        headers = dict(FULL)
        if token:
            headers["X-Failover-Token"] = token
        content = body if isinstance(body, str) or body is None else json.dumps(body)
        transport = httpx.ASGITransport(app=app)
        async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
            return await c.request(method, path, headers=headers, content=content)

    async def merge_status(self) -> str:
        return str((await self.do("GET", "/failover/status")).json()["merge"])

    async def wait_for(self, merge: str) -> None:
        for _ in range(400):
            if await self.merge_status() == merge:
                return
            await asyncio.sleep(0.005)
        raise AssertionError(f"merge never became {merge!r}")

    async def report(self) -> Any:
        resp = await self.do("GET", "/report", token="")
        assert resp.status_code == 200
        return resp.json()["report"]


@pytest.fixture
def make(tmp_path: Path) -> Any:
    def build(state_json: str = state("FAILED_OVER"), outcome: Any = swapped) -> Fixture:
        return Fixture(tmp_path, state_json, outcome)

    return build


async def test_routes_need_the_token(make: Any) -> None:
    f = make()
    for token in ("", "wrong"):
        resp = await f.do("POST", "/failover", token, {"mode": "hold"})
        assert (resp.status_code, resp.json()) == (401, {"error": "invalid failover token"})
        assert (await f.do("GET", "/failover/status", token)).status_code == 401
        assert (await f.do("POST", "/failover/outage", token, OUTAGE)).status_code == 401
    assert f.ctl.mode == "normal"


async def test_hold_only_when_the_state_is_not_normal(make: Any) -> None:
    f = make(state("NORMAL"))
    resp = await f.do("POST", "/failover", body={"mode": "hold"})
    assert (resp.status_code, resp.json()) == (409, {"error": "the failover state is NORMAL"})
    f.params.values[STATE] = state("FAILED_OVER")
    resp = await f.do("POST", "/failover", body={"mode": "hold"})
    assert (resp.status_code, resp.json()) == (200, {"mode": "hold", "merge": "", "error": None})


async def test_lock_answers_merge_pending_except_for_what_the_failover_needs(make: Any) -> None:
    f = make()
    assert (await f.do("GET", "/habits")).status_code == 200
    assert (await f.do("POST", "/failover", body={"mode": "hold"})).status_code == 200
    resp = await f.do("GET", "/habits")
    assert (resp.status_code, resp.json()) == (
        503,
        {"error": "Restoring data after the failover. Back in a moment.", "code": "merge_pending"},
    )
    for method, path in [
        ("GET", "/health"),
        ("POST", "/login"),
        ("GET", "/report"),
        ("GET", "/failover/status"),
    ]:
        assert (await f.do(method, path)).status_code != 503, path


async def test_merge_needs_the_right_state_and_a_dump(make: Any) -> None:
    f = make()
    resp = await f.do("POST", "/failover", body={"mode": "merge"})
    assert (resp.status_code, resp.json()) == (409, {"error": "the failover state is FAILED_OVER"})
    f.params.values[STATE] = state("FAILING_BACK")
    del f.params.values[DUMP]
    resp = await f.do("POST", "/failover", body={"mode": "merge"})
    assert (resp.status_code, resp.json()) == (409, {"error": "no failback dump yet"})
    resp = await f.do("POST", "/failover", body={"mode": "explode"})
    assert (resp.status_code, resp.json()) == (400, {"error": "mode must be hold or merge"})
    assert (await f.do("POST", "/failover", body="{")).json() == {"error": "invalid body"}
    assert f.restorer.runs == []


async def test_an_unreadable_state_is_a_500(make: Any) -> None:
    f = make()
    f.params.err = RuntimeError("no network")
    resp = await f.do("POST", "/failover", body={"mode": "hold"})
    assert (resp.status_code, resp.json()) == (500, {"error": "cannot read the failover state"})
    assert (await f.do("POST", "/failover/outage", body=OUTAGE)).status_code == 500


async def test_merge_downloads_both_dumps_runs_and_unlocks(make: Any) -> None:
    f = make(state("FAILING_BACK"))
    assert (await f.do("POST", "/failover", body={"mode": "merge"})).status_code == 200
    await f.wait_for("done")
    assert (f.ctl.mode, f.after) == ("normal", 1)
    assert sorted(f.store.keys) == ["gv-db/hourly/base.sql.gz", "gv-db/hourly/dump.sql.gz"]
    [req] = f.restorer.runs
    assert (req.id, req.base, req.ec2) == (
        "dump",
        f.dir / "downloads" / "base.sql.gz",
        f.dir / "downloads" / "dump.sql.gz",
    )


async def test_the_report_is_kept_until_dismissed(make: Any) -> None:
    f = make(state("FAILING_BACK"), reporting)
    assert await f.report() is None
    assert (await f.do("POST", "/failover", body={"mode": "merge"})).status_code == 200
    await f.wait_for("done")
    report = await f.report()
    assert report["items"][0]["table"] == "parents"

    f.ctl = f.controller()
    assert await f.report() is not None, "the report survives a restart"
    assert (await f.do("DELETE", "/report", token="")).status_code == 204
    assert await f.report() is None
    assert not (f.dir / "report.json").exists()


async def test_a_restore_already_done_keeps_the_earlier_report(make: Any) -> None:
    f = make(state("FAILING_BACK"), reporting)
    await f.do("POST", "/failover", body={"mode": "merge"})
    await f.wait_for("done")
    f.ctl = f.controller()
    f.restorer.outcome = lambda _: Outcome(swapped=True, already_done=True)
    f.params.values[STATE] = state("PENDING_MERGE")
    await f.ctl.start()
    await f.wait_for("done")
    assert len(f.restorer.runs) == 2
    assert await f.report() is not None


async def test_a_failed_merge_is_reported_and_can_be_retried(make: Any) -> None:
    f = make(
        state("FAILING_BACK"),
        lambda call: RuntimeError("disk full") if call == 1 else Outcome(swapped=True),
    )
    await f.do("POST", "/failover", body={"mode": "merge"})
    await f.wait_for("failed")
    assert (await f.do("GET", "/failover/status")).json()["error"] == "disk full"
    assert f.ctl.mode == "merge"
    await f.do("POST", "/failover", body={"mode": "merge"})
    await f.wait_for("done")


async def test_a_merge_does_not_run_twice_at_once(make: Any) -> None:
    release = asyncio.Event()

    async def slow(_: int) -> Outcome:
        await release.wait()
        return Outcome(swapped=True)

    f = make(state("FAILING_BACK"), slow)
    for _ in range(3):
        assert (await f.do("POST", "/failover", body={"mode": "merge"})).status_code == 200
    release.set()
    await f.wait_for("done")
    assert len(f.restorer.runs) == 1


@pytest.mark.parametrize(
    ("state_json", "mode", "runs"),
    [
        (state("NORMAL"), "normal", 0),
        ("", "normal", 0),
        (state("FAILED_OVER"), "hold", 0),
        (state("FAILING_BACK"), "hold", 0),
        (state("PENDING_MERGE"), "merge", 1),
    ],
)
async def test_start(make: Any, state_json: str, mode: str, runs: int) -> None:
    f = make(state_json)
    await f.ctl.start()
    assert f.ctl.mode == mode
    if runs:
        await f.wait_for("done")
    assert len(f.restorer.runs) == runs


async def test_start_holds_while_the_state_is_unreadable_then_lets_go(make: Any) -> None:
    f = make(state("NORMAL"))
    f.params.err = RuntimeError("no network")
    await f.ctl.start()
    assert f.ctl.mode == "hold"
    f.params.err = None
    for _ in range(200):
        if f.ctl.mode == "normal":
            break
        await asyncio.sleep(0.005)
    assert f.ctl.mode == "normal"


async def test_stop_cancels_the_retry(make: Any) -> None:
    f = make()
    f.params.err = RuntimeError("no network")
    await f.ctl.start()
    await f.ctl.stop()
    assert f.ctl.mode == "hold"


async def test_allowed(make: Any) -> None:
    f = make(state("NORMAL"))
    assert await f.ctl.allowed()
    f.params.err = RuntimeError("no network")
    assert await f.ctl.allowed(), "an unreadable state does not stop the worker"
    f.params.err = None
    f.params.values[STATE] = state("FAILED_OVER")
    assert not await f.ctl.allowed()
    assert f.ctl.mode == "hold", "a failover the push never reached holds the app"
    assert not await f.ctl.allowed()


async def test_an_unreadable_report_file_is_ignored(tmp_path: Path) -> None:
    (tmp_path / "report.json").write_text("{")
    f = Fixture(tmp_path, state("NORMAL"), swapped)
    assert f.ctl.report is None


# Outages


async def restored(make: Any) -> tuple[Fixture, FakeOutages]:
    f = make(state("FAILING_BACK"), with_last_alive)
    outages = FakeOutages()
    f.ctl.outages = cast(Outages, outages)
    await f.do("POST", "/failover", body={"mode": "merge"})
    await f.wait_for("done")
    f.params.values[STATE] = state("HOME_READY")
    return f, outages


@pytest.mark.parametrize(
    "body",
    [
        "not json",
        {},
        {"ec2_up_at": "2026-08-01T11:00:00Z", "home_at": "2026-08-01T10:20:00Z"},
        {"ec2_up_at": "2026-08-01T10:20:00Z", "home_at": "2999-01-01T00:00:00Z"},
    ],
)
async def test_outage_refuses_a_body_that_makes_no_sense(make: Any, body: Any) -> None:
    f, outages = await restored(make)
    resp = await f.do("POST", "/failover/outage", body=body)
    assert (resp.status_code, resp.json()) == (400, {"error": "invalid body"})
    assert outages.calls == []


async def test_outage_only_while_home_ready(make: Any) -> None:
    f, outages = await restored(make)
    f.params.values[STATE] = state("PENDING_MERGE")
    resp = await f.do("POST", "/failover/outage", body=OUTAGE)
    assert (resp.status_code, resp.json()) == (
        409,
        {"error": "the failover state is PENDING_MERGE"},
    )
    assert outages.calls == []


async def test_outage_without_the_uptime_history_is_503(make: Any) -> None:
    f = make(state("HOME_READY"))
    resp = await f.do("POST", "/failover/outage", body=OUTAGE)
    assert (resp.status_code, resp.json()) == (503, {"error": "uptime is not available"})


async def test_outage_records_with_homes_last_heartbeat_then_forgets_it(make: Any) -> None:
    f, outages = await restored(make)
    assert (await f.do("POST", "/failover/outage", body=OUTAGE)).status_code == 200
    assert (await f.do("POST", "/failover/outage", body=OUTAGE)).status_code == 200
    assert outages.calls == [(ALIVE, EC2_UP, HOME_AT), (None, EC2_UP, HOME_AT)]


async def test_outage_failure(make: Any) -> None:
    f, _ = await restored(make)

    class Failing:
        async def record_failover(self, *_: object) -> None:
            raise RuntimeError("db down")

    f.ctl.outages = cast(Outages, Failing())
    resp = await f.do("POST", "/failover/outage", body=OUTAGE)
    assert (resp.status_code, resp.json()) == (500, {"error": "cannot record the failover"})


async def test_beating(make: Any) -> None:
    held = make(state("FAILED_OVER"))
    await held.ctl.start()
    assert not await held.ctl.beating()

    f, _ = await restored(make)
    assert f.ctl.mode == "normal"
    assert not await f.ctl.beating(), "not until the state is NORMAL"
    f.params.values[STATE] = state("NORMAL")
    assert await f.ctl.beating()
    f.params.err = RuntimeError("no network")
    assert await f.ctl.beating()


# Wiring


def test_database_url_keeps_everything_but_the_database() -> None:
    from gv_api.failover.setup import database_url

    url = "postgresql://u:p@db:5432/gv?sslmode=disable"
    assert database_url(url, "postgres") == "postgresql://u:p@db:5432/postgres?sslmode=disable"


async def test_a_held_home_skips_the_migrations(monkeypatch: pytest.MonkeyPatch) -> None:
    from gv_api.main import build

    async def held(self: FailoverController) -> None:
        self.set_mode("hold")

    monkeypatch.setattr(FailoverController, "start", held)
    cfg = CFG.model_copy(
        update={
            "database_url": "postgresql://u:p@127.0.0.1:1/gv",
            "failover_token": TOKEN,
            "backup_s3_bucket": "bucket",
            "backup_interval_ms": 0,
        }
    )
    app = build(cfg)
    ctl = app.state.container.services[FailoverController]
    # The database is unreachable: starting at all means no migration ran.
    async with app.router.lifespan_context(app):
        assert ctl.mode == "hold"
        assert app.state.container.services[FailoverController] is ctl
