import asyncio
import importlib
import os
import pkgutil

from alembic import context
from sqlalchemy.engine import Connection

import gv_api
from gv_api.db.engine import create_engine
from gv_api.db.metadata import metadata

# Every domain's tables.py registers its tables on the shared metadata.
for module in pkgutil.iter_modules(gv_api.__path__):
    if module.ispkg:
        try:
            importlib.import_module(f"gv_api.{module.name}.tables")
        except ModuleNotFoundError as err:
            if err.name != f"gv_api.{module.name}.tables":
                raise


def run(connection: Connection) -> None:
    context.configure(connection=connection, target_metadata=metadata)
    with context.begin_transaction():
        context.run_migrations()


async def run_with_own_engine() -> None:
    engine = create_engine(os.environ["DATABASE_URL"], pool_size=1)
    try:
        async with engine.begin() as conn:
            await conn.run_sync(run)
    finally:
        await engine.dispose()


if context.is_offline_mode():
    raise SystemExit("offline mode is not supported")

connection = context.config.attributes.get("connection")
if connection is not None:
    run(connection)
else:
    asyncio.run(run_with_own_engine())
