from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.database import dispose_engine, init_engine
from app.logging import setup_logging
from app.routers import router


@asynccontextmanager
async def lifespan(_: FastAPI):
    setup_logging()
    await init_engine()
    yield
    await dispose_engine()


app = FastAPI(lifespan=lifespan)
app.include_router(router)
