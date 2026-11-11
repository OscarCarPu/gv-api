from dataclasses import dataclass
from datetime import datetime

from pydantic import ConfigDict, Field

from gv_api.core.encoding import ApiModel, Timestamp


@dataclass
class Range:
    kind: str  # up, down or failover
    start: datetime
    end: datetime


class RangeUptime(ApiModel):
    range: str
    uptime: float | None = None
    up_seconds: float
    down_seconds: float
    failover_seconds: float
    range_start: Timestamp
    range_end: Timestamp


class Overview(ApiModel):
    up_since: Timestamp
    ranges: list[RangeUptime] = []


class Window(ApiModel):
    kind: str
    start_time: Timestamp
    end_time: Timestamp
    seconds: float


class WindowsReport(ApiModel):
    model_config = ConfigDict(serialize_by_alias=True)

    from_: Timestamp = Field(serialization_alias="from")
    to: Timestamp
    uptime: float | None = None
    up_seconds: float
    down_seconds: float
    failover_seconds: float
    outages: int
    windows: list[Window] = []
    truncated: bool = False
