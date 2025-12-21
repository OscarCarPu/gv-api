from typing import Annotated

from fastapi import Depends

from app.core.dependencies import SessionDep
from app.habits.repository import HabitLogRepository, HabitRepository
from app.habits.service import HabitLogService, HabitService


def get_habit_repository(session: SessionDep) -> HabitRepository:
    return HabitRepository(session)


def get_habit_log_repository(session: SessionDep) -> HabitLogRepository:
    return HabitLogRepository(session)


HabitRepoDep = Annotated[HabitRepository, Depends(get_habit_repository)]
HabitLogRepoDep = Annotated[HabitLogRepository, Depends(get_habit_log_repository)]


def get_habit_service(habit_repo: HabitRepoDep, log_repo: HabitLogRepoDep) -> HabitService:
    return HabitService(habit_repo, log_repo)


def get_habit_log_service(habit_repo: HabitRepoDep, log_repo: HabitLogRepoDep) -> HabitLogService:
    return HabitLogService(habit_repo, log_repo)


HabitServiceDep = Annotated[HabitService, Depends(get_habit_service)]
HabitLogServiceDep = Annotated[HabitLogService, Depends(get_habit_log_service)]
