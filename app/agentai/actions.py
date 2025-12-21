from datetime import date
from decimal import Decimal

from pydantic_ai import Agent, RunContext
from pydantic_ai.messages import ToolReturnPart

from app.core.config import get_settings
from app.habits.models import HabitLog
from app.habits.service import HabitLogService, HabitService


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
            return "No habits available."

        habits_list = [h.to_dict() for h in habits]
        return f"Available habits:\n{habits_list}"

    def _create_agent(self, habits_context: str) -> Agent:
        """Create an agent with the current habits context."""
        agent = Agent(
            model=get_settings().agent_model,
            system_prompt=f"""You are an action router.
Your job is to understand what the user wants
and call the appropriate action to fulfill their request.
Always use the available tools to complete the request.

{habits_context}

Use the habit ID when logging habits.""",
        )

        @agent.tool
        async def insert_habit_log(
            ctx: RunContext, habit_id: int, log_date: date, value: Decimal
        ) -> str:
            """Insert or update a habit log entry for a specific habit."""
            try:
                result = await self.actions.insert_habit_log(habit_id, log_date, value)
                return (
                    f"Successfully logged habit {result.habit_id} "
                    f"with value {result.value} for {result.log_date}"
                )
            except ValueError as e:
                return f"Error: {e}"
            except Exception as e:
                return f"Error logging habit: {e}"

        return agent

    async def run(self, request: str) -> str:
        habits_context = await self._get_habits_context()
        agent = self._create_agent(habits_context)
        result = await agent.run(request)

        # Return tool results if any action was taken
        for msg in result.all_messages():
            if hasattr(msg, "parts"):
                for part in msg.parts:
                    if isinstance(part, ToolReturnPart):
                        return str(part.content)

        return result.output or "No action taken"
