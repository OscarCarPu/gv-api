"""Test factories for habit models."""

from datetime import date, datetime
from decimal import Decimal
from functools import partial

from factory.base import Factory
from factory.declarations import LazyFunction, Sequence, Trait

from app.core import TZ
from app.habits.enums import ComparisonType, TargetFrequency, ValueType
from app.habits.models import Habit, HabitLog


class HabitFactory(Factory):
    class Meta:  # type: ignore
        model = Habit

    id = Sequence(lambda n: n + 1)
    name = Sequence(lambda n: f"Habit {n}")
    description = None
    value_type = ValueType.boolean
    unit = None
    frequency = TargetFrequency.daily
    target_value = None
    target_min = None
    target_max = None
    comparison_type = None
    start_date = None
    end_date = None
    default_value = None
    streak_strict = True
    icon = "fas fa-check"
    big_step = None
    small_step = None
    created_at = LazyFunction(partial(datetime.now, TZ))
    updated_at = LazyFunction(partial(datetime.now, TZ))

    class Params:
        numeric_gte = Trait(
            value_type=ValueType.numeric,
            comparison_type=ComparisonType.greater_equal_than,
            target_value=Decimal("10"),
        )
        numeric_range = Trait(
            value_type=ValueType.numeric,
            comparison_type=ComparisonType.in_range,
            target_value=Decimal("10"),  # Required to pass early check in service
            target_min=Decimal("5"),
            target_max=Decimal("15"),
        )
        weekly = Trait(
            frequency=TargetFrequency.weekly,
            start_date=LazyFunction(date.today),
        )
        monthly = Trait(
            frequency=TargetFrequency.monthly,
            start_date=LazyFunction(date.today),
        )


class HabitLogFactory(Factory):
    class Meta:  # type: ignore
        model = HabitLog

    id = Sequence(lambda n: n + 1)
    habit_id = 1
    log_date = LazyFunction(date.today)
    value = Decimal("1")
    created_at = LazyFunction(partial(datetime.now, TZ))
    updated_at = LazyFunction(partial(datetime.now, TZ))
