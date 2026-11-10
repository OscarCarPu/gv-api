from dataclasses import asdict
from datetime import date, datetime, time
from decimal import Decimal
from typing import Any

from sqlalchemy import Select, delete, exists, func, select, text, union, update
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncConnection, AsyncEngine

from gv_api.plan.errors import NotFoundError, TaskNotFoundError
from gv_api.plan.schemas import NewBlock, PlanBlockResponse, RecurringCommitmentResponse
from gv_api.plan.tables import plan_blocks, recurring_commitment_skips, recurring_commitments
from gv_api.tasks.tables import tasks

pb = plan_blocks
rc = recurring_commitments

_BLOCK_COLUMNS = (
    "id",
    "plan_date",
    "started_at",
    "ended_at",
    "task_id",
    "label",
    "note",
    "event_ref",
    "commitment_id",
)


def _with_task(source: Any) -> Select[*tuple[Any, ...]]:
    """The block columns of `source` (the table, or an INSERT/UPDATE ... RETURNING) with its
    task's fields."""
    return select(
        *(source.c[name] for name in _BLOCK_COLUMNS),
        tasks.c.name.label("task_name"),
        tasks.c.task_type,
        tasks.c.recurrence.label("task_recurrence"),
        tasks.c.started_at.label("task_started_at"),
        tasks.c.finished_at.label("task_finished_at"),
    ).select_from(source.outerjoin(tasks, tasks.c.id == source.c.task_id))


_BLOCKS = _with_task(pb)
_RETURNING = tuple(pb.c[name] for name in _BLOCK_COLUMNS)

_COMMITMENTS = (
    select(
        rc.c.id,
        rc.c.task_id,
        tasks.c.name.label("task_name"),
        rc.c.label,
        rc.c.days_of_week,
        func.to_char(rc.c.start_time, "HH24:MI").label("start_time"),
        func.to_char(rc.c.end_time, "HH24:MI").label("end_time"),
        rc.c.active,
    )
    .select_from(rc.join(tasks, tasks.c.id == rc.c.task_id))
    .order_by(rc.c.id)
)

# Busy hours per local day, splitting multi-day blocks. Blocks with a task or an event count;
# label-only blocks do not. `last` is inclusive.
_BUSY_HOURS = text("""
WITH tz_blocks AS (
    SELECT started_at AT TIME ZONE CAST(:tz AS text) AS local_start,
           ended_at AT TIME ZONE CAST(:tz AS text) AS local_end
    FROM plan_blocks
    WHERE task_id IS NOT NULL OR event_ref IS NOT NULL
),
days AS (
    SELECT generate_series(CAST(:first AS date), CAST(:last AS date), interval '1 day') AS day
)
SELECT days.day::date AS day,
       SUM(GREATEST(0, EXTRACT(EPOCH FROM (
           LEAST(tz.local_end, days.day + interval '1 day') - GREATEST(tz.local_start, days.day)
       )) / 3600.0))::numeric AS hours
FROM days
JOIN tz_blocks tz ON tz.local_start < days.day + interval '1 day' AND tz.local_end > days.day
GROUP BY days.day
ORDER BY days.day
""")

# A block already under way counts only from `since`.
_PLANNED_HOURS = text("""
SELECT task_id,
       SUM(EXTRACT(EPOCH FROM (ended_at - GREATEST(started_at, CAST(:since AS timestamptz))))
           / 3600.0)::numeric AS hours
FROM plan_blocks
WHERE task_id = ANY(CAST(:ids AS int[])) AND ended_at > CAST(:since AS timestamptz)
GROUP BY task_id
""")

# The commitment's generated blocks from `from_date` on follow its current days and times, as
# wall-clock times in `tz`: blocks on dropped weekdays go, the rest move.
_DROP_OFF_DAYS = text("""
DELETE FROM plan_blocks pb
USING recurring_commitments rc
WHERE rc.id = :id AND pb.commitment_id = rc.id AND pb.plan_date >= :from_date
  AND NOT (EXTRACT(DOW FROM pb.plan_date)::smallint = ANY(rc.days_of_week))
""")

