from datetime import UTC, date, datetime, time, timedelta
from decimal import Decimal
from typing import Any, Protocol
from zoneinfo import ZoneInfo

from gv_api.core.dates import Clock, utcnow
from gv_api.plan.errors import (
    DaysOfWeekRequiredError,
    InvalidTimeRangeError,
    LabelRequiredError,
    LabelTooLongError,
    NotFoundError,
    OverlapError,
)
from gv_api.plan.schemas import (
    CreateCommitmentRequest,
    CreatePlanBlockRequest,
    NewBlock,
    PlanBlockResponse,
    PlanRangeResponse,
    PlanTodayResponse,
    PlanTotals,
    RecurringCommitmentResponse,
    UpdateCommitmentRequest,
    UpdatePlanBlockRequest,
)
from gv_api.tasks.schemas import TimeEntrySummaryResponse

MAX_LABEL = 200


class Repository(Protocol):
    async def list_by_date(self, day: date) -> list[PlanBlockResponse]: ...
    async def list_by_range(self, start: datetime, end: datetime) -> list[PlanBlockResponse]: ...
    async def get(self, id: int) -> PlanBlockResponse: ...
    async def get_by_event_ref(self, event_ref: str) -> PlanBlockResponse: ...
    async def get_task_name(self, task_id: int) -> str: ...
    async def has_overlap(self, start: datetime, end: datetime, exclude_id: int | None) -> bool: ...
    async def create(self, block: NewBlock) -> PlanBlockResponse: ...
    async def create_generated(self, block: NewBlock) -> None: ...
    async def update(self, id: int, values: dict[str, Any]) -> PlanBlockResponse: ...
    async def update_times(self, id: int, start: datetime, end: datetime) -> None: ...
    async def clear_event_ref(self, id: int) -> None: ...
    async def delete(self, id: int) -> None: ...
    async def delete_ending_after(self, t: datetime) -> None: ...
    async def sum_busy_hours_by_date(
        self, first: date, last: date, tz: str
    ) -> dict[date, Decimal]: ...
    async def sum_planned_hours_by_task(
        self, task_ids: list[int], since: datetime
    ) -> dict[int, Decimal]: ...
    async def list_commitments(self) -> list[RecurringCommitmentResponse]: ...
    async def list_active_commitments(self) -> list[RecurringCommitmentResponse]: ...
    async def create_commitment(
        self, task_id: int, label: str, days_of_week: list[int], start: time, end: time
    ) -> RecurringCommitmentResponse: ...
    async def update_commitment(
        self, id: int, values: dict[str, Any]
    ) -> RecurringCommitmentResponse: ...
    async def delete_commitment(self, id: int) -> None: ...
    async def list_taken_dates(self, first: date, end: date) -> set[tuple[int, date]]: ...
    async def insert_skip(self, commitment_id: int, day: date) -> None: ...
    async def apply_commitment_schedule(self, id: int, from_date: date, tz: str) -> None: ...
    async def delete_future_commitment_blocks(
        self, commitment_id: int, since: datetime
    ) -> None: ...


class SummaryProvider(Protocol):
    async def get_time_entry_summary(self) -> TimeEntrySummaryResponse: ...


def parse_hhmm(raw: str) -> time:
    return datetime.strptime(raw, "%H:%M").time()


def weekday(d: date) -> int:
    """0 = Sunday, as days_of_week stores it."""
    return d.isoweekday() % 7


def check_label(label: str) -> str:
    trimmed = label.strip()
    if not trimmed:
        raise LabelRequiredError
    if len(trimmed) > MAX_LABEL:
        raise LabelTooLongError
    return trimmed


