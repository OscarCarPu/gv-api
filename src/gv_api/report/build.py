"""What home changed since the dump both sides started from that the data restored from EC2
does not hold. It only reads: the restore keeps EC2's data as it is."""

import json
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.report.schemas import Item, Related, Report, Row

# Bookkeeping, not data.
_NOT_DATA = ("schema_migrations", "alembic_version")

_TABLES = text("""
SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind = 'r'
""")

# A table's key is its primary key, or else its first unique constraint without a nullable
# column.
_KEYS = text("""
WITH keys AS (
  SELECT k.conrelid, k.conkey,
         row_number() OVER (PARTITION BY k.conrelid ORDER BY (k.contype = 'p') DESC, k.conname)
           AS rank
  FROM pg_constraint k
  WHERE k.contype IN ('p', 'u')
    AND NOT EXISTS (
      SELECT 1 FROM unnest(k.conkey) x(attnum)
      JOIN pg_attribute na ON na.attrelid = k.conrelid AND na.attnum = x.attnum
      WHERE NOT na.attnotnull)
)
SELECT c.relname, a.attname
FROM keys
JOIN pg_class c ON c.oid = keys.conrelid
CROSS JOIN LATERAL unnest(keys.conkey) WITH ORDINALITY AS u(attnum, ord)
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = u.attnum
WHERE keys.rank = 1
ORDER BY c.relname, u.ord
""")

_REFS = text("""
SELECT c.relname, a.attname, p.relname
FROM pg_constraint k
JOIN pg_class c ON c.oid = k.conrelid
JOIN pg_class p ON p.oid = k.confrelid
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.conkey[1]
WHERE k.contype = 'f' AND cardinality(k.conkey) = 1
""")


@dataclass
class Table:
    name: str
    key: list[str] = field(default_factory=list)
    refs: dict[str, str] = field(default_factory=dict)  # column -> table it points at


@dataclass
class Version:
    data: Row
    sig: str  # canonical jsonb text without the ignored columns


Snapshot = dict[str, dict[str, Version]]


async def introspect(engine: AsyncEngine) -> tuple[list[Table], list[str]]:
    """The public tables with a key, by name, and the names of those without one."""
    async with engine.connect() as conn:
        names = sorted(r[0] for r in await conn.execute(_TABLES) if r[0] not in _NOT_DATA)
        tables = {n: Table(n) for n in names}
        for table, col in await conn.execute(_KEYS):
            if table in tables:
                tables[table].key.append(col)
        for table, col, parent in await conn.execute(_REFS):
            if table in tables:
                tables[table].refs[col] = parent
    keyed = [tables[n] for n in names if tables[n].key]
    return keyed, [n for n in names if not tables[n].key]


def row_key(values: list[Any]) -> str:
    return json.dumps(values, ensure_ascii=False, separators=(",", ":"))


async def read(engine: AsyncEngine, table: Table, ignore: list[str]) -> dict[str, Version]:
    query = text(
        f"SELECT to_jsonb(t)::text, (to_jsonb(t) - CAST(:ignore AS text[]))::text"  # noqa: S608
        f' FROM public."{table.name}" t'
    )
    out = {}
    async with engine.connect() as conn:
        for full, sig in await conn.execute(query, {"ignore": ignore}):
            data = json.loads(full)
            key = row_key([data[c] for c in table.key])
            for col in ignore:
                data.pop(col, None)
            out[key] = Version(data, sig)
    return out


def _op(base: Version | None, home: Version | None) -> str:
    if base is None:
        return "insert"
    return "delete" if home is None else "update"


def _same(a: Version | None, b: Version | None) -> bool:
    if a is None or b is None:
        return a is b
    return a.sig == b.sig


def _data(v: Version | None) -> Row | None:
    return None if v is None else v.data


def _related(table: Table, item: Item, snaps: list[Snapshot]) -> dict[str, Related] | None:
    """The row each foreign-key column of the item points at, from the first side that has it."""
    out: dict[str, Related] = {}
    base, home, ec2 = snaps
    for col, parent in table.refs.items():
        for d in (item.home, item.ec2, item.base):
            if d is None or d.get(col) is None:
                continue
            key = row_key([d[col]])
            found = next(
                (s[parent][key] for s in (ec2, home, base) if key in s.get(parent, {})), None
            )
            if found is not None:
                out[col] = Related(table=parent, row=found.data)
                break
    return out or None


async def build(
    base: AsyncEngine,
    home: AsyncEngine,
    ec2: AsyncEngine,
    ignore: dict[str, list[str]] | None = None,
    skip: tuple[str, ...] = (),
) -> Report:
    """Compare home with base, and keep what differs from the row EC2 holds."""
    ignore = ignore or {}
    tables, keyless = await introspect(home)
    tables = [t for t in tables if t.name not in skip]
    keyless = [n for n in keyless if n not in skip]

    snaps: list[Snapshot] = []
    for engine in (base, home, ec2):
        snaps.append({t.name: await read(engine, t, ignore.get(t.name, [])) for t in tables})
    base_rows, home_rows, _ = snaps

    report = Report(at=datetime.now(UTC), items=[], skipped=keyless or None)
    for t in tables:
        for k in sorted(base_rows[t.name].keys() | home_rows[t.name].keys()):
            b, h, e = (rows[t.name].get(k) for rows in snaps)
            if _same(b, h) or _same(h, e):
                continue
            item = Item(
                table=t.name, key=k, op=_op(b, h), base=_data(b), home=_data(h), ec2=_data(e)
            )
            item.related = _related(t, item, snaps)
            report.items.append(item)
    return report