_RESCHEDULE = text("""
UPDATE plan_blocks pb SET
    started_at = (pb.plan_date + rc.start_time) AT TIME ZONE CAST(:tz AS text),
    ended_at = (pb.plan_date + rc.end_time) AT TIME ZONE CAST(:tz AS text)
FROM recurring_commitments rc
WHERE rc.id = :id AND pb.commitment_id = rc.id AND pb.plan_date >= :from_date
""")


class PlanRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def _blocks(self, query: Select[*tuple[Any, ...]]) -> list[PlanBlockResponse]:
        async with self._engine.connect() as conn:
            rows = await conn.execute(query.order_by(pb.c.started_at))
        return [PlanBlockResponse.model_construct(**r._mapping) for r in rows]

    async def _block(self, conn: AsyncConnection, id: int) -> PlanBlockResponse:
        row = (await conn.execute(_BLOCKS.where(pb.c.id == id))).one_or_none()
        if row is None:
            raise NotFoundError
        return PlanBlockResponse.model_construct(**row._mapping)

    async def _commitment(self, conn: AsyncConnection, id: int) -> RecurringCommitmentResponse:
        row = (await conn.execute(_COMMITMENTS.where(rc.c.id == id))).one()
        return RecurringCommitmentResponse.model_construct(**row._mapping)

    # Blocks

    async def list_by_date(self, day: date) -> list[PlanBlockResponse]:
        return await self._blocks(_BLOCKS.where(pb.c.plan_date == day))

    async def list_by_range(self, start: datetime, end: datetime) -> list[PlanBlockResponse]:
        """Every block touching [start, end), so multi-day blocks show on each day."""
        return await self._blocks(_BLOCKS.where(pb.c.started_at < end, pb.c.ended_at > start))

    async def get(self, id: int) -> PlanBlockResponse:
        async with self._engine.connect() as conn:
            return await self._block(conn, id)

    async def get_by_event_ref(self, event_ref: str) -> PlanBlockResponse:
        async with self._engine.connect() as conn:
            id = (
                await conn.execute(select(pb.c.id).where(pb.c.event_ref == event_ref))
            ).scalar_one_or_none()
            if id is None:
                raise NotFoundError
            return await self._block(conn, id)

    async def get_task_name(self, task_id: int) -> str:
        async with self._engine.connect() as conn:
            name = (
                await conn.execute(select(tasks.c.name).where(tasks.c.id == task_id))
            ).scalar_one_or_none()
        if name is None:
            raise TaskNotFoundError
        return str(name)

    async def has_overlap(self, start: datetime, end: datetime, exclude_id: int | None) -> bool:
        """Plain interval overlap, whatever the plan date, so multi-day blocks conflict on each
        day."""
        cond = [pb.c.started_at < end, pb.c.ended_at > start]
        if exclude_id is not None:
            cond.append(pb.c.id != exclude_id)
        async with self._engine.connect() as conn:
            return bool((await conn.execute(select(exists().where(*cond)))).scalar())

    async def _write(self, stmt: Any) -> PlanBlockResponse:
        """One statement that writes a block and returns it joined with its task."""
        query = _with_task(stmt.returning(*_RETURNING).cte("written"))
        async with self._engine.connect() as conn:
            row = (await conn.execute(query)).one_or_none()
        if row is None:
            raise NotFoundError
        return PlanBlockResponse.model_construct(**row._mapping)

    async def create(self, block: NewBlock) -> PlanBlockResponse:
        return await self._write(pb.insert().values(asdict(block)))

    async def create_generated(self, block: NewBlock) -> None:
        """Does nothing when a concurrent call already generated the occurrence."""
        stmt = (
            insert(pb)
            .values(asdict(block))
            .on_conflict_do_nothing(
                index_elements=["commitment_id", "plan_date"],
                index_where=pb.c.commitment_id.is_not(None),
            )
        )
        async with self._engine.begin() as conn:
            await conn.execute(stmt)

    async def update(self, id: int, values: dict[str, Any]) -> PlanBlockResponse:
        return await self._write(update(pb).where(pb.c.id == id).values(values or {"id": pb.c.id}))

    async def update_times(self, id: int, start: datetime, end: datetime) -> None:
        values = {"started_at": start, "ended_at": end, "plan_date": start.date()}
        async with self._engine.begin() as conn:
            await conn.execute(update(pb).where(pb.c.id == id).values(values))

    async def clear_event_ref(self, id: int) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(update(pb).where(pb.c.id == id).values(event_ref=None))

    async def delete(self, id: int) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(delete(pb).where(pb.c.id == id))

    async def delete_ending_after(self, t: datetime) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(delete(pb).where(pb.c.ended_at >= t))

    async def sum_busy_hours_by_date(self, first: date, last: date, tz: str) -> dict[date, Decimal]:
        async with self._engine.connect() as conn:
            rows = await conn.execute(_BUSY_HOURS, {"tz": tz, "first": first, "last": last})
        return {r.day: r.hours for r in rows}

    async def sum_planned_hours_by_task(
        self, task_ids: list[int], since: datetime
    ) -> dict[int, Decimal]:
        async with self._engine.connect() as conn:
            rows = await conn.execute(_PLANNED_HOURS, {"ids": task_ids, "since": since})
        return {r.task_id: r.hours for r in rows}

    # Commitments

    async def _commitments(
        self, query: Select[*tuple[Any, ...]]
    ) -> list[RecurringCommitmentResponse]:
        async with self._engine.connect() as conn:
            rows = await conn.execute(query)
        return [RecurringCommitmentResponse.model_construct(**r._mapping) for r in rows]

    async def list_commitments(self) -> list[RecurringCommitmentResponse]:
        return await self._commitments(_COMMITMENTS)

    async def list_active_commitments(self) -> list[RecurringCommitmentResponse]:
        return await self._commitments(_COMMITMENTS.where(rc.c.active))

    async def create_commitment(
        self, task_id: int, label: str, days_of_week: list[int], start: time, end: time
    ) -> RecurringCommitmentResponse:
        values = {
            "task_id": task_id,
            "label": label,
            "days_of_week": days_of_week,
            "start_time": start,
            "end_time": end,
        }
        async with self._engine.begin() as conn:
            id = (await conn.execute(rc.insert().values(values).returning(rc.c.id))).scalar_one()
            return await self._commitment(conn, id)

    async def update_commitment(
        self, id: int, values: dict[str, Any]
    ) -> RecurringCommitmentResponse:
        async with self._engine.begin() as conn:
            stmt = update(rc).where(rc.c.id == id).values(values or {"id": rc.c.id})
            if (await conn.execute(stmt.returning(rc.c.id))).one_or_none() is None:
                raise NotFoundError
            return await self._commitment(conn, id)

    async def delete_commitment(self, id: int) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(delete(rc).where(rc.c.id == id))

    async def list_taken_dates(self, first: date, end: date) -> set[tuple[int, date]]:
        """(commitment id, date) in [first, end) that already has a block or was skipped."""
        sk = recurring_commitment_skips
        blocks = select(pb.c.commitment_id, pb.c.plan_date).where(
            pb.c.commitment_id.is_not(None), pb.c.plan_date >= first, pb.c.plan_date < end
        )
        skips = select(sk.c.commitment_id, sk.c.skip_date).where(
            sk.c.skip_date >= first, sk.c.skip_date < end
        )
        async with self._engine.connect() as conn:
            return {(r[0], r[1]) for r in await conn.execute(union(blocks, skips))}

    async def insert_skip(self, commitment_id: int, day: date) -> None:
        stmt = (
            insert(recurring_commitment_skips)
            .values(commitment_id=commitment_id, skip_date=day)
            .on_conflict_do_nothing()
        )
        async with self._engine.begin() as conn:
            await conn.execute(stmt)

    async def apply_commitment_schedule(self, id: int, from_date: date, tz: str) -> None:
        params = {"id": id, "from_date": from_date, "tz": tz}
        async with self._engine.begin() as conn:
            await conn.execute(_DROP_OFF_DAYS, params)
            await conn.execute(_RESCHEDULE, params)

    async def delete_future_commitment_blocks(self, commitment_id: int, since: datetime) -> None:
        """The commitment's blocks that have not started yet."""
        stmt = delete(pb).where(pb.c.commitment_id == commitment_id, pb.c.started_at >= since)
        async with self._engine.begin() as conn:
            await conn.execute(stmt)
