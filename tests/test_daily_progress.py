"""Integration tests for daily progress."""

from datetime import date, timedelta
from decimal import Decimal

from app.habits.enums import TargetFrequency, ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate
from app.habits.service import HabitLogService, HabitService


class TestDailyProgress:
    async def test_empty_progress_no_habits(self, habit_service: HabitService, _clean_habits):
        progress = await habit_service.get_daily_progress(date.today())
        assert progress == []

    async def test_progress_with_unlogged_habit(self, habit_service: HabitService, _clean_habits):
        await habit_service.create(HabitCreate(name="Unlogged", value_type=ValueType.boolean))
        progress = await habit_service.get_daily_progress(date.today())
        assert len(progress) == 1
        assert progress[0].is_logged is False
        assert progress[0].is_target_met is False

    async def test_progress_with_logged_habit(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        habit = await habit_service.create(HabitCreate(name="Logged", value_type=ValueType.boolean))
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=date.today(), value=Decimal("1"))
        )
        progress = await habit_service.get_daily_progress(date.today())
        assert len(progress) == 1
        assert progress[0].is_logged is True
        assert progress[0].is_target_met is True

    async def test_progress_mixed_habits(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        logged = await habit_service.create(
            HabitCreate(name="Logged", value_type=ValueType.boolean)
        )
        logged_id = logged.id
        await habit_service.create(HabitCreate(name="Unlogged", value_type=ValueType.boolean))
        await log_service.create(
            HabitLogCreate(habit_id=logged_id, log_date=date.today(), value=Decimal("1"))
        )
        progress = await habit_service.get_daily_progress(date.today())
        assert len(progress) == 2
        logged_count = sum(1 for p in progress if p.is_logged)
        assert logged_count == 1

    async def test_progress_excludes_non_due_habits(
        self, habit_service: HabitService, _clean_habits
    ):
        tomorrow = date.today() + timedelta(days=1)
        await habit_service.create(
            HabitCreate(name="Future", value_type=ValueType.boolean, start_date=tomorrow)
        )
        progress = await habit_service.get_daily_progress(date.today())
        assert len(progress) == 0

    async def test_progress_weekly_habit_not_due(self, habit_service: HabitService, _clean_habits):
        today = date.today()
        await habit_service.create(
            HabitCreate(
                name="Weekly",
                value_type=ValueType.boolean,
                frequency=TargetFrequency.weekly,
                start_date=today,
            )
        )
        tomorrow = today + timedelta(days=1)
        progress = await habit_service.get_daily_progress(tomorrow)
        assert len(progress) == 0