class PlanService:
    def __init__(
        self, repo: Repository, tasks: SummaryProvider, loc: ZoneInfo, now: Clock = utcnow
    ) -> None:
        self._repo = repo
        self._tasks = tasks
        self._loc = loc
        self._now = now

    def _local_now(self) -> datetime:
        return self._now().astimezone(self._loc)

    async def get_today(self) -> PlanTodayResponse:
        now = self._local_now()
        today = now.date()
        blocks = await self._repo.list_by_date(today)
        task_secs = free_secs = 0
        for b in blocks:
            if b.task_id is not None:
                task_secs += int((b.ended_at - b.started_at).total_seconds())
            elif b.ended_at > now:
                # Free time already gone can no longer be used: count only what is still ahead.
                free_secs += int((b.ended_at - max(b.started_at, now)).total_seconds())
        budget = await self._tasks.get_time_entry_summary()
        return PlanTodayResponse(
            date=today,
            blocks=blocks,
            totals=PlanTotals(task_seconds=task_secs, free_seconds=free_secs),
            budget=budget,
        )

    async def get_range(self, first: date, end: date) -> PlanRangeResponse:
        await self.ensure_recurring_blocks(first, end)
        blocks = await self._repo.list_by_range(
            datetime.combine(first, time(), UTC), datetime.combine(end, time(), UTC)
        )
        return PlanRangeResponse(from_=first, to=end, blocks=blocks)

    async def ensure_recurring_blocks(self, first: date, end: date) -> None:
        """A block in [first, end) for each active commitment on each of its weekdays, unless
        one exists or the date was skipped. Runs on every range read; a unique index makes
        concurrent calls safe."""
        commitments = await self._repo.list_active_commitments()
        if not commitments:
            return
        taken = await self._repo.list_taken_dates(first, end)
        for c in commitments:
            start, finish = parse_hhmm(c.start_time), parse_hhmm(c.end_time)
            day = first
            while day < end:
                if weekday(day) in c.days_of_week and (c.id, day) not in taken:
                    await self._repo.create_generated(
                        NewBlock(
                            plan_date=day,
                            started_at=datetime.combine(day, start, self._loc),
                            ended_at=datetime.combine(day, finish, self._loc),
                            task_id=c.task_id,
                            label=c.label,
                            commitment_id=c.id,
                        )
                    )
                day += timedelta(days=1)

    async def busy_hours_by_date(self, first: date, end: date) -> dict[date, Decimal]:
        await self.ensure_recurring_blocks(first, end)
        return await self._repo.sum_busy_hours_by_date(
            first, end - timedelta(days=1), self._loc.key
        )

    async def planned_hours_by_task(
        self, task_ids: list[int], since: datetime
    ) -> dict[int, Decimal]:
        """Hours each task still has ahead in the plan, so urgency does not count them twice."""
        if not task_ids:
            return {}
        return await self._repo.sum_planned_hours_by_task(task_ids, since)

    # Blocks

    async def create(self, req: CreatePlanBlockRequest) -> PlanBlockResponse:
        start, end = req.started_at, req.ended_at
        if start is None or end is None or end <= start:
            raise InvalidTimeRangeError
        label = await self._resolve_label(req.task_id, req.label)
        if await self._repo.has_overlap(start, end, None):
            raise OverlapError
        return await self._repo.create(
            NewBlock(
                plan_date=start.astimezone(self._loc).date(),
                started_at=start,
                ended_at=end,
                task_id=req.task_id,
                label=label,
                note=req.note,
                event_ref=req.event_ref,
            )
        )

    async def _resolve_label(self, task_id: int | None, label: str | None) -> str:
        """The caller's label, else the linked task's name."""
        if label is not None and label.strip():
            return check_label(label)
        if task_id is not None:
            return await self._repo.get_task_name(task_id)
        raise LabelRequiredError

    async def update(self, id: int, req: UpdatePlanBlockRequest) -> PlanBlockResponse:
        current: PlanBlockResponse | None = None
        start, end = req.started_at, req.ended_at
        if start is None and end is None:
            times = None
        elif start is not None and end is not None:
            times = (start, end)
        else:
            # One side only: check it against the stored other side.
            current = await self._repo.get(id)
            times = (start or current.started_at, end or current.ended_at)
        if times is not None and times[1] <= times[0]:
            raise InvalidTimeRangeError

        values: dict[str, Any] = {}
        if req.label is not None:
            values["label"] = check_label(req.label)

        if times is not None:
            if await self._repo.has_overlap(*times, id):
                raise OverlapError
            current = current or await self._repo.get(id)
            # Moved to another day: detach it from its commitment and skip the original date,
            # or the next generation pass recreates it there.
            if (
                current.commitment_id is not None
                and times[0].astimezone(self._loc).date() != current.plan_date
            ):
                values["commitment_id"] = None
                await self._repo.insert_skip(current.commitment_id, current.plan_date)

        if start is not None:
            values["started_at"] = start
            values["plan_date"] = start.astimezone(UTC).date()
        if end is not None:
            values["ended_at"] = end
        if req.clear_task:
            values["task_id"] = None
        elif req.task_id is not None:
            values["task_id"] = req.task_id
        if req.clear_note:
            values["note"] = None
        elif req.note is not None:
            values["note"] = req.note
        return await self._repo.update(id, values)

    async def delete(self, id: int) -> None:
        block = await self._repo.get(id)
        if block.commitment_id is not None:
            await self._repo.insert_skip(block.commitment_id, block.plan_date)
        await self._repo.delete(id)

    async def delete_future(self) -> None:
        await self._repo.delete_ending_after(self._now())

    async def sync_event_time(self, event_ref: str, start: datetime, end: datetime) -> None:
        """Keep the block linked to a calendar event in step with it, without Update's checks."""
        try:
            block = await self._repo.get_by_event_ref(event_ref)
        except NotFoundError:
            return
        await self._repo.update_times(block.id, start, end)

    async def detach_event(self, event_ref: str) -> None:
        """Unlink the block from an event that changed identity or was deleted; the block
        stays."""
        try:
            block = await self._repo.get_by_event_ref(event_ref)
        except NotFoundError:
            return
        await self._repo.clear_event_ref(block.id)

    # Commitments

    async def list_commitments(self) -> list[RecurringCommitmentResponse]:
        return await self._repo.list_commitments()

    async def create_commitment(self, req: CreateCommitmentRequest) -> RecurringCommitmentResponse:
        label = req.label.strip()
        if not label:
            raise LabelRequiredError
        if not req.days_of_week:
            raise DaysOfWeekRequiredError
        await self._repo.get_task_name(req.task_id)
        return await self._repo.create_commitment(
            req.task_id,
            label,
            req.days_of_week,
            parse_hhmm(req.start_time),
            parse_hhmm(req.end_time),
        )

    async def update_commitment(
        self, id: int, req: UpdateCommitmentRequest
    ) -> RecurringCommitmentResponse:
        values: dict[str, Any] = {}
        if req.label is not None:
            if not (label := req.label.strip()):
                raise LabelRequiredError
            values["label"] = label
        if req.days_of_week is not None:
            if not req.days_of_week:
                raise DaysOfWeekRequiredError
            values["days_of_week"] = req.days_of_week
        if req.start_time is not None:
            values["start_time"] = parse_hhmm(req.start_time)
        if req.end_time is not None:
            values["end_time"] = parse_hhmm(req.end_time)
        if req.active is not None:
            values["active"] = req.active
        updated = await self._repo.update_commitment(id, values)

        now = self._local_now()
        if req.active is not None and not updated.active:
            # Paused: unstarted blocks go. Resuming regenerates them; skips are kept.
            await self._repo.delete_future_commitment_blocks(updated.id, now)
        elif updated.active and {"days_of_week", "start_time", "end_time"} & values.keys():
            await self._repo.apply_commitment_schedule(updated.id, now.date(), self._loc.key)
        return updated

    async def delete_commitment(self, id: int) -> None:
        # The foreign key only nulls commitment_id, which would leave future occurrences
        # behind. Past ones stay.
        await self._repo.delete_future_commitment_blocks(id, self._now())
        await self._repo.delete_commitment(id)
