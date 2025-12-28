from collections.abc import Sequence
from datetime import date
from decimal import Decimal
from typing import Any

from sqlalchemy import ColumnElement, UnaryExpression, and_, case, desc, func, or_, select, true

from app.common import BaseRepository

from .models import ComparisonType, Habit, HabitLog, TargetFrequency, ValueType


class HabitRepository(BaseRepository[Habit]):
    """Repository for Habit data access."""

    model = Habit

    async def get_by_name(self, name: str) -> Habit | None:
        """Find a habit by its name."""
        return await self.get_by(name=name)

    async def get_by_name_icase(self, name: str) -> Habit | None:
        """Find a habit by its name (case-insensitive exact match)."""
        stmt = select(Habit).where(func.lower(Habit.name) == name.lower())
        result = await self.session.execute(stmt)
        return result.scalars().first()

    async def get_all(
        self,
        *filters: ColumnElement[bool],
        frequency: TargetFrequency | None = None,
        order_by: UnaryExpression[Any] | ColumnElement[Any] | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> Sequence[Habit]:
        """Get all habits, optionally filtered by frequency."""
        all_filters = list(filters)
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

    async def get_active_habits(self, target_date: date) -> Sequence[Habit]:
        """Get habits that are active on a given date (between start_date and end_date)."""
        filters = [
            or_(Habit.start_date.is_(None), Habit.start_date <= target_date),
            or_(Habit.end_date.is_(None), Habit.end_date >= target_date),
        ]
        return await self.get_all(*filters)

    async def search_by_name(self, name: str) -> Habit | None:
        """Search for a habit by name."""
        return await self.get_by(name=func.lower(name).like(f"%%{name}%%"))


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

    async def get_value_for_date(
        self,
        habit_id: int,
        target_date: date,
    ) -> Decimal | None:
        """Get the log value for a specific date, or None if no log exists."""
        stmt = (
            select(HabitLog.value)
            .where(HabitLog.habit_id == habit_id)
            .where(HabitLog.log_date == target_date)
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_logs_in_range(
        self,
        habit_id: int,
        start_date: date,
        end_date: date,
    ) -> Sequence[HabitLog]:
        """Get all logs for a habit within a date range."""
        filters = [
            HabitLog.habit_id == habit_id,
            HabitLog.log_date >= start_date,
            HabitLog.log_date <= end_date,
        ]
        return await self.get_all(*filters, order_by=HabitLog.log_date.asc())

    async def get_period_sum(
        self,
        habit_id: int,
        start_date: date,
        end_date: date,
    ) -> Decimal:
        """Get the sum of log values for a habit within a date range."""
        stmt = (
            select(func.coalesce(func.sum(HabitLog.value), Decimal("0")))
            .where(HabitLog.habit_id == habit_id)
            .where(HabitLog.log_date >= start_date)
            .where(HabitLog.log_date <= end_date)
        )
        result = await self.session.execute(stmt)
        return result.scalar() or Decimal("0")

    async def get_stats_for_habit(
        self,
        habit: Habit,
        start_date: date,
        end_date: date,
    ) -> tuple[int, Decimal | None, int]:
        """
        Get aggregated stats for a habit over a period.
        Returns (total_logs, average_value, targets_met_count).
        """
        target_met_expr = self._build_target_met_expression(habit)

        stmt = (
            select(
                func.count(HabitLog.id).label("total_logs"),
                func.avg(HabitLog.value).label("average_value"),
                func.sum(case((target_met_expr, 1), else_=0)).label("targets_met"),
            )
            .where(HabitLog.habit_id == habit.id)
            .where(HabitLog.log_date >= start_date)
            .where(HabitLog.log_date <= end_date)
        )

        result = await self.session.execute(stmt)
        row = result.one()

        total_logs = row.total_logs or 0
        average_value = Decimal(str(row.average_value)) if row.average_value is not None else None
        targets_met = row.targets_met or 0

        return total_logs, average_value, targets_met

    async def get_dates_with_target_met(
        self,
        habit: Habit,
        start_date: date | None = None,
    ) -> Sequence[date]:
        """Get all dates where the target was met for a habit."""
        target_met_expr = self._build_target_met_expression(habit)

        stmt = (
            select(HabitLog.log_date)
            .where(HabitLog.habit_id == habit.id)
            .where(target_met_expr)
            .order_by(HabitLog.log_date)
        )

        if start_date:
            stmt = stmt.where(HabitLog.log_date >= start_date)

        result = await self.session.execute(stmt)
        return [row[0] for row in result.all()]

    async def get_all_log_dates(
        self,
        habit_id: int,
        start_date: date | None = None,
    ) -> Sequence[date]:
        """Get all dates with logs for a habit."""
        stmt = (
            select(HabitLog.log_date)
            .where(HabitLog.habit_id == habit_id)
            .order_by(HabitLog.log_date)
        )

        if start_date:
            stmt = stmt.where(HabitLog.log_date >= start_date)

        result = await self.session.execute(stmt)
        return [row[0] for row in result.all()]

    def _build_target_met_expression(self, habit: Habit):
        """Build SQL expression for checking if target is met."""
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
                return and_(
                    HabitLog.value >= habit.target_min,
                    HabitLog.value <= habit.target_max,
                )
