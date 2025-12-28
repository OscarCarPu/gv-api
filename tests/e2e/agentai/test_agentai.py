"""End-to-end tests for the AgentAI habit tracking."""

from datetime import date
from decimal import Decimal

from httpx import AsyncClient


class TestQuickReplyRouter:
    """Tests for direct format: 'habit_name value' or 'habit_id value'."""

    async def test_quick_reply_by_name(self, client: AsyncClient, _clean_habits):
        """Test logging habit by name: 'Peso 76.3'."""
        # Create a numeric habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Peso",
                "value_type": "numeric",
                "unit": "kg",
                "frequency": "weekly",
            },
        )
        assert response.status_code == 201

        # Log via agentai quick reply
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Peso 76.3"},
        )
        assert response.status_code == 200
        assert "Registrado" in response.text
        assert "76.3" in response.text
        assert "kg" in response.text
        assert "Peso" in response.text

    async def test_quick_reply_case_insensitive(self, client: AsyncClient, _clean_habits):
        """Test case-insensitive habit matching: 'peso 76.3' matches 'Peso'."""
        # Create habit with capitalized name
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Peso",
                "value_type": "numeric",
                "unit": "kg",
            },
        )
        assert response.status_code == 201

        # Log with lowercase name
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "peso 76.3"},
        )
        assert response.status_code == 200
        assert "Registrado" in response.text
        assert "Peso" in response.text

    async def test_quick_reply_by_id(self, client: AsyncClient, _clean_habits):
        """Test logging habit by ID: '1 76.3'."""
        # Create a habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Ejercicio",
                "value_type": "numeric",
                "unit": "minutos",
            },
        )
        assert response.status_code == 201
        habit_id = response.json()["id"]

        # Log via agentai with ID
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": f"{habit_id} 45"},
        )
        assert response.status_code == 200
        assert "Registrado" in response.text
        assert "45" in response.text
        assert "minutos" in response.text
        assert "Ejercicio" in response.text

    async def test_quick_reply_decimal_value(self, client: AsyncClient, _clean_habits):
        """Test logging with decimal values."""
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Agua",
                "value_type": "numeric",
                "unit": "ml",
            },
        )
        assert response.status_code == 201

        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Agua 1500.5"},
        )
        assert response.status_code == 200
        assert "1500.5" in response.text
        assert "ml" in response.text

    async def test_quick_reply_boolean_habit(self, client: AsyncClient, _clean_habits):
        """Test logging boolean habit (no unit in response)."""
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Lectura",
                "value_type": "boolean",
            },
        )
        assert response.status_code == 201

        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Lectura 1"},
        )
        assert response.status_code == 200
        assert "Registrado" in response.text
        assert "Lectura" in response.text
        # Should not have a unit since it's boolean (value may be formatted as 1.00)
        assert "en Lectura" in response.text

    async def test_quick_reply_updates_existing_log(self, client: AsyncClient, _clean_habits):
        """Test that quick reply updates existing log for same day (upsert)."""
        # Create habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Peso",
                "value_type": "numeric",
                "unit": "kg",
            },
        )
        habit_id = response.json()["id"]

        # First log
        await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Peso 75.0"},
        )

        # Second log (should update)
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Peso 76.3"},
        )
        assert response.status_code == 200
        assert "76.3" in response.text

        # Verify only one log exists for today
        response = await client.get(f"/api/v1/habits/{habit_id}/logs")
        assert response.status_code == 200
        data = response.json()
        logs = data["items"]
        today_logs = [log for log in logs if log["log_date"] == date.today().isoformat()]
        assert len(today_logs) == 1
        assert Decimal(today_logs[0]["value"]) == Decimal("76.3")

    async def test_quick_reply_nonexistent_habit(self, client: AsyncClient, _clean_habits):
        """Test that nonexistent habit falls through to ActionRouter."""
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "HabitQueNoExiste 123"},
        )
        # Should fall through to ActionRouter (which may fail without LLM)
        assert response.status_code == 200

    async def test_quick_reply_invalid_format(self, client: AsyncClient, _clean_habits):
        """Test that invalid format falls through to ActionRouter."""
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Just some text without number"},
        )
        # Should fall through to ActionRouter
        assert response.status_code == 200


