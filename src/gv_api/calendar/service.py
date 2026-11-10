import logging
from datetime import datetime, timedelta
from typing import Any, Protocol

from gv_api.calendar.errors import InvalidError
from gv_api.calendar.schemas import CreateEventRequest, Event, UpdateEventRequest

log = logging.getLogger("gv_api")

MAX_SPAN = timedelta(days=2 * 366)


class Repository(Protocol):
    async def list(self, start: datetime, end: datetime) -> list[Event]: ...
    async def get(self, id: int) -> Event: ...
    async def create(self, name: str, start: datetime, end: datetime) -> Event: ...
    async def update(self, id: int, values: dict[str, Any]) -> Event: ...
    async def delete(self, id: int) -> None: ...


class PlanSync(Protocol):
    """Keeps the plan block linked to an event (event_ref = the event id) in step with it."""

    async def sync_event_time(self, event_ref: str, start: datetime, end: datetime) -> None: ...
    async def detach_event(self, event_ref: str) -> None: ...


class CalendarService:
    def __init__(self, repo: Repository, plan: PlanSync) -> None:
        self._repo = repo
        self._plan = plan

    async def list_events(self, start: datetime, end: datetime) -> list[Event]:
        if end <= start:
            raise InvalidError("to must be after from")
        if end - start > MAX_SPAN:
            raise InvalidError("range is longer than two years")
        return await self._repo.list(start, end)

    async def get_event(self, id: int) -> Event:
        return await self._repo.get(id)

    async def create_event(self, req: CreateEventRequest) -> Event:
        name = req.name.strip()
        if not name:
            raise InvalidError("name is required")
        start, end = req.start_time, req.end_time
        if start is None or end is None or end <= start:
            raise InvalidError("end_time must be after start_time")
        return await self._repo.create(name, start, end)

    async def update_event(self, id: int, req: UpdateEventRequest) -> Event:
        if req.name is None and req.start_time is None and req.end_time is None:
            raise InvalidError("nothing to update")
        current = await self._repo.get(id)
        values: dict[str, Any] = {}
        if req.name is not None:
            if not (name := req.name.strip()):
                raise InvalidError("name cannot be empty")
            values["name"] = name
        start = req.start_time or current.start_time
        end = req.end_time or current.end_time
        if end <= start:
            raise InvalidError("end_time must be after start_time")
        if req.start_time is not None:
            values["start_time"] = start
        if req.end_time is not None:
            values["end_time"] = end

        updated = await self._repo.update(id, values)
        if req.start_time is not None or req.end_time is not None:
            try:
                await self._plan.sync_event_time(str(id), updated.start_time, updated.end_time)
            except Exception as err:
                log.error(
                    "calendar: syncing linked plan block", extra={"event": id, "error": str(err)}
                )
        return updated

    async def delete_event(self, id: int) -> None:
        await self._repo.delete(id)
        try:
            await self._plan.detach_event(str(id))
        except Exception as err:
            log.error("calendar: detaching plan block", extra={"event": id, "error": str(err)})
