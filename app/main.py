from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.core import dispose_engine, init_engine, setup_logging
from app.health import router as health_router


@asynccontextmanager
async def lifespan(_: FastAPI):
    setup_logging()
    await init_engine()
    yield
    await dispose_engine()


app = FastAPI(lifespan=lifespan)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["GET"],
    allow_headers=["*"],
)

app.include_router(health_router)
