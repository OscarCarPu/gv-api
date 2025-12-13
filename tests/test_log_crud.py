"""Integration tests for habit log CRUD operations."""

from datetime import date, timedelta
from decimal import Decimal

import pytest

from app.core import ConflictError, NotFoundError
from app.habits.enums import ComparisonType, ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate, HabitLogUpdate
from app.habits.service import HabitLogService, HabitService


@pytest.fixture
async def habit(habit_service: HabitService, _clean_habits):
    return await habit_service.create(HabitCreate(name="Test Habit", value_type=ValueType.boolean))


@pytest.fixture
async def numeric_habit(habit_service: HabitService, _clean_habits):
    return await habit_service.create(
        HabitCreate(
            name="Numeric Habit",
            value_type=ValueType.numeric,
            comparison_type=ComparisonType.greater_equal_than,
            target_value=Decimal("10"),
        )
    )


class TestLogCreate:
    async def test_create_log(self, log_service: HabitLogService, habit):
        habit_id = habit.id
        data = HabitLogCreate(habit_id=habit_id, log_date=date.today(), value=Decimal("1"))
        log = await log_service.create(data)
        assert log.habit_id == habit_id
        assert log.value == Decimal("1")

    async def test_create_duplicate_log_fails(self, log_service: HabitLogService, habit):
        habit_id = habit.id
        data = HabitLogCreate(habit_id=habit_id, log_date=date.today(), value=Decimal("1"))
        await log_service.create(data)
        with pytest.raises(ConflictError):
            await log_service.create(data)


class TestLogUpsert:
    async def test_upsert_creates_new_log(self, log_service: HabitLogService, habit):
        data = HabitLogCreate(habit_id=habit.id, log_date=date.today(), value=Decimal("1"))
        log, created = await log_service.upsert(data)
        assert created is True

    async def test_upsert_updates_existing_log(self, log_service: HabitLogService, habit):
        data = HabitLogCreate(habit_id=habit.id, log_date=date.today(), value=Decimal("1"))
        await log_service.upsert(data)
        data.value = Decimal("0")
        log, created = await log_service.upsert(data)
        assert created is False
        assert log.value == Decimal("0")


class TestQuickLog:
    async def test_quick_log_boolean_defaults_to_1(self, log_service: HabitLogService, habit):
        log = await log_service.quick_log(habit.id)
        assert log.value == Decimal("1")
        assert log.log_date == date.today()

    async def test_quick_log_numeric_uses_target(self, log_service: HabitLogService, numeric_habit):
        log = await log_service.quick_log(numeric_habit.id)
        assert log.value == Decimal("10")


class TestLogList:
    async def test_list_logs_by_habit(self, log_service: HabitLogService, habit):
        habit_id = habit.id
        today = date.today()
        for i in range(3):
            await log_service.create(
                HabitLogCreate(
                    habit_id=habit_id, log_date=today - timedelta(days=i), value=Decimal("1")
                )
            )
        result = await log_service.list_by_habit(habit_id)
        assert result.total == 3

    async def test_list_logs_with_date_range(self, log_service: HabitLogService, habit):
        habit_id = habit.id
        today = date.today()
        for i in range(5):
            await log_service.create(
                HabitLogCreate(
                    habit_id=habit_id, log_date=today - timedelta(days=i), value=Decimal("1")
                )
            )
        result = await log_service.list_by_habit(
            habit_id, start_date=today - timedelta(days=2), end_date=today
        )
        assert result.total == 3


class TestLogUpdateDelete:
    async def test_update_log_value(self, log_service: HabitLogService, habit):
        data = HabitLogCreate(habit_id=habit.id, log_date=date.today(), value=Decimal("1"))
        log = await log_service.create(data)
        updated = await log_service.update(log.id, HabitLogUpdate(value=Decimal("0")))
        assert updated.value == Decimal("0")

    async def test_delete_log(self, log_service: HabitLogService, habit):
        data = HabitLogCreate(habit_id=habit.id, log_date=date.today(), value=Decimal("1"))
        log = await log_service.create(data)
        await log_service.delete(log.id)
        with pytest.raises(NotFoundError):
            await log_service.get(log.id)
