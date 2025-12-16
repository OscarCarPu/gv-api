"""Tests for the /habits/{id}/history endpoint."""

from datetime import date, timedelta
from decimal import Decimal

from app.habits.enums import TargetFrequency, ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate
from app.habits.service import HabitLogService, HabitService


class TestHistoryEndpoint:
    """Tests for get_habit_history functionality."""

    async def test_empty_history(self, habit_service: HabitService, _clean_habits):
        """Should return periods with zero values when no logs exist."""
        habit = await habit_service.create(HabitCreate(name="Empty", value_type=ValueType.boolean))
        today = date.today()
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today - timedelta(days=2),
            end_date=today,
        )
        assert result.habit_id == habit.id
        assert result.time_period == "daily"
        assert len(result.periods) == 3
        assert all(p.total_value == Decimal("0") for p in result.periods)

    async def test_daily_aggregation(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should aggregate logs by day."""
        habit = await habit_service.create(HabitCreate(name="Daily", value_type=ValueType.numeric))
        today = date.today()
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("10"))
        )
        await log_service.create(
            HabitLogCreate(
                habit_id=habit.id, log_date=today - timedelta(days=1), value=Decimal("5")
            )
        )
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today - timedelta(days=1),
            end_date=today,
        )
        assert len(result.periods) == 2
        # First period is older
        assert result.periods[0].total_value == Decimal("5")
        assert result.periods[1].total_value == Decimal("10")

    async def test_weekly_aggregation(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should aggregate logs by week."""
        habit = await habit_service.create(
            HabitCreate(
                name="Weekly",
                value_type=ValueType.numeric,
                frequency=TargetFrequency.weekly,
            )
        )
        today = date.today()
        # Log multiple days in the same week
        for i in range(3):
            log_date = today - timedelta(days=i)
            await log_service.create(
                HabitLogCreate(habit_id=habit.id, log_date=log_date, value=Decimal("1"))
            )
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today - timedelta(days=6),
            end_date=today,
            time_period="week",
        )
        # Should be aggregated into weeks
        assert result.time_period == "week"
        # The sum should include all logs in the current week
        total = sum(p.total_value for p in result.periods)
        assert total == Decimal("3")

    async def test_monthly_aggregation(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should aggregate logs by month."""
        habit = await habit_service.create(
            HabitCreate(
                name="Monthly",
                value_type=ValueType.numeric,
                frequency=TargetFrequency.monthly,
            )
        )
        today = date.today()
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("100"))
        )
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today.replace(day=1),
            end_date=today,
            time_period="month",
        )
        assert result.time_period == "month"
        assert len(result.periods) == 1
        assert result.periods[0].total_value == Decimal("100")

    async def test_includes_empty_periods(
        self, habit_service: HabitService, log_service: HabitLogService, _clean_habits
    ):
        """Should include periods with no logs as zero value."""
        habit = await habit_service.create(HabitCreate(name="Sparse", value_type=ValueType.boolean))
        today = date.today()
        # Only log today, not yesterday
        await log_service.create(
            HabitLogCreate(habit_id=habit.id, log_date=today, value=Decimal("1"))
        )
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today - timedelta(days=1),
            end_date=today,
        )
        assert len(result.periods) == 2
        # Yesterday should be zero
        assert result.periods[0].total_value == Decimal("0")
        assert result.periods[1].total_value == Decimal("1")

    async def test_default_time_period_uses_habit_frequency(
        self, habit_service: HabitService, _clean_habits
    ):
        """Should use habit's frequency as default time_period."""
        habit = await habit_service.create(
            HabitCreate(
                name="Weekly Habit",
                value_type=ValueType.boolean,
                frequency=TargetFrequency.weekly,
            )
        )
        result = await habit_service.get_habit_history(habit.id)
        assert result.time_period == "weekly"

    async def test_period_boundaries(self, habit_service: HabitService, _clean_habits):
        """Should return correct period start and end dates."""
        habit = await habit_service.create(
            HabitCreate(name="Boundaries", value_type=ValueType.boolean)
        )
        today = date.today()
        result = await habit_service.get_habit_history(
            habit.id,
            start_date=today,
            end_date=today,
        )
        assert len(result.periods) == 1
        assert result.periods[0].period_start == today
        assert result.periods[0].period_end == today
