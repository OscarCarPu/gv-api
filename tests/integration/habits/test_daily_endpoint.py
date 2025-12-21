"""Tests for the /habits/daily endpoint."""

from datetime import date, timedelta
from decimal import Decimal

from app.habits.enums import ComparisonType, ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate
from app.habits.service import HabitLogService, HabitService


class TestDailyEndpoint:
    """Tests for get_daily_habits functionality."""

    async def test_empty_response_no_habits(self, habit_service: HabitService, _clean_habits):
        """Should return empty list when no habits exist."""
        result = await habit_service.get_daily_habits()
        assert result == []

    async def test_returns_active_habit(self, habit_service: HabitService, _clean_habits):
        """Should return habits that are active today."""
        await habit_service.create(HabitCreate(name="Active Habit", value_type=ValueType.boolean))
        result = await habit_service.get_daily_habits()
        assert len(result) == 1
        assert result[0].name == "Active Habit"

    async def test_excludes_future_habit(self, habit_service: HabitService, _clean_habits):
        """Should exclude habits that haven't started yet."""
        tomorrow = date.today() + timedelta(days=1)
        await habit_service.create(
            HabitCreate(
                name="Future Habit",
                value_type=ValueType.boolean,
                start_date=tomorrow,
            )
        )
        result = await habit_service.get_daily_habits()
        assert len(result) == 0

    async def test_excludes_ended_habit(self, habit_service: HabitService, _clean_habits):
        """Should exclude habits that have ended."""
        yesterday = date.today() - timedelta(days=1)
        await habit_service.create(
            HabitCreate(
                name="Ended Habit",
                value_type=ValueType.boolean,
                end_date=yesterday,
            )
        )
        result = await habit_service.get_daily_habits()
        assert len(result) == 0

    async def test_current_period_value(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should return sum of logs in current period."""
        habit = await habit_service.create(HabitCreate(name="Test", value_type=ValueType.numeric))
        today = date.today()
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("5"))
        )
        result = await habit_service.get_daily_habits()
        assert len(result) == 1
        assert result[0].current_period_value == Decimal("5")

    async def test_current_streak_with_consecutive_days(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should calculate current streak correctly."""
        habit = await habit_service.create(
            HabitCreate(name="Streak Test", value_type=ValueType.boolean)
        )
        today = date.today()
        # Log for today and yesterday
        for i in range(3):
            await log_service.create(
                HabitLogCreate(
                    habit_id=habit.id,
                    log_date=today - timedelta(days=i),
                    value=Decimal("1"),
                )
            )
        result = await habit_service.get_daily_habits()
        assert result[0].current_streak == 3

    async def test_streak_breaks_on_missing_day_required(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """For required habits, missing day breaks streak."""
        habit = await habit_service.create(
            HabitCreate(name="Required", value_type=ValueType.boolean, is_required=True)
        )
        today = date.today()
        # Log for today, skip yesterday, log day before
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("1"))
        )
        await log_service.create(
            HabitLogCreate(
                habit_id=habit.id, log_date=today - timedelta(days=2), value=Decimal("1")
            )
        )
        result = await habit_service.get_daily_habits()
        assert result[0].current_streak == 1

    async def test_streak_continues_on_missing_day_non_required(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """For non-required habits, missing day doesn't break streak."""
        habit = await habit_service.create(
            HabitCreate(name="Optional", value_type=ValueType.boolean, is_required=False)
        )
        today = date.today()
        # Log for today, skip yesterday, log day before
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("1"))
        )
        await log_service.create(
            HabitLogCreate(
                habit_id=habit.id, log_date=today - timedelta(days=2), value=Decimal("1")
            )
        )
        result = await habit_service.get_daily_habits()
        # Non-required habits ignore missing days
        assert result[0].current_streak == 2

    async def test_numeric_habit_stats(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should calculate stats for numeric habits."""
        await habit_service.create(
            HabitCreate(
                name="Numeric",
                value_type=ValueType.numeric,
                comparison_type=ComparisonType.greater_equal_than,
                target_value=Decimal("10"),
            )
        )
        result = await habit_service.get_daily_habits()
        assert result[0].target_value == Decimal("10")
        assert result[0].comparison_type == ComparisonType.greater_equal_than
