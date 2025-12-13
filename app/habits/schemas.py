from datetime import date, datetime
from decimal import Decimal

from pydantic import BaseModel, ConfigDict, field_validator, model_validator

from app.common.validations import (
    sanitize_color,
    sanitize_description,
    sanitize_name,
    sanitize_unit,
    validate_icon,
)
from app.habits.constants import DEFAULT_COLOR, DEFAULT_ICON
from app.habits.enums import ComparisonType, TargetFrequency, ValueType
from app.habits.validations import validate_target_config


class PaginatedResponse[T](BaseModel):
    """Generic paginated response wrapper."""

    items: list[T]
    total: int
    page: int
    page_size: int
    total_pages: int


class HabitBase(BaseModel):
    name: str
    description: str | None = None
    value_type: ValueType
    unit: str | None = None
    frequency: TargetFrequency | None = None
    target_value: Decimal | None = None
    target_min: Decimal | None = None
    target_max: Decimal | None = None
    comparison_type: ComparisonType | None = None
    start_date: date | None = None
    end_date: date | None = None
    is_required: bool = True
    color: str = DEFAULT_COLOR
    icon: str = DEFAULT_ICON

    @field_validator("name")
    @classmethod
    def _sanitize_name(cls, v: str) -> str:
        return sanitize_name(v)

    @field_validator("description")
    @classmethod
    def _sanitize_description(cls, v: str | None) -> str | None:
        return sanitize_description(v)

    @field_validator("unit")
    @classmethod
    def _sanitize_unit(cls, v: str | None) -> str | None:
        return sanitize_unit(v)

    @field_validator("color")
    @classmethod
    def _sanitize_color(cls, v: str) -> str:
        return sanitize_color(v)

    @field_validator("icon")
    @classmethod
    def _validate_icon(cls, v: str) -> str:
        return validate_icon(v)

    @model_validator(mode="after")
    def _validate_target_config(self):
        validate_target_config(
            self.value_type,
            self.comparison_type,
            self.target_value,
            self.target_min,
            self.target_max,
        )
        return self


class HabitCreate(HabitBase):
    pass


class HabitUpdate(BaseModel):
    name: str | None = None
    description: str | None = None
    unit: str | None = None
    frequency: TargetFrequency | None = None
    target_value: Decimal | None = None
    target_min: Decimal | None = None
    target_max: Decimal | None = None
    comparison_type: ComparisonType | None = None
    start_date: date | None = None
    end_date: date | None = None
    is_required: bool | None = None
    color: str | None = None
    icon: str | None = None
    active: bool | None = None

    @field_validator("name")
    @classmethod
    def _sanitize_name(cls, v: str | None) -> str | None:
        return sanitize_name(v) if v is not None else None

    @field_validator("description")
    @classmethod
    def _sanitize_description(cls, v: str | None) -> str | None:
        return sanitize_description(v)

    @field_validator("unit")
    @classmethod
    def _sanitize_unit(cls, v: str | None) -> str | None:
        return sanitize_unit(v)

    @field_validator("color")
    @classmethod
    def _sanitize_color(cls, v: str | None) -> str | None:
        return sanitize_color(v) if v is not None else None

    @field_validator("icon")
    @classmethod
    def _validate_icon(cls, v: str | None) -> str | None:
        return validate_icon(v) if v is not None else None


class HabitRead(HabitBase):
    model_config = ConfigDict(from_attributes=True)

    id: int
    created_at: datetime
    updated_at: datetime


class HabitLogBase(BaseModel):
    log_date: date
    value: Decimal


class HabitLogCreate(HabitLogBase):
    habit_id: int


class HabitLogBody(HabitLogBase):
    """Log body for nested endpoints where habit_id comes from URL."""

    pass


class QuickLogBody(BaseModel):
    """Request body for quick-log endpoint."""

    value: Decimal | None = None
    log_date: date | None = None


class HabitLogUpdate(BaseModel):
    log_date: date | None = None
    value: Decimal | None = None


class HabitLogRead(HabitLogBase):
    model_config = ConfigDict(from_attributes=True)

    id: int
    habit_id: int
    created_at: datetime
    updated_at: datetime


class HabitWithLogs(HabitRead):
    logs: list[HabitLogRead] = []


class HabitStats(BaseModel):
    total_logs: int
    completion_rate: Decimal
    current_streak: int
    longest_streak: int
    average_value: Decimal | None


class DailyProgress(BaseModel):
    habit_id: int
    habit_name: str
    is_due: bool
    is_logged: bool
    is_target_met: bool
    logged_value: Decimal | None


class HabitStreak(BaseModel):
    current: int
    longest: int
    last_completed: date | None
