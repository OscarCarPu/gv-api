"""UptimeService, the heartbeat, and the uptime router (semi tier, home side only)."""

import asyncio
from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from typing import Any, cast

import httpx
import pytest

from gv_api import uptime
from gv_api.auth import AuthService
from gv_api.core.app import create_app
from gv_api.core.deps import ON_FAILOVER
from gv_api.uptime import Heartbeat, UptimeService
from gv_api.uptime.errors import InvalidRangeError
from gv_api.uptime.schemas import Range
from gv_api.uptime.service import Repository

from .test_tasks_router import AUTH, CFG, FAIL, FULL, ping
from .test_tasks_service import FakeRepo

NOW = datetime(2026, 8, 31, 12, tzinfo=UTC)
SEMI = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "semi")}


def at(hours_ago: float) -> datetime:
    return NOW - timedelta(hours=hours_ago)


def rng(kind: str, start: float, end: float) -> Range:
    return Range(kind, at(start), at(end))


def svc(repo: FakeRepo) -> UptimeService:
    return UptimeService(cast(Repository, repo), lambda: NOW)


def lookback(overview: Any, name: str) -> Any:
    return next(r for r in overview.ranges if r.range == name)


async def test_nothing_covered_yet_has_no_percentage() -> None:
    o = await svc(FakeRepo(tracking_start=NOW, ranges=[])).overview()
    assert [r.uptime for r in o.ranges] == [None] * 4
    assert o.up_since == NOW


async def test_only_up_is_one_hundred() -> None:
    o = await svc(FakeRepo(tracking_start=at(10), ranges=[rng("up", 10, 0)])).overview()
    assert lookback(o, "all").uptime == 100
    assert o.up_since == at(10)


async def test_downtime_lowers_it_and_failover_does_not() -> None:
    ranges = [rng("up", 10, 6), rng("down", 6, 5), rng("failover", 5, 3), rng("up", 3, 0)]
    o = await svc(FakeRepo(tracking_start=at(10), ranges=ranges)).overview()
    everything = lookback(o, "all")
    assert everything.uptime == 90
    assert (everything.down_seconds, everything.failover_seconds) == (3600, 7200)
    assert o.up_since == at(3), "up since the last range that was not up"


async def test_a_lookback_cuts_the_range_and_keeps_the_start() -> None:
    start = at(24 * 400)
    o = await svc(FakeRepo(tracking_start=start, ranges=[])).overview()
    assert lookback(o, "month").range_start == NOW - timedelta(days=30)
    assert lookback(o, "year").range_start == NOW - timedelta(days=365)
    assert lookback(o, "all").range_start == start
    o = await svc(FakeRepo(tracking_start=at(10), ranges=[])).overview()
    assert lookback(o, "month").range_start == at(10)


async def test_windows_default_to_the_last_thirty_days() -> None:
    repo = FakeRepo(tracking_start=NOW - timedelta(days=365), ranges=[])
    report = await svc(repo).windows(None, None, None)
    assert repo.calls[1] == ("ranges", NOW - timedelta(days=30), NOW)
    assert (report.to, report.windows, report.uptime) == (NOW, [], None)


async def test_windows_clamp_a_future_end_and_an_early_start() -> None:
    repo = FakeRepo(tracking_start=at(10), ranges=[])
    report = await svc(repo).windows(at(100), NOW + timedelta(hours=48), None)
    assert (report.from_, report.to) == (at(10), NOW)


async def test_windows_reject_a_backwards_range() -> None:
    with pytest.raises(InvalidRangeError):
        await svc(FakeRepo(tracking_start=at(100))).windows(at(1), at(5), None)


async def test_windows_list_newest_first_clipped_and_count_downtimes() -> None:
    ranges = [rng("up", 20, 8), rng("down", 8, 7), rng("failover", 7, 4), rng("up", 4, 0)]
    report = await svc(FakeRepo(tracking_start=at(20), ranges=ranges)).windows(at(10), NOW, None)
    assert [w.kind for w in report.windows] == ["up", "failover", "down", "up"]
    assert report.windows[3].seconds == 2 * 3600
    assert report.windows[3].start_time == at(20), "a window keeps its own bounds"
    assert (report.outages, report.down_seconds, report.failover_seconds, report.up_seconds) == (
        1,
        3600,
        3 * 3600,
        6 * 3600,
    )
    assert (report.uptime, report.truncated) == (90, False)
    assert report.model_dump(mode="json")["from"] == "2026-08-31T02:00:00Z"


async def test_windows_flag_truncation_and_cap_the_limit() -> None:
    ranges = [rng("up", 20, 8), rng("down", 8, 7), rng("up", 7, 0)]
    report = await svc(FakeRepo(tracking_start=at(20), ranges=ranges)).windows(at(20), NOW, 2)
    assert (len(report.windows), report.truncated, report.outages) == (2, True, 1)
    report = await svc(FakeRepo(tracking_start=at(20), ranges=ranges)).windows(None, None, 10**6)
    assert len(report.windows) == 3


