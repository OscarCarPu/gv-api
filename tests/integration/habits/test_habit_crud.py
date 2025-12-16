"""Integration tests for habit CRUD operations."""

import pytest

from app.core import ConflictError, NotFoundError
from app.habits.enums import ValueType
from app.habits.schemas import HabitCreate, HabitUpdate
from app.habits.service import HabitService


class TestHabitCreate:
    async def test_create_boolean_habit(self, habit_service: HabitService, _clean_habits):
        data = HabitCreate(name="Exercise", value_type=ValueType.boolean)
        habit = await habit_service.create(data)
        assert habit.name == "Exercise"
        assert habit.value_type.value == "boolean"

    async def test_create_duplicate_name_fails(self, habit_service: HabitService, _clean_habits):
        data = HabitCreate(name="Exercise", value_type=ValueType.boolean)
        await habit_service.create(data)
        with pytest.raises(ConflictError):
            await habit_service.create(data)


class TestHabitRead:
    async def test_get_existing_habit(self, habit_service: HabitService, _clean_habits):
        data = HabitCreate(name="Reading", value_type=ValueType.boolean)
        created = await habit_service.create(data)
        habit = await habit_service.get(created.id)
        assert habit.id == created.id

    async def test_get_nonexistent_habit_fails(self, habit_service: HabitService, _clean_habits):
        with pytest.raises(NotFoundError):
            await habit_service.get(99999)

    async def test_get_all_paginated(self, habit_service: HabitService, _clean_habits):
        for i in range(5):
            await habit_service.create(HabitCreate(name=f"Habit {i}", value_type=ValueType.boolean))
        result = await habit_service.get_all(page=1, page_size=2)
        assert len(result.items) == 2
        assert result.total == 5
        assert result.total_pages == 3


class TestHabitUpdate:
    async def test_update_habit_name(self, habit_service: HabitService, _clean_habits):
        data = HabitCreate(name="Old Name", value_type=ValueType.boolean)
        created = await habit_service.create(data)
        updated = await habit_service.update(created.id, HabitUpdate(name="New Name"))
        assert updated.name == "New Name"

    async def test_update_to_existing_name_fails(self, habit_service: HabitService, _clean_habits):
        await habit_service.create(HabitCreate(name="First", value_type=ValueType.boolean))
        second = await habit_service.create(
            HabitCreate(name="Second", value_type=ValueType.boolean)
        )
        with pytest.raises(ConflictError):
            await habit_service.update(second.id, HabitUpdate(name="First"))


class TestHabitDelete:
    async def test_delete_habit(self, habit_service: HabitService, _clean_habits):
        data = HabitCreate(name="ToDelete", value_type=ValueType.boolean)
        created = await habit_service.create(data)
        await habit_service.delete(created.id)
        with pytest.raises(NotFoundError):
            await habit_service.get(created.id)
