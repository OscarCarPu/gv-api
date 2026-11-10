"""The habits router over the real service and a fake repository."""

from datetime import UTC, date, datetime, timedelta
from types import SimpleNamespace
from typing import Any
from zoneinfo import ZoneInfo

import httpx
import pytest

from gv_api import habits
from gv_api.auth import AuthService
from gv_api.config import Config
from gv_api.core.app import create_app
from gv_api.habits import HabitsService
from gv_api.habits.schemas import HabitWithLog
from gv_api.history import Point

from .test_habits_service import FakeRepo, habit

CFG = Config(
    password="p",
    semiprivate_password="s",
    jwt_secret="secret",
    totp_secret="t",
    allowed_origins=["http://x"],
)
AUTH = AuthService(CFG)
FULL = {"Authorization": "Bearer " + AUTH.generate_token(timedelta(days=1), "full")}
NOW = datetime(2026, 3, 19, 10, 0, tzinfo=UTC)


async def ping() -> None:
    return None


def client(repo: FakeRepo) -> httpx.AsyncClient:
    app = create_app(CFG, ping=ping, routers=[habits.router])
    svc = HabitsService(repo, ZoneInfo("UTC"), lambda: NOW)
    app.state.container = SimpleNamespace(services={AuthService: AUTH, HabitsService: svc})
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://t")


async def call(
    repo: FakeRepo, method: str, path: str, body: str | None = None, auth: bool = True
) -> tuple[int, Any]:
    async with client(repo) as c:
        resp = await c.request(method, path, content=body, headers=FULL if auth else {})
    if resp.headers.get("content-type") == "application/json":
        return resp.status_code, resp.json()
    return resp.status_code, resp.text


class DailyRepo(FakeRepo):
    async def get_habits_with_logs(self, view_date: date, streak_today: date) -> list[HabitWithLog]:
        self._call("get_habits_with_logs", view_date, streak_today)
        return [
            HabitWithLog.model_construct(
                id=1,
                name="Exercise",
                description=None,
                frequency="daily",
                target_min=0.10000000149011612,
                target_max=None,
                recording_required=True,
                log_value=64.22000122070312,
                period_value=64.22000122070312,
                current_streak=2,
                longest_streak=3,
            )
        ]


# GET /habits


async def test_daily_view() -> None:
    repo = DailyRepo()
    status, body = await call(repo, "GET", "/habits?date=2025-01-31")
    assert repo.calls == [("get_habits_with_logs", date(2025, 1, 31), date(2025, 1, 31))]
    assert status == 200
    assert body == [
        {
            "id": 1,
            "name": "Exercise",
            "description": None,
            "frequency": "daily",
            "target_min": 0.1,
            "target_max": None,
            "recording_required": True,
            "log_value": 64.22,
            "period_value": 64.22,
            "current_streak": 2,
            "longest_streak": 3,
        }
    ]


async def test_empty_daily_view_is_null() -> None:
    async with client(FakeRepo()) as c:
        resp = await c.get("/habits", headers=FULL)
    assert (resp.status_code, resp.text) == (200, "null")


@pytest.mark.parametrize("path", ["/habits?date=bad", "/habits?date=2026-02-30"])
async def test_daily_view_bad_date_is_a_500(path: str) -> None:
    assert await call(FakeRepo(), "GET", path) == (500, {"error": "Failed to get daily habits"})


async def test_daily_view_repo_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    assert await call(repo, "GET", "/habits") == (500, {"error": "Failed to get daily habits"})


async def test_needs_a_full_token() -> None:
    assert await call(FakeRepo(), "GET", "/habits", auth=False) == (401, {"error": "Unauthorized"})


# POST /habits/log

LOG = '{"habit_id": 1, "date": "2025-01-31", "value": 42.5, "foo": 1}'


async def test_log() -> None:
    repo = FakeRepo(habits={1: habit()})
    assert await call(repo, "POST", "/habits/log", LOG) == (200, {"status": "ok"})
    assert repo.calls[0] == ("upsert_log", 1, date(2025, 1, 31), 42.5)


@pytest.mark.parametrize(
    "body",
    [
        "not json",
        '{"habit_id": "abc"}',
        '{"habit_id": 4294967296, "date": "2025-01-31"}',
        '{"habit_id": 1, "date": 20250131}',
    ],
)
async def test_log_invalid_body(body: str) -> None:
    assert await call(FakeRepo(), "POST", "/habits/log", body) == (400, {"error": "Invalid Body"})


@pytest.mark.parametrize(
    ("repo", "body"),
    [
        (FakeRepo(fail=RuntimeError("db")), LOG),
        (FakeRepo(), '{"habit_id": 1, "date": "bad"}'),
        (FakeRepo(), LOG),  # unknown habit
    ],
)
async def test_log_errors_are_500s(repo: FakeRepo, body: str) -> None:
    assert await call(repo, "POST", "/habits/log", body) == (500, {"error": "Failed to log habit"})


# POST /habits


async def test_create() -> None:
    repo = FakeRepo()
    status, body = await call(repo, "POST", "/habits", '{"name": "Exercise", "frequency": null}')
    assert status == 201
    assert body == {
        "id": 1,
        "name": "Exercise",
        "description": None,
        "frequency": "daily",
        "target_min": None,
        "target_max": None,
        "recording_required": True,
        "current_streak": 0,
        "longest_streak": 0,
    }


