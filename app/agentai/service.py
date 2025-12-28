import logging

from app.agentai.actions import ActionRouter, QuickReplyRouter
from app.habits.service import HabitLogService, HabitService

logger = logging.getLogger(__name__)


class AgentAIService:
    def __init__(self, habit_service: HabitService, habit_log_service: HabitLogService):
        self.action_router = ActionRouter(habit_service, habit_log_service)
        self.quick_reply_router = QuickReplyRouter(habit_service, habit_log_service)

    async def transform_into_text(self, voice_file: bytes) -> str:
        return ""

    async def text_into_action(self, text: str) -> str:
        try:
            quick_result = await self.quick_reply_router.handle(text)
            if quick_result:
                return quick_result
            return await self.action_router.handle(text)
        except Exception as e:
            logger.exception("Error processing agent request")
            return f"Error processing request: {e}"
