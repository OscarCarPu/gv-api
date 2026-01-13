from datetime import date, datetime
from decimal import Decimal

from pydantic import BaseModel, ConfigDict, field_validator, model_validator

from app.common.validations import (
    sanitize_description,
    sanitize_name,
    sanitize_unit,
    validate_icon,
)
from app.habits.constants import DEFAULT_ICON, ErrorMessages
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
    frequency: TargetFrequency = TargetFrequency.daily
    target_value: Decimal | None = None
    target_min: Decimal | None = None
    target_max: Decimal | None = None
    comparison_type: ComparisonType | None = None
    start_date: date | None = None
    end_date: date | None = None
    is_required: bool = True
    icon: str = DEFAULT_ICON
    big_step: Decimal | None = None
    small_step: Decimal | None = None

    @field_validator("big_step", "small_step")
    @classmethod
    def _validate_step_positive(cls, v: Decimal | None) -> Decimal | None:
        if v is not None and v <= 0:
            raise ValueError(ErrorMessages.STEP_MUST_BE_POSITIVE)
        return v

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
    value_type: ValueType | None = None
    unit: str | None = None
    frequency: TargetFrequency | None = None
    target_value: Decimal | None = None
    target_min: Decimal | None = None
    target_max: Decimal | None = None
    comparison_type: ComparisonType | None = None
    start_date: date | None = None
    end_date: date | None = None
    is_required: bool | None = None
    icon: str | None = None
    big_step: Decimal | None = None
    small_step: Decimal | None = None

    @field_validator("big_step", "small_step")
    @classmethod
    def _validate_step_positive(cls, v: Decimal | None) -> Decimal | None:
        if v is not None and v <= 0:
            raise ValueError(ErrorMessages.STEP_MUST_BE_POSITIVE)
        return v

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


# --- New Stats Schemas ---


class HabitTodayStats(BaseModel):
    """Stats for a habit on today's dashboard."""

    model_config = ConfigDict(from_attributes=True)

    # Habit info
    id: int
    name: str
    value_type: ValueType
    unit: str | None
    frequency: TargetFrequency
    target_value: Decimal | None
    target_min: Decimal | None
    target_max: Decimal | None
    comparison_type: ComparisonType | None
    is_required: bool
    icon: str
    big_step: Decimal | None
    small_step: Decimal | None

    # Stats
    current_streak: int
    longest_streak: int
    average_value: Decimal | None
    average_completion_rate: Decimal | None
    current_period_value: Decimal | None
    date_value: Decimal | None
