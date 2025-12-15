from datetime import date
from decimal import Decimal

from sqlalchemy import Connection, ForeignKey, Numeric, String, UniqueConstraint, event, select
from sqlalchemy.orm import Mapped, Mapper, mapped_column, relationship, validates

from app.common.validations import (
    COLOR_MAX_LENGTH,
    NAME_MAX_LENGTH,
    UNIT_MAX_LENGTH,
    sanitize_color,
    sanitize_description,
    sanitize_name,
    sanitize_unit,
    validate_icon,
)
from app.core.database import Base
from app.habits.constants import DEFAULT_COLOR, DEFAULT_ICON
from app.habits.enums import ComparisonType, TargetFrequency, ValueType
from app.habits.validations import validate_target_config


class Habit(Base):
    name: Mapped[str] = mapped_column(String(NAME_MAX_LENGTH))
    description: Mapped[str | None] = mapped_column(default=None)
    value_type: Mapped[ValueType]
    unit: Mapped[str | None] = mapped_column(String(UNIT_MAX_LENGTH), default=None)
    frequency: Mapped[TargetFrequency | None] = mapped_column(default=None)
    target_value: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    target_min: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    target_max: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    comparison_type: Mapped[ComparisonType | None] = mapped_column(default=None)
    start_date: Mapped[date | None] = mapped_column(default=None)
    end_date: Mapped[date | None] = mapped_column(default=None)
    is_required: Mapped[bool] = mapped_column(default=True)
    color: Mapped[str] = mapped_column(String(COLOR_MAX_LENGTH), default=DEFAULT_COLOR)
    icon: Mapped[str] = mapped_column(default=DEFAULT_ICON)

    logs: Mapped[list["HabitLog"]] = relationship(back_populates="habit")  # noqa: UP037

    @validates("name")
    def _validate_name(self, key: str, value: str) -> str:
        return sanitize_name(value)

    @validates("description")
    def _validate_description(self, key: str, value: str | None) -> str | None:
        return sanitize_description(value)

    @validates("unit")
    def _validate_unit(self, key: str, value: str | None) -> str | None:
        return sanitize_unit(value)

    @validates("color")
    def _validate_color(self, key: str, value: str) -> str:
        return sanitize_color(value)

    @validates("icon")
    def _validate_icon(self, key: str, value: str) -> str:
        return validate_icon(value)

    def validate_target_config(self) -> None:
        """Validate that target configuration is consistent."""
        validate_target_config(
            self.value_type,
            self.comparison_type,
            self.target_value,
            self.target_min,
            self.target_max,
        )


@event.listens_for(Habit, "before_insert")
@event.listens_for(Habit, "before_update")
def _validate_habit_before_persist(
    mapper: Mapper[Habit], connection: object, target: Habit
) -> None:
    """Validate Habit state before insert or update."""
    target.validate_target_config()


class HabitLog(Base):
    __table_args__ = (UniqueConstraint("habit_id", "log_date", name="uq_habit_log_date"),)

    habit_id: Mapped[int] = mapped_column(ForeignKey("habit.id"), index=True)
    log_date: Mapped[date] = mapped_column(index=True)
    value: Mapped[Decimal] = mapped_column(Numeric(10, 2))

    habit: Mapped["Habit"] = relationship(back_populates="logs")  # noqa: UP037

    def validate_value_for_type(self, value_type: ValueType) -> None:
        """Validate that the log value is compatible with the habit's value type."""
        if value_type == ValueType.boolean and self.value not in (Decimal("0"), Decimal("1")):
            raise ValueError("Boolean habits only accept 0 or 1")
        if value_type == ValueType.numeric and self.value < Decimal("0"):
            raise ValueError("Numeric habit values must be non-negative")


@event.listens_for(HabitLog, "before_insert")
@event.listens_for(HabitLog, "before_update")
def _validate_habit_log_before_persist(
    mapper: Mapper[HabitLog], connection: Connection, target: HabitLog
) -> None:
    """Validate HabitLog value against the habit's value_type."""
    result = connection.execute(select(Habit.value_type).where(Habit.id == target.habit_id))
    row = result.first()
    if row:
        target.validate_value_for_type(row[0])
