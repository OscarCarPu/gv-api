from typing import Annotated

from fastapi import APIRouter, Depends, UploadFile
from starlette.responses import PlainTextResponse

from app.agentai.service import AgentAIService
from app.core.security import require_auth
from app.habits.dependencies import HabitLogServiceDep, HabitServiceDep

router = APIRouter(prefix="/agentai", tags=["agent"], dependencies=[Depends(require_auth)])


def get_agentai_service(
    habit_service: HabitServiceDep, log_service: HabitLogServiceDep
) -> AgentAIService:
    return AgentAIService(habit_service, log_service)


AgentAIServiceDep = Annotated[AgentAIService, Depends(get_agentai_service)]


@router.post(
    "/request",
    response_class=PlainTextResponse,
    summary="Request something in voice file",
)
async def request(voice_file: UploadFile, service: AgentAIServiceDep) -> PlainTextResponse:
    file_content = await voice_file.read()
    text = await service.transform_into_text(file_content)
    result = await service.text_into_action(text)
    return PlainTextResponse(result)


@router.post(
    "/request-text",
    response_class=PlainTextResponse,
    summary="Request something in text",
)
async def request_text(text: str, service: AgentAIServiceDep) -> PlainTextResponse:
    result = await service.text_into_action(text)
    return PlainTextResponse(result)
