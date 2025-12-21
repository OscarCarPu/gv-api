"""End-to-end tests for the complete habit tracking flow."""

from datetime import date, timedelta
from decimal import Decimal

from httpx import AsyncClient


class TestHabitTrackingFlow:
    """Complete flow: create habit -> log entries -> check stats -> check history."""

    async def test_complete_habit_flow(self, client: AsyncClient, _clean_habits):
        # 1. Create a boolean habit
        response = await client.post(
            "/api/v1/habits",
            json={"name": "Exercise", "value_type": "boolean"},
        )
        assert response.status_code == 201
        habit = response.json()
        habit_id = habit["id"]
        assert habit["name"] == "Exercise"

        # 2. Log entries for past 3 days
        for i in range(3):
            log_date = (date.today() - timedelta(days=i)).isoformat()
            response = await client.post(
                f"/api/v1/habits/{habit_id}/logs",
                json={"log_date": log_date, "value": "1"},
            )
            assert response.status_code == 201

        # 3. Check today's habits (includes stats and streak)
        response = await client.get("/api/v1/habits/daily")
        assert response.status_code == 200
        today_habits = response.json()
        assert len(today_habits) == 1
        habit_stats = today_habits[0]
        assert habit_stats["id"] == habit_id
        assert habit_stats["current_streak"] == 3
        assert Decimal(habit_stats["current_period_value"]) == Decimal("1")

        # 4. Check habit history
        response = await client.get(
            f"/api/v1/habits/{habit_id}/history",
            params={
                "start_date": (date.today() - timedelta(days=2)).isoformat(),
                "end_date": date.today().isoformat(),
            },
        )
        assert response.status_code == 200
        history = response.json()
        assert history["habit_id"] == habit_id
        assert len(history["periods"]) == 3
        # All periods should have value 1 (logged each day)
        assert all(Decimal(p["total_value"]) == Decimal("1") for p in history["periods"])

    async def test_numeric_habit_flow(self, client: AsyncClient, _clean_habits):
        # 1. Create a numeric habit with target
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Water Intake",
                "value_type": "numeric",
                "unit": "glasses",
                "comparison_type": "greater_equal_than",
                "target_value": "8",
            },
        )
        assert response.status_code == 201
        habit_id = response.json()["id"]

        # 2. Log with different values
        values = [10, 6, 8]  # met, not met, met
        for i, value in enumerate(values):
            response = await client.post(
                f"/api/v1/habits/{habit_id}/logs",
                json={
                    "log_date": (date.today() - timedelta(days=i)).isoformat(),
                    "value": str(value),
                },
            )
            assert response.status_code == 201

        # 3. Check today's habits - includes current period value
        response = await client.get("/api/v1/habits/daily")
        assert response.status_code == 200
        today_habits = response.json()
        assert len(today_habits) == 1
        habit_stats = today_habits[0]
        assert Decimal(habit_stats["current_period_value"]) == Decimal("10")
        assert Decimal(habit_stats["target_value"]) == Decimal("8")

        # 4. Check habit history
        response = await client.get(
            f"/api/v1/habits/{habit_id}/history",
            params={
                "start_date": (date.today() - timedelta(days=2)).isoformat(),
                "end_date": date.today().isoformat(),
            },
        )
        assert response.status_code == 200
        history = response.json()
        assert len(history["periods"]) == 3
        # Check values: oldest first (8, 6, 10)
        assert Decimal(history["periods"][0]["total_value"]) == Decimal("8")
        assert Decimal(history["periods"][1]["total_value"]) == Decimal("6")
        assert Decimal(history["periods"][2]["total_value"]) == Decimal("10")

    async def test_habit_update_delete_flow(self, client: AsyncClient, _clean_habits):
        # 1. Create habit
        response = await client.post(
            "/api/v1/habits",
            json={"name": "Original", "value_type": "boolean"},
        )
        habit_id = response.json()["id"]

        # 2. Update habit
        response = await client.patch(
            f"/api/v1/habits/{habit_id}",
            json={"name": "Updated", "color": "#FF0000"},
        )
        assert response.status_code == 200
        assert response.json()["name"] == "Updated"
        assert response.json()["color"] == "#FF0000"

        # 3. Delete habit
        response = await client.delete(f"/api/v1/habits/{habit_id}")
        assert response.status_code == 204

        # 4. Verify deleted
        response = await client.get(f"/api/v1/habits/{habit_id}")
        assert response.status_code == 404
