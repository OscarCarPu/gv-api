from gv_api.core.encoding import ApiModel, Float32
from gv_api.core.http import F32, Body, Int32


class Habit(ApiModel):
    id: int
    name: str
    description: str | None = None
    frequency: str
    target_min: Float32 | None = None
    target_max: Float32 | None = None
    recording_required: bool
    current_streak: int
    longest_streak: int


class HabitWithLog(ApiModel):
    id: int
    name: str
    description: str | None = None
    frequency: str
    target_min: Float32 | None = None
    target_max: Float32 | None = None
    recording_required: bool
    log_value: Float32 | None = None
    period_value: Float32
    current_streak: int
    longest_streak: int


class LogUpsertRequest(Body):
    habit_id: Int32 = 0
    date: str = ""
    value: F32 = 0


class CreateHabitRequest(Body):
    name: str = ""
    description: str | None = None
    frequency: str | None = None
    target_min: F32 | None = None
    target_max: F32 | None = None
    recording_required: bool | None = None


class UpdateHabitRequest(Body):
    name: str = ""
    description: str | None = None
    frequency: str = ""
    target_min: F32 | None = None
    target_max: F32 | None = None
    recording_required: bool = False
