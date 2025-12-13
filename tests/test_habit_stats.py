"""Integration tests for habit statistics."""

from datetime import date, timedelta
from decimal import Decimal

import pytest

from app.habits.enums import ComparisonType, ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate
from app.habits.service import HabitLogService, HabitService


@pytest.fixture
async def habit_with_logs(habit_service: HabitService, log_service: HabitLogService, _clean_habits):
    habit = await habit_service.create(
        HabitCreate(name="Daily Habit", value_type=ValueType.boolean)
    )
    habit_id = habit.id
    today = date.today()
    for i in range(5):
        await log_service.create(
            HabitLogCreate(
                habit_id=habit_id, log_date=today - timedelta(days=i), value=Decimal("1")
            )
        )
    return await habit_service.get(habit_id)


@pytest.fixture
async def numeric_habit_with_logs(
    habit_service: HabitService, log_service: HabitLogService, _clean_habits
):
    habit = await habit_service.create(
        HabitCreate(
            name="Numeric Habit",
            value_type=ValueType.numeric,
            comparison_type=ComparisonType.greater_equal_than,
            target_value=Decimal("10"),
        )
    )
    habit_id = habit.id
    today = date.today()
    for i, val in enumerate([15, 8, 12, 10, 5]):
        await log_service.create(
            HabitLogCreate(
                habit_id=habit_id, log_date=today - timedelta(days=i), value=Decimal(str(val))
            )
        )
    return await habit_service.get(habit_id)


class TestHabitStats:
    async def test_stats_with_no_logs(self, habit_service: HabitService, _clean_habits):
        habit = await habit_service.create(
            HabitCreate(name="Empty Habit", value_type=ValueType.boolean)
        )
        stats = await habit_service.get_stats(habit.id, days=7)
        assert stats.total_logs == 0
        assert stats.completion_rate == Decimal("0")
        assert stats.current_streak == 0

    async def test_stats_with_consecutive_logs(self, habit_service: HabitService, habit_with_logs):
        stats = await habit_service.get_stats(habit_with_logs.id, days=7)
        assert stats.total_logs == 5
        assert stats.current_streak == 5

    async def test_numeric_habit_average(
        self, habit_service: HabitService, numeric_habit_with_logs
    ):
        stats = await habit_service.get_stats(numeric_habit_with_logs.id, days=7)
        assert stats.average_value == Decimal("10")  # (15+8+12+10+5)/5

    async def test_boolean_habit_no_average(self, habit_service: HabitService, habit_with_logs):
        stats = await habit_service.get_stats(habit_with_logs.id, days=7)
        assert stats.average_value is None


class TestHabitStreak:
    async def test_streak_with_consecutive_days(self, habit_service: HabitService, habit_with_logs):
        streak = await habit_service.get_streak(habit_with_logs.id)
        assert streak.current == 5
        assert streak.longest == 5
        assert streak.last_completed == date.today()

    async def test_streak_with_gap(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        habit = await habit_service.create(
            HabitCreate(name="Gap Habit", value_type=ValueType.boolean)
        )
        habit_id = habit.id
        today = date.today()
        await log_service.create(
            HabitLogCreate(habit_id=habit_id, log_date=today, value=Decimal("1"))
        )
        await log_service.create(
            HabitLogCreate(
                habit_id=habit_id, log_date=today - timedelta(days=2), value=Decimal("1")
            )
        )
        streak = await habit_service.get_streak(habit_id)
        assert streak.current == 1

    async def test_streak_empty_habit(self, habit_service: HabitService, _clean_habits):
        habit = await habit_service.create(HabitCreate(name="Empty", value_type=ValueType.boolean))
        streak = await habit_service.get_streak(habit.id)
        assert streak.current == 0
        assert streak.longest == 0
        assert streak.last_completed is None
