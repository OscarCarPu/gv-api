import math
from datetime import date, timedelta
from decimal import Decimal

from app.common.constants import DEFAULT_PAGE, DEFAULT_PAGE_SIZE
from app.core import ConflictError, NotFoundError, ValidationError

from .models import ComparisonType, Habit, HabitLog, TargetFrequency, ValueType
from .repository import HabitLogRepository, HabitRepository
from .schemas import (
    DailyProgress,
    HabitCreate,
    HabitLogCreate,
    HabitLogRead,
    HabitLogUpdate,
    HabitRead,
    HabitStats,
    HabitStreak,
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

    # --- Business Logic ---

    def check_target_met(self, habit: Habit, value: Decimal) -> bool:
        """Check if a logged value meets the habit's target."""
        if habit.value_type == ValueType.boolean:
            return value == Decimal("1")

        if habit.comparison_type is None or habit.target_value is None:
            return True  # No target defined

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

    async def get_stats(self, habit_id: int, days: int = 30) -> HabitStats:
        """Calculate statistics for a habit over a period."""
        habit = await self.get(habit_id)
        start_date = date.today() - timedelta(days=days)

        # Database-level aggregation for stats
        total_logs, average_value, targets_met = await self.log_repo.get_stats_aggregated(
            habit, start_date
        )

        if total_logs == 0:
            return HabitStats(
                total_logs=0,
                completion_rate=Decimal("0"),
                current_streak=0,
                longest_streak=0,
                average_value=None,
            )

        # Calculate completion rate
        expected_days = self._calculate_expected_days(habit, start_date, date.today())
        completion_rate = Decimal(targets_met) / Decimal(max(expected_days, 1)) * 100

        # Calculate streaks using pre-filtered dates from database
        dates_met = await self.log_repo.get_dates_target_met(habit, start_date)
        current_streak, longest_streak = self._calculate_streaks_from_dates(habit, set(dates_met))

        # Only include average for numeric habits
        if habit.value_type != ValueType.numeric:
            average_value = None

        return HabitStats(
            total_logs=total_logs,
            completion_rate=completion_rate.quantize(Decimal("0.1")),
            current_streak=current_streak,
            longest_streak=longest_streak,
            average_value=average_value,
        )

    def _calculate_expected_days(self, habit: Habit, start: date, end: date) -> int:
        """Calculate how many times a habit should have been completed."""
        count = 0
        current = start
        while current <= end:
            if self._is_due_on_date(habit, current):
                count += 1
            current += timedelta(days=1)
        return count

    def _calculate_streaks(self, habit: Habit, logs: list[HabitLog]) -> tuple[int, int]:
        """Calculate current and longest streaks (frequency-aware)."""
        if not logs:
            return 0, 0

        dates_met = {log.log_date for log in logs if self.check_target_met(habit, log.value)}
        return self._calculate_streaks_from_dates(habit, dates_met)

    def _calculate_streaks_from_dates(self, habit: Habit, dates_met: set[date]) -> tuple[int, int]:
        """Calculate current and longest streaks from pre-filtered dates."""
        if not dates_met:
            return 0, 0

        # Current streak: count consecutive due dates met, starting from today backwards
        current_streak = 0
        check_date = date.today()
        min_date = min(dates_met)

        # Find the most recent due date (today or before)
        while check_date >= min_date and not self._is_due_on_date(habit, check_date):
            check_date -= timedelta(days=1)

        # Count consecutive due dates that were met
        while check_date >= min_date:
            if self._is_due_on_date(habit, check_date):
                if check_date in dates_met:
                    current_streak += 1
                else:
                    break  # Streak broken - due date was missed
            check_date -= timedelta(days=1)

        # Longest streak: find consecutive due dates that were all met
        sorted_dates = sorted(dates_met)
        longest_streak = 0
        current_run = 0

        # Build list of all due dates in the log range
        due_dates: list[date] = []
        current = sorted_dates[0]
        while current <= sorted_dates[-1]:
            if self._is_due_on_date(habit, current):
                due_dates.append(current)
            current += timedelta(days=1)

        # Count consecutive due dates that were met
        for due_date in due_dates:
            if due_date in dates_met:
                current_run += 1
                longest_streak = max(longest_streak, current_run)
            else:
                current_run = 0

        return current_streak, longest_streak

    async def get_streak(self, habit_id: int) -> HabitStreak:
        """Get streak information for a habit."""
        habit = await self.get(habit_id)

        # Get pre-filtered dates from database
        dates_met_list = await self.log_repo.get_dates_target_met(habit)

        if not dates_met_list:
            return HabitStreak(current=0, longest=0, last_completed=None)

        dates_met = set(dates_met_list)
        current_streak, longest_streak = self._calculate_streaks_from_dates(habit, dates_met)
        last_completed = max(dates_met_list)

        return HabitStreak(
            current=current_streak,
            longest=longest_streak,
            last_completed=last_completed,
        )

    async def get_daily_progress(self, target_date: date) -> list[DailyProgress]:
        """Get progress for all habits on a specific day."""
        habits = await self.habit_repo.get_all()

        logs = await self.log_repo.get_logs_for_date(target_date)
        logs_by_habit = {log.habit_id: log for log in logs}

        progress = []
        for habit in habits:
            # Skip if habit frequency doesn't apply today
            if habit.id is None or not self._is_due_on_date(habit, target_date):
                continue

            log = logs_by_habit.get(habit.id)
            progress.append(
                DailyProgress(
                    habit_id=habit.id,
                    habit_name=habit.name,
                    is_due=True,
                    is_logged=log is not None,
                    is_target_met=self.check_target_met(habit, log.value) if log else False,
                    logged_value=log.value if log else None,
                )
            )

        return progress

    def _is_due_on_date(self, habit: Habit, target_date: date) -> bool:
        """Check if a habit is due on a specific date."""
        if habit.start_date and target_date < habit.start_date:
            return False
        if habit.end_date and target_date > habit.end_date:
            return False

        match habit.frequency:
            case TargetFrequency.daily:
                return True
            case TargetFrequency.weekly:
                # Due on the same weekday as start_date, or Monday if no start
                start = habit.start_date or habit.created_at.date()
                return target_date.weekday() == start.weekday()
            case TargetFrequency.monthly:
                # Due on the same day of month as start_date
                start = habit.start_date or habit.created_at.date()
                return target_date.day == start.day
            case _:
                return True


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

    async def upsert(self, data: HabitLogCreate) -> tuple[HabitLog, bool]:
        """Create or update a log. Returns (log, was_created)."""
        habit = await self._get_habit(data.habit_id)
        self._validate_log_date(habit, data.log_date)

        existing = await self.log_repo.get_by_habit_and_date(data.habit_id, data.log_date)

        if existing:
            existing.value = data.value
            return await self.log_repo.update(existing), False

        log = HabitLog(**data.model_dump())
        return await self.log_repo.create(log), True

    async def quick_log(
        self,
        habit_id: int,
        value: Decimal | None = None,
        log_date: date | None = None,
    ) -> HabitLog:
        """Quick log with smart defaults."""
        habit = await self._get_habit(habit_id)
        target_date = log_date or date.today()
        self._validate_log_date(habit, target_date)

        # Default value based on type
        if value is None:
            if habit.value_type == ValueType.boolean:
                value = Decimal("1")
            elif habit.target_value:
                value = habit.target_value
            else:
                value = Decimal("1")  # Default for numeric habits without target

        data = HabitLogCreate(
            habit_id=habit_id,
            log_date=target_date,
            value=value,
        )
        log, _ = await self.upsert(data)
        return log

    async def update(self, log_id: int, data: HabitLogUpdate) -> HabitLog:
        log = await self.get(log_id)

        update_data = data.model_dump(exclude_unset=True)
        for key, value in update_data.items():
            setattr(log, key, value)

        return await self.log_repo.update(log)

    async def delete(self, log_id: int) -> None:
        log = await self.get(log_id)
        await self.log_repo.delete(log)