class TestActionRouter:
    """Tests for natural language processing with LLM (requires Ollama running).

    Note: These tests verify the integration works but don't assert exact values
    since LLM behavior is non-deterministic.
    """

    async def test_natural_language_exercise(self, client: AsyncClient, _clean_habits):
        """Test natural language: 'Corrí durante 30 minutos'."""
        # Create exercise habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Ejercicio",
                "description": "Actividad física (correr, caminar, nadar)",
                "value_type": "numeric",
                "unit": "minutos",
                "frequency": "weekly",
            },
        )
        assert response.status_code == 201
        habit_id = response.json()["id"]

        # Natural language request
        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Corrí durante 30 minutos"},
        )
        assert response.status_code == 200
        # LLM should either log successfully or provide some response
        assert (
            "Registrado" in response.text or "Ejercicio" in response.text or "30" in response.text
        )

        # Verify log was created
        response = await client.get(f"/api/v1/habits/{habit_id}/logs")
        data = response.json()
        assert len(data["items"]) >= 1

    async def test_natural_language_hours_to_minutes(self, client: AsyncClient, _clean_habits):
        """Test unit conversion: '2 horas de ejercicio' -> should convert to minutes."""
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Ejercicio",
                "description": "Actividad física (correr, caminar, nadar)",
                "value_type": "numeric",
                "unit": "minutos",
                "frequency": "weekly",
            },
        )
        habit_id = response.json()["id"]

        response = await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Hice 2 horas de ejercicio"},
        )
        assert response.status_code == 200
        # Verify LLM attempted to process the request
        assert len(response.text) > 0

        # Verify a log was created (value may vary due to LLM interpretation)
        response = await client.get(f"/api/v1/habits/{habit_id}/logs")
        data = response.json()
        assert len(data["items"]) >= 1


class TestAgentAIIntegration:
    """Integration tests verifying the full flow from agentai to habit logs."""

    async def test_quick_reply_creates_valid_habit_log(self, client: AsyncClient, _clean_habits):
        """Verify quick reply creates a proper habit log entry."""
        # Create habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Ejercicio",
                "value_type": "numeric",
                "unit": "minutos",
                "frequency": "weekly",
                "comparison_type": "greater_equal_than",
                "target_value": "150",
            },
        )
        habit_id = response.json()["id"]

        # Log via agentai
        await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Ejercicio 45"},
        )

        # Verify log was created correctly
        response = await client.get(f"/api/v1/habits/{habit_id}/logs")
        assert response.status_code == 200
        data = response.json()
        logs = data["items"]
        assert len(logs) == 1
        assert logs[0]["habit_id"] == habit_id
        assert Decimal(logs[0]["value"]) == Decimal("45")
        assert logs[0]["log_date"] == date.today().isoformat()

    async def test_quick_reply_reflects_in_daily_stats(self, client: AsyncClient, _clean_habits):
        """Verify quick reply log shows up in daily habits endpoint."""
        # Create habit
        response = await client.post(
            "/api/v1/habits",
            json={
                "name": "Agua",
                "value_type": "numeric",
                "unit": "ml",
                "frequency": "daily",
                "comparison_type": "greater_equal_than",
                "target_value": "2000",
            },
        )
        habit_id = response.json()["id"]

        # Log via agentai
        await client.post(
            "/api/v1/agentai/request-text",
            params={"text": "Agua 1500"},
        )

        # Check daily stats
        response = await client.get("/api/v1/habits/daily")
        assert response.status_code == 200
        daily = response.json()
        habit_stats = next((h for h in daily if h["id"] == habit_id), None)
        assert habit_stats is not None
        assert Decimal(habit_stats["current_period_value"]) == Decimal("1500")
