from fastapi import APIRouter, status
from sqlalchemy import text

from app.core import DbConnectionDep, get_logger

router = APIRouter(tags=["health"])
logger = get_logger(__name__)


@router.get(
    "/",
    summary="Root",
    description="Basic endpoint to verify the API is running.",
    responses={status.HTTP_200_OK: {"description": "API is running"}},
)
async def root():
    logger.debug("Root endpoint hit")
    return {"status": "ok"}


@router.get(
    "/health",
    summary="Health check",
    description="Health check endpoint that verifies database connectivity.",
    responses={
        status.HTTP_200_OK: {"description": "Service is healthy"},
        status.HTTP_503_SERVICE_UNAVAILABLE: {"description": "Database connection failed"},
    },
)
async def health(conn: DbConnectionDep):
    logger.debug("Health check requested")
    await conn.execute(text("SELECT 1"))
    logger.debug("Database connection established")
    return {"status": "ok"}
