from fastapi import APIRouter
from sqlalchemy import text

from app.dependencies import DbConnectionDep
from app.logging import get_logger

router = APIRouter(tags=["health"])
logger = get_logger(__name__)


@router.get("/")
async def root():
    logger.debug("Root endpoint hit")
    return {"status": "ok"}


@router.get("/health")
async def health(conn: DbConnectionDep):
    logger.debug("Health check requested")
    await conn.execute(text("SELECT 1"))
    logger.debug("Database connection established")
    return {"status": "ok"}
