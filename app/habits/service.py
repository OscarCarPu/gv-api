import math
from datetime import date, timedelta
from decimal import Decimal

from app.common.constants import DEFAULT_PAGE, DEFAULT_PAGE_SIZE
from app.core import ConflictError, NotFoundError, ValidationError

from .models import ComparisonType, Habit, HabitLog, TargetFrequency, ValueType
from .repository import HabitLogRepository, HabitRepository
from .schemas import (
    HabitCreate,
    HabitLogCreate,
    HabitLogRead,
    HabitLogUpdate,
    HabitRead,
    HabitTodayStats,
    HabitUpdate,
    PaginatedResponse,
)


class HabitService:
    def __init__(
        self,
        habit_repo: HabitRepository,
        log_repo: HabitLogRepository,
    ):
        self.habit_repo = habit_repo
        self.log_repo = log_repo

    # --- Core CRUD ---

    async def get(self, habit_id: int) -> Habit:
        habit = await self.habit_repo.get(habit_id)
        if not habit:
            raise NotFoundError("Habit not found")
        return habit

    async def get_all(
        self,
        frequency: TargetFrequency | None = None,
        page: int = DEFAULT_PAGE,
        page_size: int = DEFAULT_PAGE_SIZE,
    ) -> PaginatedResponse[HabitRead]:
        offset = (page - 1) * page_size
        total = await self.habit_repo.count_all(frequency)
        items = await self.habit_repo.get_all(frequency=frequency, limit=page_size, offset=offset)
        return PaginatedResponse(
            items=[HabitRead.model_validate(item) for item in items],
            total=total,
            page=page,
            page_size=page_size,
            total_pages=math.ceil(total / page_size) if total > 0 else 0,
        )

    async def create(self, data: HabitCreate) -> Habit:
        # Check for duplicate name
        existing = await self.habit_repo.get_by_name(data.name)
        if existing:
            raise ConflictError(f"Habit '{data.name}' already exists")

        habit = Habit(**data.model_dump())
        return await self.habit_repo.create(habit)

    async def update(self, habit_id: int, data: HabitUpdate) -> Habit:
        habit = await self.get(habit_id)

        # Check name uniqueness if changing
        if data.name and data.name != habit.name:
            existing = await self.habit_repo.get_by_name(data.name)
            if existing:
                raise ConflictError(f"Habit '{data.name}' already exists")

        update_data = data.model_dump(exclude_unset=True)
        for key, value in update_data.items():
            setattr(habit, key, value)

        return await self.habit_repo.update(habit)

    async def delete(self, habit_id: int) -> None:
        habit = await self.get(habit_id)
        await self.habit_repo.delete(habit)

    # --- Daily Habits Stats ---

    async def get_daily_habits(self, target_date: date | None = None) -> list[HabitTodayStats]:
        """Get all active habits for a date with their statistics."""
        if target_date is None:
            target_date = date.today()
        habits = await self.habit_repo.get_active_habits(target_date)

        results = []
        for habit in habits:
            stats = await self._calculate_habit_stats(habit, target_date)
            results.append(stats)

        return results

    async def _calculate_habit_stats(self, habit: Habit, today: date) -> HabitTodayStats:
        """Calculate all stats for a single habit."""
        # Get period boundaries
        period_start, period_end = self._get_period_boundaries(habit.frequency, today)

        # Get current period value (sum of logs in current period)
        current_period_value = await self.log_repo.get_period_sum(
            habit.id, period_start, period_end
        )

        # Get value for the specific target date (single day's log)
        # If no log exists and default_value is set, use the default
        date_value = await self.log_repo.get_value_for_date(habit.id, today)
        if date_value is None and habit.default_value is not None:
            date_value = habit.default_value

        # Calculate stats for last 30 periods (excluding current period)
        stats_start = self._get_periods_ago(habit.frequency, today, 30)
        stats_end = period_start - timedelta(days=1)  # Exclude current period

        has_objective = habit.target_value is not None or habit.comparison_type is not None

        if stats_end >= stats_start:
            total_logs, avg_value, periods_met = await self.log_repo.get_stats_for_habit(
                habit, stats_start, stats_end
            )
            if has_objective:
                expected_periods = self._count_periods_between(
                    habit.frequency, stats_start, stats_end
                )
                avg_completion = (
                    Decimal(periods_met) / Decimal(max(expected_periods, 1)) * 100
                ).quantize(Decimal("0.1"))
            else:
                avg_completion = None

        else:
            avg_value = None
            avg_completion = None if not has_objective else Decimal("0")

        # Calculate streaks
        dates_met = await self.log_repo.get_dates_with_target_met(habit)
        all_log_dates = await self.log_repo.get_all_log_dates(habit.id)
        current_streak, longest_streak = self._calculate_streaks(
            habit, today, set(dates_met), set(all_log_dates)
        )

        return HabitTodayStats(
            id=habit.id,
            name=habit.name,
            description=habit.description,
            value_type=habit.value_type,
            unit=habit.unit,
            frequency=habit.frequency,
            target_value=habit.target_value,
            target_min=habit.target_min,
            target_max=habit.target_max,
            comparison_type=habit.comparison_type,
            default_value=habit.default_value,
            streak_strict=habit.streak_strict,
            icon=habit.icon,
            big_step=habit.big_step,
            small_step=habit.small_step,
            current_streak=current_streak if current_streak is not None else None,
            longest_streak=longest_streak if longest_streak is not None else None,
            average_value=avg_value,
            average_completion_rate=avg_completion,
            current_period_value=current_period_value,
            date_value=date_value,
        )

    def _get_period_boundaries(
        self, frequency: TargetFrequency, target_date: date
    ) -> tuple[date, date]:
        """Get the start and end dates for the period containing target_date."""
        match frequency:
            case TargetFrequency.daily:
                return target_date, target_date
            case TargetFrequency.weekly:
                # Week starts on Monday (weekday 0)
                start = target_date - timedelta(days=target_date.weekday())
                end = start + timedelta(days=6)
                return start, end
            case TargetFrequency.monthly:
                start = target_date.replace(day=1)
                # Get last day of month
                if target_date.month == 12:
                    end = target_date.replace(day=31)
                else:
                    end = target_date.replace(month=target_date.month + 1, day=1) - timedelta(
                        days=1
                    )
                return start, end
            case _:
                return target_date, target_date

    def _get_periods_ago(self, frequency: TargetFrequency, target_date: date, periods: int) -> date:
        """Get the start date of N periods ago."""
        match frequency:
            case TargetFrequency.daily:
                return target_date - timedelta(days=periods)
            case TargetFrequency.weekly:
                return target_date - timedelta(weeks=periods)
            case TargetFrequency.monthly:
                # Approximate: go back periods months
                year = target_date.year
                month = target_date.month - periods
                while month <= 0:
                    month += 12
                    year -= 1
                return date(year, month, 1)
            case _:
                return target_date - timedelta(days=periods)

    def _count_periods_between(
        self, frequency: TargetFrequency, start_date: date, end_date: date
    ) -> int:
        """Count the number of periods between two dates."""
        if end_date < start_date:
            return 0

        match frequency:
            case TargetFrequency.daily:
                return (end_date - start_date).days + 1
            case TargetFrequency.weekly:
                # Count weeks": "201.00
                start_week = start_date - timedelta(days=start_date.weekday())
                end_week = end_date - timedelta(days=end_date.weekday())
                return ((end_week - start_week).days // 7) + 1
            case TargetFrequency.monthly:
                # Count months
                months = (end_date.year - start_date.year) * 12
                months += end_date.month - start_date.month + 1
                return months

    def _calculate_streaks(
        self,
        habit: Habit,
        today: date,
        dates_met: set[date],
        all_log_dates: set[date],
    ) -> tuple[int | None, int | None]:
        """Calculate current and longest streaks. If habit has no target, streaks are None."""
        if not habit.has_target:
            return None, None
        if not dates_met:
            return 0, 0

        # For streak calculation, we need to check consecutive periods
        current_streak = self._calculate_current_streak(habit, today, dates_met, all_log_dates)
        longest_streak = self._calculate_longest_streak(habit, dates_met, all_log_dates)

        return current_streak, longest_streak

    def _calculate_current_streak(
        self,
        habit: Habit,
        today: date,
        dates_met: set[date],
        all_log_dates: set[date],
    ) -> int:
        """Calculate current streak counting backwards from today."""
        streak = 0
        check_date = today
        min_log_date = min(all_log_dates) if all_log_dates else today
        while True:
            period_start, period_end = self._get_period_boundaries(habit.frequency, check_date)
            period_met = any(d in dates_met for d in self._dates_in_range(period_start, period_end))
            if period_met:
                streak += 1
            else:
                # If habit has a target, use streak_strict to decide if missing day breaks streak
                if habit.has_target:
                    if habit.streak_strict:
                        break
                    else:
                        # For non-strict, missing log does not break streak
                        period_logged = any(
                            d in all_log_dates
                            for d in self._dates_in_range(period_start, period_end)
                        )
                        if period_logged:
                            break
                        if period_end < min_log_date:
                            break
                        streak += 1
                else:
                    # No target: treat as strict
                    break
            check_date = period_start - timedelta(days=1)
            if streak > 1000 or check_date.year < 2000:
                break
        return streak

    def _calculate_longest_streak(
        self,
        habit: Habit,
        dates_met: set[date],
        all_log_dates: set[date],
    ) -> int:
        """Calculate the longest streak ever."""
        if not dates_met:
            return 0

        # Sort dates and find longest consecutive period sequence
        sorted_dates = sorted(dates_met)
        min_date = sorted_dates[0]
        max_date = sorted_dates[-1]

        longest = 0
        current = 0
        check_date = min_date

        while check_date <= max_date:
            period_start, period_end = self._get_period_boundaries(habit.frequency, check_date)

            period_met = any(d in dates_met for d in self._dates_in_range(period_start, period_end))

            if period_met:
                current += 1
                longest = max(longest, current)
            else:
                if habit.default_value is None:
                    current = 0
                else:
                    period_logged = any(
                        d in all_log_dates for d in self._dates_in_range(period_start, period_end)
                    )
                    if period_logged:
                        current = 0
                    else:
                        # No log - still counts towards streak for non-required
                        current += 1
                        longest = max(longest, current)

            # Move to next period
            check_date = period_end + timedelta(days=1)

        return longest

    def _dates_in_range(self, start: date, end: date) -> list[date]:
        """Generate all dates in a range."""
        dates = []
        current = start
        while current <= end:
            dates.append(current)
            current += timedelta(days=1)
        return dates

    def _check_target_met(self, habit: Habit, value: Decimal) -> bool:
        """Check if a value meets the habit's target."""
        if habit.value_type == ValueType.boolean:
            return value == Decimal("1")

        if habit.comparison_type is None or habit.target_value is None:
            return True

        match habit.comparison_type:
            case ComparisonType.equals:
                return value == habit.target_value
            case ComparisonType.greater_than:
                return value > habit.target_value
            case ComparisonType.less_than:
                return value < habit.target_value
            case ComparisonType.greater_equal_than:
                return value >= habit.target_value
            case ComparisonType.less_equal_than:
                return value <= habit.target_value
            case ComparisonType.in_range:
                if habit.target_min is None or habit.target_max is None:
                    return True
                return habit.target_min <= value <= habit.target_max


class HabitLogService:
    def __init__(
        self,
        habit_repo: HabitRepository,
        log_repo: HabitLogRepository,
    ):
        self.habit_repo = habit_repo
        self.log_repo = log_repo

    async def _get_habit(self, habit_id: int) -> Habit:
        """Helper to get habit or raise NotFoundError."""
        habit = await self.habit_repo.get(habit_id)
        if not habit:
            raise NotFoundError("Habit not found")
        return habit

    def _validate_log_date(self, habit: Habit, log_date: date) -> None:
        """Validate that the log date is within the habit's active period."""
        if habit.start_date and log_date < habit.start_date:
            raise ValidationError(f"Cannot log before habit start date ({habit.start_date})")
        if habit.end_date and log_date > habit.end_date:
            raise ValidationError(f"Cannot log after habit end date ({habit.end_date})")

    async def get(self, log_id: int) -> HabitLog:
        log = await self.log_repo.get(log_id)
        if not log:
            raise NotFoundError("Habit log not found")
        return log

    async def list_by_habit(
        self,
        habit_id: int,
        start_date: date | None = None,
        end_date: date | None = None,
        page: int = DEFAULT_PAGE,
        page_size: int = DEFAULT_PAGE_SIZE,
    ) -> PaginatedResponse[HabitLogRead]:
        await self._get_habit(habit_id)  # Validate habit exists
        offset = (page - 1) * page_size
        total = await self.log_repo.count_by_habit_id(habit_id, start_date, end_date)
        items = await self.log_repo.get_by_habit_id(
            habit_id, start_date, end_date, limit=page_size, offset=offset
        )
        return PaginatedResponse(
            items=[HabitLogRead.model_validate(item) for item in items],
            total=total,
            page=page,
            page_size=page_size,
            total_pages=math.ceil(total / page_size) if total > 0 else 0,
        )

    async def create(self, data: HabitLogCreate) -> HabitLog:
        habit = await self._get_habit(data.habit_id)
        self._validate_log_date(habit, data.log_date)

        # Check for duplicate log on same date
        existing = await self.log_repo.get_by_habit_and_date(data.habit_id, data.log_date)
        if existing:
            raise ConflictError(
                f"Log already exists for this habit on {data.log_date}. Use PATCH to update."
            )

        log = HabitLog(**data.model_dump())
        return await self.log_repo.create(log)

    async def update(self, log_id: int, data: HabitLogUpdate) -> HabitLog:
        log = await self.get(log_id)

        update_data = data.model_dump(exclude_unset=True)
        for key, value in update_data.items():
            setattr(log, key, value)

        return await self.log_repo.update(log)

    async def delete(self, log_id: int) -> None:
        log = await self.get(log_id)
        await self.log_repo.delete(log)

    async def upsert(self, habit_id: int, log_date: date, value: Decimal) -> HabitLog:
        """Upsert a habit log: update if exists, create if not."""
        habit = await self._get_habit(habit_id)
        self._validate_log_date(habit, log_date)

        existing = await self.log_repo.get_by_habit_and_date(habit_id, log_date)
        if existing:
            existing.value = value
            return await self.log_repo.update(existing)

        log = HabitLog(habit_id=habit_id, log_date=log_date, value=value)
        return await self.log_repo.create(log)

    async def modify(self, habit_id: int, log_date: date, value: Decimal) -> HabitLog:
        """Modify a habit log: add value to existing log or create if not exists."""
        habit = await self._get_habit(habit_id)
        self._validate_log_date(habit, log_date)

        existing = await self.log_repo.get_by_habit_and_date(habit_id, log_date)
        if existing:
            existing.value += value
            return await self.log_repo.update(existing)

        log = HabitLog(habit_id=habit_id, log_date=log_date, value=value)
        return await self.log_repo.create(log)
