"""From internal/habits/service_test.go, with a hand-written repository fake."""

from dataclasses import dataclass, field
from datetime import UTC, date, datetime, timedelta
from zoneinfo import ZoneInfo

import pytest

from gv_api.habits import HabitsService
from gv_api.habits.errors import NotFoundError
from gv_api.habits.schemas import (
    CreateHabitRequest,
    Habit,
    HabitWithLog,
    LogUpsertRequest,
    UpdateHabitRequest,
)
from gv_api.history import Point

NOW = datetime(2026, 3, 19, 10, 0, tzinfo=UTC)  # a Thursday
TODAY = NOW.date()


def d(iso: str) -> date:
    return date.fromisoformat(iso)


def habit(**kw: object) -> Habit:
    base: dict[str, object] = {
        "id": 1,
        "name": "h",
        "frequency": "daily",
        "recording_required": True,
        "current_streak": 0,
        "longest_streak": 0,
    }
    return Habit.model_construct(**(base | kw))


@dataclass
class FakeRepo:
    habits: dict[int, Habit] = field(default_factory=dict)
    calls: list[tuple[object, ...]] = field(default_factory=list)
    points: list[Point] = field(default_factory=list)
    fail: Exception | None = None

    def _call(self, *args: object) -> None:
        self.calls.append(args)
        if self.fail is not None:
            raise self.fail

    def names(self) -> list[object]:
        return [c[0] for c in self.calls]

    async def get_habits_with_logs(self, view_date: date, streak_today: date) -> list[HabitWithLog]:
        self._call("get_habits_with_logs", view_date, streak_today)
        return []

    async def upsert_log(self, habit_id: int, day: date, value: float) -> None:
        self._call("upsert_log", habit_id, day, value)

    async def create_habit(
        self,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit:
        args = (name, description, frequency, target_min, target_max, recording_required)
        self._call("create_habit", *args)
        return habit(name=name, frequency=frequency, recording_required=recording_required)

    async def update_habit(
        self,
        id: int,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit:
        args = (id, name, description, frequency, target_min, target_max, recording_required)
        self._call("update_habit", *args)
        if id not in self.habits:
            raise NotFoundError
        return habit(id=id, name=name, frequency=frequency)

    async def delete_habit(self, id: int) -> None:
        self._call("delete_habit", id)

    async def get_habit_by_id(self, id: int) -> Habit:
        self._call("get_habit_by_id", id)
        if id not in self.habits:
            raise NotFoundError
        return self.habits[id]

    async def recalculate_streak(self, habit_id: int, today: date) -> None:
        self._call("recalculate_streak", habit_id, today)

    async def get_habit_history(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[Point]:
        self._call("sum", habit_id, frequency, start_at, end_at, fill_zeros)
        return self.points

    async def get_habit_history_avg(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[Point]:
        self._call("avg", habit_id, frequency, start_at, end_at, fill_zeros)
        return self.points


def service(repo: FakeRepo, now: datetime = NOW, tz: str = "UTC") -> HabitsService:
    return HabitsService(repo, ZoneInfo(tz), lambda: now)


# log_habit


async def test_log_with_targets_recalculates_the_streak() -> None:
    repo = FakeRepo(habits={1: habit(target_min=1)})
    await service(repo).log_habit(LogUpsertRequest(habit_id=1, date="2026-03-17", value=1))
    assert repo.calls == [
        ("upsert_log", 1, d("2026-03-17"), 1),
        ("get_habit_by_id", 1),
        ("recalculate_streak", 1, TODAY),
    ]


async def test_log_streak_uses_today_in_the_zone() -> None:
    repo = FakeRepo(habits={1: habit(target_max=0)})
    late = datetime(2026, 3, 19, 23, 30, tzinfo=UTC)
    await service(repo, late, "Europe/Madrid").log_habit(
        LogUpsertRequest(habit_id=1, date="2026-03-17", value=1)
    )
    assert repo.calls[-1] == ("recalculate_streak", 1, d("2026-03-20"))


async def test_log_without_targets_skips_the_streak() -> None:
    repo = FakeRepo(habits={1: habit()})
    await service(repo).log_habit(LogUpsertRequest(habit_id=1, date="2026-03-17", value=1))
    assert repo.names() == ["upsert_log", "get_habit_by_id"]


async def test_log_upsert_error_propagates() -> None:
    repo = FakeRepo(fail=RuntimeError("db error"))
    with pytest.raises(RuntimeError, match="db error"):
        await service(repo).log_habit(LogUpsertRequest(habit_id=1, date="2026-03-17", value=1))


async def test_log_unknown_habit_propagates() -> None:
    with pytest.raises(NotFoundError):
        await service(FakeRepo()).log_habit(
            LogUpsertRequest(habit_id=9, date="2026-03-17", value=1)
        )


async def test_log_invalid_date() -> None:
    repo = FakeRepo()
    with pytest.raises(ValueError):
        await service(repo).log_habit(LogUpsertRequest(habit_id=1, date="not-a-date", value=1))
    assert repo.calls == []


# create_habit and update_habit


async def test_create_defaults() -> None:
    repo = FakeRepo()
    resp = await service(repo).create_habit(CreateHabitRequest(name="Exercise"))
    assert repo.calls == [("create_habit", "Exercise", None, "daily", None, None, True)]
    assert resp.frequency == "daily"


async def test_create_uses_the_given_values() -> None:
    repo = FakeRepo()
    req = CreateHabitRequest(
        name="Meditate",
        description="d",
        frequency="weekly",
        target_min=5,
        recording_required=False,
    )
    await service(repo).create_habit(req)
    assert repo.calls == [("create_habit", "Meditate", "d", "weekly", 5, None, False)]


async def test_create_empty_frequency_is_daily() -> None:
    repo = FakeRepo()
    await service(repo).create_habit(CreateHabitRequest(name="x", frequency=""))
    assert repo.calls[0][3] == "daily"


async def test_create_invalid_frequency() -> None:
    with pytest.raises(ValueError, match="invalid frequency"):
        await service(FakeRepo()).create_habit(CreateHabitRequest(name="Bad", frequency="yearly"))


async def test_update_passes_through() -> None:
    repo = FakeRepo(habits={7: habit(id=7)})
    req = UpdateHabitRequest(name="Run", frequency="weekly", target_min=2)
    resp = await service(repo).update_habit(7, req)
    assert repo.calls == [("update_habit", 7, "Run", None, "weekly", 2, None, False)]
    assert resp.id == 7


async def test_update_invalid_frequency() -> None:
    with pytest.raises(ValueError, match="invalid frequency"):
        await service(FakeRepo()).update_habit(
            1, UpdateHabitRequest(name="Bad", frequency="yearly")
        )


async def test_update_not_found() -> None:
    with pytest.raises(NotFoundError):
        await service(FakeRepo()).update_habit(99, UpdateHabitRequest(name="X", frequency="daily"))


async def test_delete() -> None:
    repo = FakeRepo()
    await service(repo).delete_habit(5)
    assert repo.calls == [("delete_habit", 5)]


# get_daily_view


async def test_daily_view_defaults_to_today() -> None:
    repo = FakeRepo()
    assert await service(repo).get_daily_view("") == []
    assert repo.calls == [("get_habits_with_logs", TODAY, TODAY)]


async def test_daily_view_today_is_in_the_zone() -> None:
    repo = FakeRepo()
    late = datetime(2026, 3, 19, 23, 30, tzinfo=UTC)
    await service(repo, late, "Europe/Madrid").get_daily_view("")
    assert repo.calls == [("get_habits_with_logs", d("2026-03-20"), d("2026-03-20"))]


async def test_daily_view_parses_the_date() -> None:
    repo = FakeRepo()
    await service(repo).get_daily_view("2026-01-15")
    assert repo.calls == [("get_habits_with_logs", d("2026-01-15"), d("2026-01-15"))]


async def test_daily_view_invalid_date() -> None:
    with pytest.raises(ValueError):
        await service(FakeRepo()).get_daily_view("bad-date")


async def test_daily_view_future_date_caps_the_streak_day() -> None:
    repo = FakeRepo()
    await service(repo).get_daily_view("2099-01-01")
    assert repo.calls == [("get_habits_with_logs", d("2099-01-01"), TODAY)]


# get_history


async def test_history_frequency_defaults_to_the_habit() -> None:
    repo = FakeRepo(habits={1: habit(frequency="weekly", recording_required=False)})
    repo.points = [Point(date=d("2026-03-10"), value=5)]
    result = await service(repo).get_history(1, "", "2026-03-01", "2026-03-19")
    # 2026-03-01 is a Sunday, so its week starts 02-23; 03-19 is a Thursday, ceil to 03-23.
    assert repo.calls[-1] == ("sum", 1, "week", d("2026-02-23"), d("2026-03-23"), False)
    assert result.model_dump(mode="json") == {
        "start_at": "2026-02-23",
        "end_at": "2026-03-23",
        "data": [{"date": "2026-03-10", "value": 5}],
    }


async def test_history_explicit_finer_frequency_sums() -> None:
    repo = FakeRepo(habits={1: habit(frequency="weekly")})
    result = await service(repo).get_history(1, "daily", "2026-03-01", "2026-03-19")
    assert repo.calls[-1] == ("sum", 1, "day", d("2026-03-01"), d("2026-03-19"), True)
    assert result.data == []


async def test_history_coarser_frequency_averages() -> None:
    repo = FakeRepo(habits={1: habit(frequency="daily")})
    repo.points = [Point(date=d("2026-03-02"), value=7.5)]
    result = await service(repo).get_history(1, "weekly", "2026-03-02", "2026-03-08")
    assert repo.names()[-1] == "avg"
    assert result.data[0].value == 7.5


async def test_history_same_frequency_sums() -> None:
    repo = FakeRepo(habits={1: habit(frequency="monthly")})
    await service(repo).get_history(1, "monthly", "2026-03-02", "2026-03-08")
    assert repo.calls[-1] == ("sum", 1, "month", d("2026-03-01"), d("2026-04-01"), True)


@pytest.mark.parametrize(
    ("frequency", "start", "end"),
    [
        ("daily", TODAY - timedelta(days=28), TODAY),  # Feb 19 .. Mar 19
        ("weekly", d("2025-12-22"), d("2026-03-23")),
        ("monthly", d("2025-03-01"), d("2026-04-01")),
    ],
)
async def test_history_default_dates(frequency: str, start: date, end: date) -> None:
    repo = FakeRepo(habits={1: habit(frequency=frequency, recording_required=False)})
    result = await service(repo).get_history(1, frequency, "", "")
    assert repo.calls[-1][3:5] == (start, end)
    assert (result.start_at, result.end_at) == (start, end)


async def test_history_unknown_habit() -> None:
    with pytest.raises(NotFoundError):
        await service(FakeRepo()).get_history(1, "", "", "")


@pytest.mark.parametrize(
    ("frequency", "start_at", "end_at", "message"),
    [
        ("yearly", "", "", "invalid frequency"),
        ("daily", "bad", "", "invalid start_at date"),
        ("daily", "", "bad", "invalid end_at date"),
    ],
)
async def test_history_errors(frequency: str, start_at: str, end_at: str, message: str) -> None:
    repo = FakeRepo(habits={1: habit()})
    with pytest.raises(ValueError, match=message):
        await service(repo).get_history(1, frequency, start_at, end_at)
    assert repo.names() == ["get_habit_by_id"]
