from gv_api.core.encoding import ApiModel, Timestamp
from gv_api.core.http import Body, InTime


class Event(ApiModel):
    id: int
    name: str
    start_time: Timestamp
    end_time: Timestamp


class CreateEventRequest(Body):
    name: str = ""
    start_time: InTime | None = None
    end_time: InTime | None = None


class UpdateEventRequest(Body):
    """A patch: absent (or null) fields are left alone."""

    name: str | None = None
    start_time: InTime | None = None
    end_time: InTime | None = None
