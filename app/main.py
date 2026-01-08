from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from secure import Secure
from starlette.requests import Request

from app.agentai import router as agentai_router
from app.core import dispose_engine, get_settings, init_engine, setup_logging
from app.core.auth import router as auth_router
from app.habits import router as habits_router
from app.health import router as health_router


@asynccontextmanager
async def lifespan(_: FastAPI):
    setup_logging()
    await init_engine()
    yield
    await dispose_engine()


app = FastAPI(lifespan=lifespan)

secure_headers = Secure()


@app.middleware("http")
async def add_security_headers(request: Request, call_next):
    response = await call_next(request)
    secure_headers.set_headers(response)
    return response


settings = get_settings()
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_origins_list,
    allow_credentials=True,
    allow_methods=["GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"],
    allow_headers=["Content-Type", "X-API-Key", "Authorization"],
)


app.include_router(health_router, prefix="/api/v1")
app.include_router(auth_router, prefix="/api/v1")
app.include_router(habits_router, prefix="/api/v1")
app.include_router(agentai_router, prefix="/api/v1")
