from dataclasses import dataclass
from datetime import date, datetime

from pydantic import ConfigDict, Field

from gv_api.core.encoding import ApiModel, Day, DayTime, Timestamp
from gv_api.core.http import Body, Int32, InTime
from gv_api.tasks.schemas import TimeEntrySummaryResponse

# Responses


class PlanBlockResponse(ApiModel):
    id: int
    plan_date: DayTime
    started_at: Timestamp
    ended_at: Timestamp
    task_id: int | None = None
    task_name: str | None = None
    label: str
    note: str | None = None
    event_ref: str | None = None
    commitment_id: int | None = None
    task_type: str | None = None
    task_recurrence: int | None = None
    task_started_at: Timestamp | None = None
    task_finished_at: Timestamp | None = None


class PlanRangeResponse(ApiModel):
    model_config = ConfigDict(serialize_by_alias=True)

    from_: Day = Field(serialization_alias="from")
    to: Day
    blocks: list[PlanBlockResponse] = []


class PlanTotals(ApiModel):
    task_seconds: int
    free_seconds: int


class PlanTodayResponse(ApiModel):
    date: Day
    blocks: list[PlanBlockResponse] = []
    totals: PlanTotals
    budget: TimeEntrySummaryResponse


class RecurringCommitmentResponse(ApiModel):
    id: int
    task_id: int
    task_name: str
    label: str
    days_of_week: list[int] = []
    start_time: str  # HH:MM
    end_time: str
    active: bool


@dataclass
class NewBlock:
    plan_date: date
    started_at: datetime
    ended_at: datetime
    label: str
    task_id: int | None = None
    note: str | None = None
    event_ref: str | None = None
    commitment_id: int | None = None


# Requests


class CreatePlanBlockRequest(Body):
    started_at: InTime | None = None
    ended_at: InTime | None = None
    task_id: Int32 | None = None
    label: str | None = None
    note: str | None = None
    event_ref: str | None = None


class UpdatePlanBlockRequest(Body):
    started_at: InTime | None = None
    ended_at: InTime | None = None
    task_id: Int32 | None = None
    clear_task: bool = False
    label: str | None = None
    note: str | None = None
    clear_note: bool = False


class CreateCommitmentRequest(Body):
    task_id: Int32 = 0
    label: str = ""
    days_of_week: list[Int32] = []
    start_time: str = ""
    end_time: str = ""


class UpdateCommitmentRequest(Body):
    label: str | None = None
    days_of_week: list[Int32] | None = None
    start_time: str | None = None
    end_time: str | None = None
    active: bool | None = None
