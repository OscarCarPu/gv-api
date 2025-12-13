"""Unit tests for streak algorithm."""

from datetime import date, timedelta
from decimal import Decimal

import pytest
from pytest_mock import MockerFixture

from app.habits.enums import TargetFrequency
from app.habits.service import HabitService

from .factories import HabitFactory


@pytest.fixture
def service(mocker: MockerFixture) -> HabitService:
    return HabitService(mocker.MagicMock(), mocker.MagicMock())


class TestCheckTargetMet:
    def test_boolean_habit(self, service: HabitService):
        habit = HabitFactory.build()
        assert service.check_target_met(habit, Decimal("1")) is True
        assert service.check_target_met(habit, Decimal("0")) is False

    def test_numeric_gte(self, service: HabitService):
        habit = HabitFactory.build(numeric_gte=True)
        assert service.check_target_met(habit, Decimal("10")) is True
        assert service.check_target_met(habit, Decimal("9")) is False

    def test_numeric_in_range(self, service: HabitService):
        habit = HabitFactory.build(numeric_range=True)
        assert service.check_target_met(habit, Decimal("10")) is True
        assert service.check_target_met(habit, Decimal("4")) is False


class TestIsDueOnDate:
    def test_daily_always_due(self, service: HabitService):
        habit = HabitFactory.build()
        assert service._is_due_on_date(habit, date.today()) is True

    def test_weekly_same_weekday(self, service: HabitService):
        monday = date(2024, 1, 1)
        habit = HabitFactory.build(frequency=TargetFrequency.weekly, start_date=monday)
        assert service._is_due_on_date(habit, monday + timedelta(days=7)) is True
        assert service._is_due_on_date(habit, monday + timedelta(days=1)) is False

    def test_monthly_same_day(self, service: HabitService):
        start = date(2024, 1, 15)
        habit = HabitFactory.build(frequency=TargetFrequency.monthly, start_date=start)
        assert service._is_due_on_date(habit, date(2024, 2, 15)) is True
        assert service._is_due_on_date(habit, date(2024, 2, 16)) is False


class TestCalculateStreaksFromDates:
    def test_empty_dates(self, service: HabitService):
        habit = HabitFactory.build()
        current, longest = service._calculate_streaks_from_dates(habit, set())
        assert current == 0 and longest == 0

    def test_consecutive_days(self, service: HabitService):
        habit = HabitFactory.build()
        today = date.today()
        dates = {today, today - timedelta(days=1), today - timedelta(days=2)}
        current, longest = service._calculate_streaks_from_dates(habit, dates)
        assert current == 3 and longest == 3

    def test_gap_breaks_streak(self, service: HabitService):
        habit = HabitFactory.build()
        today = date.today()
        dates = {today, today - timedelta(days=2)}
        current, longest = service._calculate_streaks_from_dates(habit, dates)
        assert current == 1

    def test_no_log_today_breaks_current(self, service: HabitService):
        habit = HabitFactory.build()
        today = date.today()
        dates = {today - timedelta(days=1), today - timedelta(days=2)}
        current, longest = service._calculate_streaks_from_dates(habit, dates)
        assert current == 0 and longest == 2
