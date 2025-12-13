"""End-to-end tests for the complete habit tracking flow."""

from datetime import date, timedelta

from httpx import AsyncClient


class TestHabitTrackingFlow:
    """Complete flow: create habit -> log entries -> check stats -> check progress."""

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

        # 2. Log entries for past 3 days using quick-log
        for i in range(3):
            log_date = (date.today() - timedelta(days=i)).isoformat()
            response = await client.post(
                f"/api/v1/habits/{habit_id}/quick-log",
                json={"log_date": log_date},
            )
            assert response.status_code == 200

        # 3. Check stats
        response = await client.get(f"/api/v1/habits/{habit_id}/stats", params={"days": 7})
        assert response.status_code == 200
        stats = response.json()
        assert stats["total_logs"] == 3
        assert stats["current_streak"] == 3

        # 4. Check streak
        response = await client.get(f"/api/v1/habits/{habit_id}/streak")
        assert response.status_code == 200
        streak = response.json()
        assert streak["current"] == 3
        assert streak["last_completed"] == date.today().isoformat()

        # 5. Check daily progress
        response = await client.get("/api/v1/habits/daily")
        assert response.status_code == 200
        progress = response.json()
        assert len(progress) == 1
        assert progress[0]["habit_id"] == habit_id
        assert progress[0]["is_logged"] is True
        assert progress[0]["is_target_met"] is True

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

        # 3. Check stats - completion rate should reflect 2/3 met
        response = await client.get(f"/api/v1/habits/{habit_id}/stats", params={"days": 7})
        assert response.status_code == 200
        stats = response.json()
        assert stats["total_logs"] == 3

        # 4. Check daily progress - today (value=10) should meet target
        response = await client.get("/api/v1/habits/daily")
        assert response.status_code == 200
        progress = response.json()
        assert progress[0]["is_target_met"] is True
        assert progress[0]["logged_value"] == "10.00"

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
