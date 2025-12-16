"""Integration tests for habit log CRUD operations."""

from datetime import date, timedelta
from decimal import Decimal

import pytest

from app.core import ConflictError, NotFoundError
from app.habits.enums import ValueType
from app.habits.schemas import HabitCreate, HabitLogCreate, HabitLogUpdate
from app.habits.service import HabitLogService, HabitService


@pytest.fixture
async def habit(habit_service: HabitService, _clean_habits):
    return await habit_service.create(HabitCreate(name="Test Habit", value_type=ValueType.boolean))


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
