import re
from datetime import date, datetime
from decimal import Decimal, InvalidOperation
from pathlib import Path

from pydantic_ai import Agent
from pydantic_ai.settings import ModelSettings

from app.core import TZ
from app.core.config import get_settings
from app.habits.models import HabitLog
from app.habits.service import HabitLogService, HabitService

PROMPT_FILE = Path(__file__).parent / "prompt.txt"


class Actions:
    def __init__(self, habit_service: HabitService, habit_log_service: HabitLogService):
        self.habit_service = habit_service
        self.habit_log_service = habit_log_service

    async def insert_habit_log(self, habit_id: int, log_date: date, value: Decimal) -> HabitLog:
        result: HabitLog = await self.habit_log_service.upsert(habit_id, log_date, value)
        return result


class ActionRouter:
    def __init__(self, habit_service: HabitService, habit_log_service: HabitLogService):
        self.habit_service = habit_service
        self.actions = Actions(habit_service, habit_log_service)

    async def _get_habits_context(self) -> str:
        """Get a formatted list of available habits for the agent context."""
        habits = await self.habit_service.habit_repo.get_all()
        if not habits:
            return "No hay hábitos disponibles."

        habits_list = "\n".join(f"- {h}" for h in habits)
        return f"Hábitos disponibles:\n{habits_list}"

    def _create_agent(self, habits_context: str) -> Agent:
        """Create an agent with the current habits context."""
        today = datetime.now(TZ).date()

        async def insert_habit_log(habit_id: int, value: float) -> str:
            """Log a habit entry. Convert units before calling (e.g., 2 hours -> 120 minutes).

            Args:
                habit_id: The habit ID from the list
                value: Numeric value in the habit's unit (after conversion)
            """
            try:
                habit = await self.habit_service.habit_repo.get(habit_id)
                habit_name = habit.name if habit else f"ID {habit_id}"
                habit_unit = habit.unit if habit and habit.unit else ""
                result = await self.actions.insert_habit_log(habit_id, today, Decimal(str(value)))
                unit_str = f" {habit_unit}" if habit_unit else ""
                return f"Registrado {result.value}{unit_str} en {habit_name}"
            except ValueError as e:
                return f"Error: {e}"
            except Exception as e:
                return f"Error al registrar hábito: {e}"

        settings = get_settings()
        agent = Agent(
            model=settings.agent_model,
            output_type=insert_habit_log,
            model_settings=ModelSettings(
                temperature=settings.agent_temperature,
            ),
            system_prompt=PROMPT_FILE.read_text().format(habits_context=habits_context),
        )

        return agent

    async def handle(self, request: str) -> str:
        habits_context = await self._get_habits_context()
        agent = self._create_agent(habits_context)
        result = await agent.run(request)
        return result.output or "No se realizó ninguna acción"


class QuickReplyRouter:
    def __init__(self, habit_service: HabitService, habit_log_service: HabitLogService):
        self.habit_service = habit_service
        self.actions = Actions(habit_service, habit_log_service)

    async def handle(self, request: str) -> str | None:
        """Handle quick reply format: 'habit_id value' or 'habit_name value'."""
        parts = request.strip().split(maxsplit=1)
        if len(parts) != 2:
            return None

        identifier, value_str = parts

        if not re.match(r"^\d+(\.\d+)?$", value_str):
            return None

        try:
            value = Decimal(value_str)
        except InvalidOperation:
            return None

        if identifier.isdigit():
            habit = await self.habit_service.habit_repo.get(int(identifier))
        else:
            habit = await self.habit_service.habit_repo.get_by_name_icase(identifier)

        if not habit:
            return None

        habit_name = habit.name
        habit_unit = habit.unit or ""

        today = datetime.now(TZ).date()
        result = await self.actions.insert_habit_log(habit.id, today, value)
        unit_str = f" {habit_unit}" if habit_unit else ""
        return f"Registrado {result.value}{unit_str} en {habit_name}"
