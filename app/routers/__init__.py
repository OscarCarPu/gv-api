from fastapi import APIRouter

from app.routers import health

router = APIRouter()
router.include_router(health.router)
