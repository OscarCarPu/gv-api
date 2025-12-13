from collections.abc import Sequence
from datetime import date
from decimal import Decimal
from typing import Any

from sqlalchemy import ColumnElement, UnaryExpression, case, desc, func, select, true

from app.common import BaseRepository

from .models import ComparisonType, Habit, HabitLog, TargetFrequency, ValueType


class HabitRepository(BaseRepository[Habit]):
    """Repository for Habit data access."""

    model = Habit

    async def get_by_name(self, name: str) -> Habit | None:
        """Find a habit by its name."""
        return await self.get_by(name=name)

    async def get_all(
        self,
        *filters: ColumnElement[bool],
        frequency: TargetFrequency | None = None,
        order_by: UnaryExpression[Any] | ColumnElement[Any] | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> Sequence[Habit]:
        """Get all habits, optionally filtered by frequency."""
        all_filters = []
        if frequency:
            all_filters.append(Habit.frequency == frequency)
        return await super().get_all(
            *all_filters, order_by=desc(Habit.created_at), limit=limit, offset=offset
        )

    async def count_all(self, frequency: TargetFrequency | None = None) -> int:
        """Count all habits, optionally filtered by frequency."""
        filters = []
        if frequency:
            filters.append(Habit.frequency == frequency)
        return await self.count(*filters)


class HabitLogRepository(BaseRepository[HabitLog]):
    """Repository for HabitLog data access."""

    model = HabitLog

    async def get_by_habit_id(
        self,
        habit_id: int,
        start_date: date | None = None,
        end_date: date | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> Sequence[HabitLog]:
        """Get logs for a specific habit, optionally filtered by date range."""
        filters = [HabitLog.habit_id == habit_id]
        if start_date:
            filters.append(HabitLog.log_date >= start_date)
        if end_date:
            filters.append(HabitLog.log_date <= end_date)
        return await self.get_all(
            *filters, order_by=desc(HabitLog.log_date), limit=limit, offset=offset
        )

    async def count_by_habit_id(
        self,
        habit_id: int,
        start_date: date | None = None,
        end_date: date | None = None,
    ) -> int:
        """Count logs for a specific habit, optionally filtered by date range."""
        filters = [HabitLog.habit_id == habit_id]
        if start_date:
            filters.append(HabitLog.log_date >= start_date)
        if end_date:
            filters.append(HabitLog.log_date <= end_date)
        return await self.count(*filters)

    async def get_by_habit_and_date(
        self,
        habit_id: int,
        log_date: date,
    ) -> HabitLog | None:
        """Get a specific log by habit and date."""
        return await self.get_by(habit_id=habit_id, log_date=log_date)

    async def get_logs_for_date(
        self,
        target_date: date,
    ) -> Sequence[HabitLog]:
        """Get all logs for a specific date."""
        return await self.get_all(HabitLog.log_date == target_date)

    async def get_logs_in_range(
        self,
        habit_id: int,
        start_date: date,
        end_date: date | None = None,
    ) -> Sequence[HabitLog]:
        """Get logs for a habit within a date range."""
        filters = [
            HabitLog.habit_id == habit_id,
            HabitLog.log_date >= start_date,
        ]
        if end_date:
            filters.append(HabitLog.log_date <= end_date)
        return await super().get_all(*filters)

    async def get_stats_aggregated(
        self,
        habit: Habit,
        start_date: date,
        end_date: date | None = None,
    ) -> tuple[int, Decimal | None, int]:
        """
        Database-level statistics calculation.
        Returns (total_logs, average_value, targets_met).
        """
        target_met_case = self._build_target_met_case(habit)

        stmt = (
            select(
                func.count(HabitLog.id).label("total_logs"),
                func.avg(HabitLog.value).label("average_value"),
                func.sum(case((target_met_case, 1), else_=0)).label("targets_met"),
            )
            .where(HabitLog.habit_id == habit.id)
            .where(HabitLog.log_date >= start_date)
        )
        if end_date:
            stmt = stmt.where(HabitLog.log_date <= end_date)

        result = await self.session.execute(stmt)
        row = result.one()

        total_logs = row.total_logs or 0
        average_value = Decimal(str(row.average_value)) if row.average_value is not None else None
        targets_met = row.targets_met or 0

        return total_logs, average_value, targets_met

    def _build_target_met_case(self, habit: Habit):
        """Build SQL CASE expression for target_met check."""
        if habit.value_type == ValueType.boolean:
            return HabitLog.value == Decimal("1")

        if habit.comparison_type is None or habit.target_value is None:
            return true()

        match habit.comparison_type:
            case ComparisonType.equals:
                return HabitLog.value == habit.target_value
            case ComparisonType.greater_than:
                return HabitLog.value > habit.target_value
            case ComparisonType.less_than:
                return HabitLog.value < habit.target_value
            case ComparisonType.greater_equal_than:
                return HabitLog.value >= habit.target_value
            case ComparisonType.less_equal_than:
                return HabitLog.value <= habit.target_value
            case ComparisonType.in_range:
                if habit.target_min is None or habit.target_max is None:
                    return true()
                return (HabitLog.value >= habit.target_min) & (HabitLog.value <= habit.target_max)

    async def get_dates_target_met(
        self,
        habit: Habit,
        start_date: date | None = None,
    ) -> Sequence[date]:
        """
        Get dates where target was met, using database-level filtering.
        Returns sorted list of dates (ascending).
        """
        target_met_case = self._build_target_met_case(habit)

        stmt = (
            select(HabitLog.log_date)
            .where(HabitLog.habit_id == habit.id)
            .where(target_met_case)
            .order_by(HabitLog.log_date)
        )
        if start_date:
            stmt = stmt.where(HabitLog.log_date >= start_date)

        result = await self.session.execute(stmt)
        return [row[0] for row in result.all()]
