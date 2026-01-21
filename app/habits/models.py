from datetime import date
from decimal import Decimal

from sqlalchemy import ForeignKey, Numeric, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.common.validations import (
    NAME_MAX_LENGTH,
    UNIT_MAX_LENGTH,
)
from app.core.database import Base
from app.habits.constants import DEFAULT_ICON
from app.habits.enums import ComparisonType, TargetFrequency, ValueType


class Habit(Base):
    name: Mapped[str] = mapped_column(String(NAME_MAX_LENGTH))
    description: Mapped[str | None] = mapped_column(default=None)
    value_type: Mapped[ValueType]
    unit: Mapped[str | None] = mapped_column(String(UNIT_MAX_LENGTH), default=None)
    frequency: Mapped[TargetFrequency] = mapped_column(default=TargetFrequency.daily)
    target_value: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    target_min: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    target_max: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    comparison_type: Mapped[ComparisonType | None] = mapped_column(default=None)
    start_date: Mapped[date | None] = mapped_column(default=None)
    end_date: Mapped[date | None] = mapped_column(default=None)
    default_value: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    streak_strict: Mapped[bool] = mapped_column(default=False)

    @property
    def has_target(self) -> bool:
        return self.target_value is not None or self.comparison_type is not None

    icon: Mapped[str] = mapped_column(default=DEFAULT_ICON)
    big_step: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)
    small_step: Mapped[Decimal | None] = mapped_column(Numeric(10, 2), default=None)

    logs: Mapped[list["HabitLog"]] = relationship(back_populates="habit")  # noqa: UP037

    def __str__(self) -> str:
        """Return LLM-friendly string representation of the habit."""
        parts = [f"ID {self.id}: {self.name} ({self.value_type.value}"]

        if self.value_type == ValueType.numeric and self.unit:
            parts.append(f", unit: {self.unit}")

        if self.comparison_type:
            if self.comparison_type == ComparisonType.in_range:
                target_str = f", target: {self.target_min}-{self.target_max}"
            else:
                symbols = {
                    ComparisonType.equals: "=",
                    ComparisonType.greater_than: ">",
                    ComparisonType.less_than: "<",
                    ComparisonType.greater_equal_than: ">=",
                    ComparisonType.less_equal_than: "<=",
                }
                target_str = (
                    f", target: {symbols.get(self.comparison_type, '')} {self.target_value}"
                )
            if self.unit:
                target_str += f" {self.unit}"
            parts.append(target_str)

        parts.append(f", {self.frequency.value})")
        return "".join(parts)


class HabitLog(Base):
    __table_args__ = (UniqueConstraint("habit_id", "log_date", name="uq_habit_log_date"),)

    habit_id: Mapped[int] = mapped_column(ForeignKey("habit.id"), index=True)
    log_date: Mapped[date] = mapped_column(index=True)
    value: Mapped[Decimal] = mapped_column(Numeric(10, 2))

    habit: Mapped["Habit"] = relationship(back_populates="logs")  # noqa: UP037

    def __str__(self) -> str:
        return f"ID {self.id}: {self.habit.name} ({self.habit.value_type.value})"