@pytest.mark.parametrize(
    ("body", "message"),
    [
        ("not json", "Invalid Body"),
        ("{}", "name is required"),
        ('{"name": "' + "x" * 41 + '"}', "name must be at most 40 characters"),
        ('{"name": "' + "ñ" * 41 + '"}', "name must be at most 40 characters"),
        ('{"name": "E", "frequency": ""}', "frequency must be daily, weekly, or monthly"),
        ('{"name": "E", "frequency": "yearly"}', "frequency must be daily, weekly, or monthly"),
        ('{"name": "E", "target_min": -1}', "target_min must be >= 0"),
        ('{"name": "E", "target_max": -0.5}', "target_max must be >= 0"),
        ('{"name": "E", "target_min": 10, "target_max": 5}', "target_min must be <= target_max"),
    ],
)
async def test_create_validation(body: str, message: str) -> None:
    repo = FakeRepo()
    assert await call(repo, "POST", "/habits", body) == (400, {"error": message})
    assert repo.calls == []


async def test_create_repo_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    status, body = await call(repo, "POST", "/habits", '{"name": "Exercise"}')
    assert (status, body) == (500, {"error": "Failed to create habit"})


# PUT /habits/{id}

UPDATE = (
    '{"name": "Exercise", "description": null, "frequency": "daily", "target_min": 1,'
    ' "target_max": null, "recording_required": true}'
)


async def test_update() -> None:
    repo = FakeRepo(habits={7: habit(id=7)})
    status, body = await call(repo, "PUT", "/habits/7", UPDATE)
    assert repo.calls == [("update_habit", 7, "Exercise", None, "daily", 1, None, True)]
    assert (status, body["id"], body["name"]) == (200, 7, "Exercise")


@pytest.mark.parametrize(
    ("path", "body", "message"),
    [
        ("/habits/abc", UPDATE, "invalid habit id"),
        ("/habits/0", "not json", "invalid habit id"),
        ("/habits/1", "not json", "Invalid Body"),
        ("/habits/1", '{"name": "", "frequency": "daily"}', "name is required"),
        ("/habits/1", '{"name": "E"}', "frequency must be daily, weekly, or monthly"),
        (
            "/habits/1",
            '{"name": "E", "frequency": "yearly"}',
            "frequency must be daily, weekly, or monthly",
        ),
        (
            "/habits/1",
            '{"name": "E", "frequency": "daily", "target_min": 10, "target_max": 5}',
            "target_min must be <= target_max",
        ),
    ],
)
async def test_update_validation(path: str, body: str, message: str) -> None:
    assert await call(FakeRepo(), "PUT", path, body) == (400, {"error": message})


async def test_update_not_found() -> None:
    assert await call(FakeRepo(), "PUT", "/habits/7", UPDATE) == (404, {"error": "habit not found"})


async def test_update_repo_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    assert await call(repo, "PUT", "/habits/7", UPDATE) == (
        500,
        {"error": "Failed to update habit"},
    )


# DELETE /habits/{id}


async def test_delete() -> None:
    repo = FakeRepo()
    async with client(repo) as c:
        resp = await c.delete("/habits/5", headers=FULL)
    assert (resp.status_code, resp.content) == (204, b"")
    assert repo.calls == [("delete_habit", 5)]


async def test_delete_invalid_id() -> None:
    assert await call(FakeRepo(), "DELETE", "/habits/abc") == (400, {"error": "invalid habit id"})


async def test_delete_repo_error() -> None:
    repo = FakeRepo(fail=RuntimeError("db"))
    assert await call(repo, "DELETE", "/habits/1") == (500, {"error": "Failed to delete habit"})


# GET /habits/{id}/history


async def test_history() -> None:
    repo = FakeRepo(habits={1: habit()}, points=[Point(date=date(2026, 3, 2), value=0.3)])
    status, body = await call(
        repo, "GET", "/habits/1/history?frequency=daily&start_at=2026-03-01&end_at=2026-03-19"
    )
    assert status == 200
    assert body == {
        "start_at": "2026-03-01",
        "end_at": "2026-03-19",
        "data": [{"date": "2026-03-02", "value": 0.3}],
    }


async def test_history_without_data_is_an_empty_list() -> None:
    status, body = await call(FakeRepo(habits={1: habit()}), "GET", "/habits/1/history")
    assert (status, body["data"]) == (200, [])


@pytest.mark.parametrize(
    ("path", "message"),
    [
        ("/habits/abc/history?frequency=daily", "invalid habit id"),
        ("/habits/1/history?frequency=yearly", "frequency must be daily, weekly, or monthly"),
    ],
)
async def test_history_validation(path: str, message: str) -> None:
    assert await call(FakeRepo(), "GET", path) == (400, {"error": message})


@pytest.mark.parametrize(
    "path", ["/habits/9/history", "/habits/1/history?start_at=bad", "/habits/1/history?end_at=x"]
)
async def test_history_errors_are_500s(path: str) -> None:
    repo = FakeRepo(habits={1: habit()})
    assert await call(repo, "GET", path) == (500, {"error": "Failed to get habit history"})


# Routing


async def test_put_on_log_is_an_invalid_id() -> None:
    assert await call(FakeRepo(), "PUT", "/habits/log", UPDATE) == (
        400,
        {"error": "invalid habit id"},
    )


async def test_wrong_method_lists_allow() -> None:
    async with client(FakeRepo()) as c:
        resp = await c.patch("/habits", headers=FULL)
    assert resp.status_code == 405
    assert resp.headers["allow"] == "GET, POST"
