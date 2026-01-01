"""Habit-related constants and error messages."""

# Default values
DEFAULT_ICON = "fas fa-check"


class ErrorMessages:
    BOOLEAN_NO_NUMERIC_TARGETS = "Boolean habits cannot have numeric targets"
    BOOLEAN_ONLY_EQUALS = "Boolean habits can only use 'equals' comparison"
    RANGE_REQUIRES_MIN_MAX = "Range comparison requires target_min and target_max"
    MIN_MUST_BE_LESS_THAN_MAX = "target_min must be less than target_max"
    COMPARISON_REQUIRES_TARGET = "Comparison '{}' requires target_value"
    BOOLEAN_ONLY_ZERO_OR_ONE = "Boolean habits only accept 0 or 1"
    NUMERIC_MUST_BE_NON_NEGATIVE = "Numeric habit values must be non-negative"