# Heartbeat


class Beats:
    def __init__(self, fail: bool = False) -> None:
        self.beats: list[datetime] = []
        self.fail = fail

    async def beat(self, now: datetime, gap: timedelta) -> None:
        self.beats.append(now)
        if self.fail:
            raise RuntimeError("db down")


async def run_for(heartbeat: Heartbeat, seconds: float) -> None:
    task = asyncio.create_task(heartbeat.run())
    await asyncio.sleep(seconds)
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task


async def test_heartbeat_beats_at_once_and_then_on_every_tick() -> None:
    beats = Beats()
    await run_for(Heartbeat(cast(Repository, beats), every=timedelta(milliseconds=10)), 0.05)
    assert len(beats.beats) >= 3


async def test_heartbeat_beats_right_at_start_even_with_a_long_interval() -> None:
    beats = Beats()
    await run_for(Heartbeat(cast(Repository, beats), every=timedelta(hours=1)), 0.02)
    assert len(beats.beats) == 1


async def test_a_closed_gate_writes_nothing() -> None:
    beats = Beats()

    async def closed() -> bool:
        return False

    hb = Heartbeat(cast(Repository, beats), every=timedelta(milliseconds=5), gate=closed)
    await run_for(hb, 0.03)
    assert beats.beats == []


async def test_a_failed_beat_does_not_stop_it(caplog: pytest.LogCaptureFixture) -> None:
    beats = Beats(fail=True)
    await run_for(Heartbeat(cast(Repository, beats), every=timedelta(milliseconds=10)), 0.05)
    assert len(beats.beats) >= 3
    assert "uptime: heartbeat failed" in caplog.messages


# Router


async def call(
    repo: FakeRepo, path: str, headers: dict[str, str] = SEMI, aws: bool = False
) -> tuple[int, Any]:
    app = create_app(CFG, ping=ping, routers=[uptime.router])
    cfg = SimpleNamespace(on_failover=aws)
    services = {AuthService: AUTH, UptimeService: svc(repo)}
    app.state.container = SimpleNamespace(services=services, cfg=cfg)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://t") as c:
        resp = await c.get(path, headers=headers)
    return resp.status_code, resp.json()


async def test_router_overview() -> None:
    status, body = await call(
        FakeRepo(tracking_start=at(10), ranges=[rng("up", 10, 0)]), "/domotics/uptime"
    )
    assert (status, body["up_since"], body["ranges"][3]["uptime"]) == (
        200,
        "2026-08-31T02:00:00Z",
        100,
    )
    assert (await call(FakeRepo(tracking_start=at(1), ranges=[]), "/domotics/uptime", FULL))[
        0
    ] == 200
    assert await call(FakeRepo(tracking_start=FAIL), "/domotics/uptime") == (
        500,
        {"error": "Failed to read uptime"},
    )


async def test_router_windows_parses_the_query() -> None:
    repo = FakeRepo(tracking_start=at(100), ranges=[])
    q = "?from=2026-08-30&to=2026-08-31T06:00:00Z&limit=5"
    status, body = await call(repo, "/domotics/uptime/windows" + q)
    assert status == 200
    assert repo.calls[1] == (
        "ranges",
        datetime(2026, 8, 30, tzinfo=UTC),
        datetime(2026, 8, 31, 6, tzinfo=UTC),
    )
    assert body["from"] == "2026-08-30T00:00:00Z"


@pytest.mark.parametrize(
    ("query", "message"),
    [
        ("?from=yesterday", "invalid from: must be RFC 3339 or YYYY-MM-DD"),
        ("?to=2026-8-1", "invalid to: must be RFC 3339 or YYYY-MM-DD"),
        ("?limit=0", "limit must be a positive integer"),
        ("?limit=-1", "limit must be a positive integer"),
        ("?limit=x", "limit must be a positive integer"),
        ("?from=2026-08-31T10:00:00Z&to=2026-08-31T09:00:00Z", "from must be before to"),
    ],
)
async def test_router_windows_rejects(query: str, message: str) -> None:
    repo = FakeRepo(tracking_start=at(100), ranges=[])
    assert await call(repo, "/domotics/uptime/windows" + query) == (400, {"error": message})


async def test_router_windows_failure() -> None:
    assert await call(FakeRepo(tracking_start=FAIL), "/domotics/uptime/windows") == (
        500,
        {"error": "Failed to read uptime windows"},
    )


async def test_router_needs_a_token_and_the_home_side() -> None:
    assert (await call(FakeRepo(), "/domotics/uptime", {}))[0] == 401
    assert await call(FakeRepo(), "/domotics/uptime", aws=True) == (
        503,
        {"error": ON_FAILOVER, "code": "unavailable_on_failover"},
    )
    assert (await call(FakeRepo(), "/domotics/uptime", {}, aws=True))[0] == 401
