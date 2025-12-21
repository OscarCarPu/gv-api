from app.agentai.actions import ActionRouter
from app.habits.service import HabitLogService, HabitService


class AgentAIService:
    def __init__(self, habit_service: HabitService, habit_log_service: HabitLogService):
        self.action_router = ActionRouter(habit_service, habit_log_service)

    async def transform_into_text(self, voice_file: bytes) -> str:
        return ""

    async def text_into_action(self, text: str) -> str:
        return await self.action_router.run(text)
