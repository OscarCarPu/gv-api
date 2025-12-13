"""Habit-specific validation functions."""

from decimal import Decimal

from app.habits.constants import ErrorMessages
from app.habits.enums import ComparisonType, ValueType


def validate_target_config(
    value_type: ValueType,
    comparison_type: ComparisonType | None,
    target_value: Decimal | None,
    target_min: Decimal | None,
    target_max: Decimal | None,
) -> None:
    """Validate that target configuration is consistent."""
    if value_type == ValueType.boolean:
        if any([target_value, target_min, target_max]):
            raise ValueError(ErrorMessages.BOOLEAN_NO_NUMERIC_TARGETS)
        if comparison_type and comparison_type != ComparisonType.equals:
            raise ValueError(ErrorMessages.BOOLEAN_ONLY_EQUALS)
        return

    # Numeric validation
    if comparison_type == ComparisonType.in_range:
        if target_min is None or target_max is None:
            raise ValueError(ErrorMessages.RANGE_REQUIRES_MIN_MAX)
        if target_min >= target_max:
            raise ValueError(ErrorMessages.MIN_MUST_BE_LESS_THAN_MAX)
    elif comparison_type is not None and comparison_type in (
        ComparisonType.equals,
        ComparisonType.greater_than,
        ComparisonType.less_than,
        ComparisonType.greater_equal_than,
        ComparisonType.less_equal_than,
    ):
        if target_value is None:
            raise ValueError(ErrorMessages.COMPARISON_REQUIRES_TARGET.format(comparison_type.value))


def validate_habit_log_value(value: Decimal, value_type: ValueType) -> None:
    """Validate that the log value is compatible with the habit's value type."""
    if value_type == ValueType.boolean and value not in (Decimal("0"), Decimal("1")):
        raise ValueError(ErrorMessages.BOOLEAN_ONLY_ZERO_OR_ONE)
    if value_type == ValueType.numeric and value < Decimal("0"):
        raise ValueError(ErrorMessages.NUMERIC_MUST_BE_NON_NEGATIVE)
