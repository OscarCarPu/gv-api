"""Habit-related enumerations."""

from enum import Enum


class TargetFrequency(str, Enum):
    daily = "daily"
    weekly = "weekly"
    monthly = "monthly"


class ValueType(str, Enum):
    boolean = "boolean"
    numeric = "numeric"


class ComparisonType(str, Enum):
    equals = "equals"
    greater_than = "greater_than"
    less_than = "less_than"
    greater_equal_than = "greater_equal_than"
    less_equal_than = "less_equal_than"
    in_range = "in_range"
