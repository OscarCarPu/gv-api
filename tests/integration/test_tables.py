"""Every domain's tables.py against the migrated schema: columns, types, nullability."""

import importlib
import pkgutil

from sqlalchemy import inspect
from sqlalchemy.dialects import postgresql
from sqlalchemy.engine import Connection
from sqlalchemy.ext.asyncio import AsyncEngine

import gv_api
from gv_api.db.metadata import metadata

DIALECT = postgresql.dialect()  # type: ignore[no-untyped-call]


def import_all_tables() -> None:
    for module in pkgutil.iter_modules(gv_api.__path__):
        if module.ispkg:
            try:
                importlib.import_module(f"gv_api.{module.name}.tables")
            except ModuleNotFoundError as err:
                if err.name != f"gv_api.{module.name}.tables":
                    raise


def reflected(conn: Connection) -> dict[str, dict[str, tuple[str, bool]]]:
    insp = inspect(conn)
    return {
        table: {
            c["name"]: (c["type"].compile(dialect=DIALECT), c["nullable"])
            for c in insp.get_columns(table)
        }
        for table in metadata.tables
        if insp.has_table(table)
    }


async def test_declared_tables_match_the_schema(engine: AsyncEngine) -> None:
    import_all_tables()
    declared = {
        name: {c.name: (c.type.compile(dialect=DIALECT), c.nullable) for c in table.columns}
        for name, table in metadata.tables.items()
    }
    assert declared, "no tables.py declares a table"
    async with engine.connect() as conn:
        actual = await conn.run_sync(reflected)
    assert declared == actual
