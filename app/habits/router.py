from datetime import date
from typing import Annotated

from fastapi import APIRouter, Depends, Query, status

from app.common.constants import DEFAULT_PAGE, DEFAULT_PAGE_SIZE, MAX_PAGE_SIZE
from app.core import SessionDep, verify_api_key

from .repository import HabitLogRepository, HabitRepository
from .schemas import (
    HabitCreate,
    HabitHistory,
    HabitLogBody,
    HabitLogCreate,
    HabitLogRead,
    HabitLogUpdate,
    HabitRead,
    HabitTodayStats,
    HabitUpdate,
    PaginatedResponse,
)
from .service import HabitLogService, HabitService

router = APIRouter(prefix="/habits", tags=["habits"], dependencies=[Depends(verify_api_key)])


# --- Repository Dependencies ---


def get_habit_repository(session: SessionDep) -> HabitRepository:
    return HabitRepository(session)


def get_log_repository(session: SessionDep) -> HabitLogRepository:
    return HabitLogRepository(session)


HabitRepoDep = Annotated[HabitRepository, Depends(get_habit_repository)]
LogRepoDep = Annotated[HabitLogRepository, Depends(get_log_repository)]


# --- Service Dependencies ---


def get_habit_service(
    habit_repo: HabitRepoDep,
    log_repo: LogRepoDep,
) -> HabitService:
    return HabitService(habit_repo, log_repo)


def get_log_service(
    habit_repo: HabitRepoDep,
    log_repo: LogRepoDep,
) -> HabitLogService:
    return HabitLogService(habit_repo, log_repo)


HabitServiceDep = Annotated[HabitService, Depends(get_habit_service)]
LogServiceDep = Annotated[HabitLogService, Depends(get_log_service)]


# --- Habits ---


@router.get(
    "",
    response_model=PaginatedResponse[HabitRead],
    summary="List habits",
    description="Retrieve a paginated list of all habits.",
    responses={status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"}},
)
async def list_habits(
    service: HabitServiceDep,
    page: Annotated[int, Query(ge=1)] = DEFAULT_PAGE,
    page_size: Annotated[int, Query(ge=1, le=MAX_PAGE_SIZE)] = DEFAULT_PAGE_SIZE,
):
    return await service.get_all(page=page, page_size=page_size)


@router.get(
    "/today",
    response_model=list[HabitTodayStats],
    summary="Get today's habits",
    description="Get all active habits for today with their statistics "
    "including streaks, averages, and current period value.",
    responses={status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"}},
)
async def get_today_habits(service: HabitServiceDep):
    return await service.get_today_habits()


@router.post(
    "",
    response_model=HabitRead,
    status_code=status.HTTP_201_CREATED,
    summary="Create habit",
    description="Create a new habit with the specified configuration.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_422_UNPROCESSABLE_CONTENT: {"description": "Validation error"},
    },
)
async def create_habit(data: HabitCreate, service: HabitServiceDep):
    return await service.create(data)


@router.get(
    "/{habit_id}",
    response_model=HabitRead,
    summary="Get habit",
    description="Retrieve a specific habit by its ID.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
    },
)
async def get_habit(habit_id: int, service: HabitServiceDep):
    return await service.get(habit_id)


@router.patch(
    "/{habit_id}",
    response_model=HabitRead,
    summary="Update habit",
    description="Update an existing habit's configuration.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
        status.HTTP_422_UNPROCESSABLE_CONTENT: {"description": "Validation error"},
    },
)
async def update_habit(habit_id: int, data: HabitUpdate, service: HabitServiceDep):
    return await service.update(habit_id, data)


@router.delete(
    "/{habit_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Delete habit",
    description="Delete a habit and all its associated logs.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
    },
)
async def delete_habit(habit_id: int, service: HabitServiceDep):
    await service.delete(habit_id)


@router.get(
    "/{habit_id}/history",
    response_model=HabitHistory,
    summary="Get habit history",
    description="Get aggregated log data for a habit over a date range. "
    "Aggregates by time period (day, week, or month).",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
    },
)
async def get_habit_history(
    habit_id: int,
    service: HabitServiceDep,
    start_date: date | None = None,
    end_date: date | None = None,
    time_period: str | None = None,
):
    return await service.get_habit_history(habit_id, start_date, end_date, time_period)


# --- Logs ---


@router.get(
    "/{habit_id}/logs",
    response_model=PaginatedResponse[HabitLogRead],
    summary="List habit logs",
    description="Retrieve a list of logs for a specific habit. Optionally filter by date range.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
    },
)
async def list_logs(
    habit_id: int,
    service: LogServiceDep,
    start_date: date | None = None,
    end_date: date | None = None,
    page: Annotated[int, Query(ge=1)] = DEFAULT_PAGE,
    page_size: Annotated[int, Query(ge=1, le=MAX_PAGE_SIZE)] = DEFAULT_PAGE_SIZE,
):
    return await service.list_by_habit(
        habit_id, start_date, end_date, page=page, page_size=page_size
    )


@router.post(
    "/{habit_id}/logs",
    response_model=HabitLogRead,
    status_code=status.HTTP_201_CREATED,
    summary="Create habit log",
    description="Create a new log entry for a habit on a specific date.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Habit not found"},
        status.HTTP_409_CONFLICT: {"description": "Log already exists for this date"},
        status.HTTP_422_UNPROCESSABLE_CONTENT: {"description": "Validation error"},
    },
)
async def create_log(habit_id: int, data: HabitLogBody, service: LogServiceDep):
    log_data = HabitLogCreate(habit_id=habit_id, **data.model_dump())
    return await service.create(log_data)


@router.patch(
    "/{habit_id}/logs/{log_id}",
    response_model=HabitLogRead,
    summary="Update habit log",
    description="Update an existing log entry's date or value.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Log not found"},
        status.HTTP_422_UNPROCESSABLE_CONTENT: {"description": "Validation error"},
    },
)
async def update_log(log_id: int, data: HabitLogUpdate, service: LogServiceDep):
    return await service.update(log_id, data)


@router.delete(
    "/{habit_id}/logs/{log_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Delete habit log",
    description="Delete a specific log entry.",
    responses={
        status.HTTP_401_UNAUTHORIZED: {"description": "Invalid or missing API key"},
        status.HTTP_404_NOT_FOUND: {"description": "Log not found"},
    },
)
async def delete_log(log_id: int, service: LogServiceDep):
    await service.delete(log_id)
