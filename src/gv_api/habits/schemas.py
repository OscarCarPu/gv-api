from gv_api.core.encoding import ApiModel, Real
from gv_api.core.http import Body, Int32


class Habit(ApiModel):
    id: int
    name: str
    description: str | None = None
    frequency: str
    target_min: Real | None = None
    target_max: Real | None = None
    recording_required: bool
    current_streak: int
    longest_streak: int


class HabitWithLog(ApiModel):
    id: int
    name: str
    description: str | None = None
    frequency: str
    target_min: Real | None = None
    target_max: Real | None = None
    recording_required: bool
    log_value: Real | None = None
    period_value: Real
    current_streak: int
    longest_streak: int


class LogUpsertRequest(Body):
    habit_id: Int32 = 0
    date: str = ""
    value: float = 0


class CreateHabitRequest(Body):
    name: str = ""
    description: str | None = None
    frequency: str | None = None
    target_min: float | None = None
    target_max: float | None = None
    recording_required: bool | None = None


class UpdateHabitRequest(Body):
    name: str = ""
    description: str | None = None
    frequency: str = ""
    target_min: float | None = None
    target_max: float | None = None
    recording_required: bool = False
