from datetime import date
from typing import Protocol
from zoneinfo import ZoneInfo

from gv_api import history
from gv_api.core.dates import Clock, parse_day, today_in, utcnow
from gv_api.habits.schemas import (
    CreateHabitRequest,
    Habit,
    HabitWithLog,
    LogUpsertRequest,
    UpdateHabitRequest,
)

FREQUENCIES = ("daily", "weekly", "monthly")
_RANK = {f: i for i, f in enumerate(FREQUENCIES)}


class Repository(Protocol):
    async def get_habits_with_logs(
        self, view_date: date, streak_today: date
    ) -> list[HabitWithLog]: ...
    async def upsert_log(self, habit_id: int, day: date, value: float) -> None: ...
    async def create_habit(
        self,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit: ...
    async def update_habit(
        self,
        id: int,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit: ...
    async def delete_habit(self, id: int) -> None: ...
    async def get_habit_by_id(self, id: int) -> Habit: ...
    async def recalculate_streak(self, habit_id: int, today: date) -> None: ...
    async def get_habit_history(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[history.Point]: ...
    async def get_habit_history_avg(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[history.Point]: ...


def _check_frequency(frequency: str) -> None:
    if frequency not in FREQUENCIES:
        raise ValueError(f"invalid frequency: {frequency}")


class HabitsService:
    def __init__(self, repo: Repository, loc: ZoneInfo, now: Clock = utcnow) -> None:
        self._repo = repo
        self._loc = loc
        self._now = now

    def _today(self) -> date:
        return today_in(self._loc, self._now)

    async def get_daily_view(self, date_str: str) -> list[HabitWithLog]:
        today = self._today()
        target = parse_day(date_str) if date_str else today
        # A future view date must not count as met for the live (target_max = 0) streaks.
        return await self._repo.get_habits_with_logs(target, min(target, today))

    async def log_habit(self, req: LogUpsertRequest) -> None:
        day = parse_day(req.date)
        await self._repo.upsert_log(req.habit_id, day, req.value)
        habit = await self._repo.get_habit_by_id(req.habit_id)
        if habit.target_min is None and habit.target_max is None:
            return
        await self._repo.recalculate_streak(req.habit_id, self._today())

    async def create_habit(self, req: CreateHabitRequest) -> Habit:
        frequency = req.frequency or "daily"
        _check_frequency(frequency)
        recording_required = True if req.recording_required is None else req.recording_required
        return await self._repo.create_habit(
            req.name, req.description, frequency, req.target_min, req.target_max, recording_required
        )

    async def update_habit(self, id: int, req: UpdateHabitRequest) -> Habit:
        _check_frequency(req.frequency)
        return await self._repo.update_habit(
            id,
            req.name,
            req.description,
            req.frequency,
            req.target_min,
            req.target_max,
            req.recording_required,
        )

    async def delete_habit(self, id: int) -> None:
        await self._repo.delete_habit(id)

    async def get_history(
        self, habit_id: int, frequency: str, start_at: str, end_at: str
    ) -> history.Response:
        habit = await self._repo.get_habit_by_id(habit_id)
        frequency = frequency or habit.frequency
        trunc = history.valid_frequency(frequency)
        start, end = history.parse_date_range(self._loc, frequency, start_at, end_at, self._now)
        # Averaged when viewed at a coarser frequency than the habit's own, summed otherwise.
        if _RANK[frequency] > _RANK[habit.frequency]:
            query = self._repo.get_habit_history_avg
        else:
            query = self._repo.get_habit_history
        data = await query(habit_id, trunc, start, end, habit.recording_required)
        return history.Response(start_at=start, end_at=end, data=data)
