"""Habit-related constants and error messages."""

# Default values
DEFAULT_ICON = "fas fa-check"


class ErrorMessages:
    RANGE_REQUIRES_MIN_MAX = "Range comparison requires target_min and target_max"
    MIN_MUST_BE_LESS_THAN_MAX = "target_min must be less than target_max"
    COMPARISON_REQUIRES_TARGET = "Comparison '{}' requires target_value"
    BOOLEAN_ONLY_ZERO_OR_ONE = "Boolean habits only accept 0 or 1"
    NUMERIC_MUST_BE_NON_NEGATIVE = "Numeric habit values must be non-negative"
    STEP_MUST_BE_POSITIVE = "Step values must be positive"
