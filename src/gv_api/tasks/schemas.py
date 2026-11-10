from datetime import date
from decimal import Decimal

from gv_api.core.encoding import ApiModel, DayTime, Money, Timestamp
from gv_api.core.http import Body, Int32, InTime

# Responses


class TaskDepRef(ApiModel):
    id: int
    name: str
    due_at: str | None = None


class TodoResponse(ApiModel):
    id: int
    task_id: int
    name: str
    is_done: bool = False


class ProjectResponse(ApiModel):
    id: int
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    parent_id: int | None = None
    started_at: Timestamp | None = None
    finished_at: Timestamp | None = None
    priority: int


class ProjectFastResponse(ApiModel):
    id: int
    name: str
    priority: int


class ProjectParentCandidate(ApiModel):
    id: int
    name: str
    path: str


class TaskFastResponse(ApiModel):
    id: int
    name: str
    project_id: int | None = None
    project_name: str | None = None
    task_type: str
    recurrence: int | None = None
    priority: int


class TaskResponse(ApiModel):
    id: int
    project_id: int | None = None
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    finished_at: Timestamp | None = None
    task_type: str
    recurrence: int | None = None
    priority: int
    estimate_hours: Money | None = None
    depends_on: list[TaskDepRef] = []
    blocks: list[TaskDepRef] = []
    blocked: bool = False


class TimeEntryResponse(ApiModel):
    id: int
    task_id: int
    started_at: Timestamp
    finished_at: Timestamp | None = None
    comment: str | None = None


class ActiveTimeEntryResponse(TimeEntryResponse):
    task_name: str
    task_description: str | None = None
    task_type: str
    recurrence: int | None = None
    priority: int
    project_name: str | None = None


class TimeEntryWithTaskResponse(ApiModel):
    id: int
    task_id: int
    task_name: str
    task_type: str
    recurrence: int | None = None
    priority: int
    project_id: int | None = None
    project_name: str | None = None
    started_at: Timestamp
    finished_at: Timestamp | None = None
    comment: str | None = None
    task_finished_at: Timestamp | None = None
    time_spent: int


class TaskDetailResponse(ApiModel):
    id: int
    project_id: int | None = None
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    finished_at: Timestamp | None = None
    task_type: str
    recurrence: int | None = None
    priority: int
    time_spent: int
    depends_on: list[TaskDepRef] = []
    blocks: list[TaskDepRef] = []
    blocked: bool = False


class TaskTimeEntriesResponse(ApiModel):
    task: TaskDetailResponse
    time_entries: list[TimeEntryResponse] = []


class TaskFullResponse(TaskDetailResponse):
    project_name: str | None = None
    estimate_hours: Money | None = None
    todos: list[TodoResponse] = []


class DepRemaining(ApiModel):
    id: int
    name: str
    remaining_hours: Money
    cumulative_hours: Money


class DueSoonSlackResponse(ApiModel):
    slack_minutes: int


class TaskByDueDateResponse(ApiModel):
    id: int
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    task_type: str
    recurrence: int | None = None
    priority: int
    # The own priority raised to the highest of anything the task transitively blocks.
    effective_priority: int = 0
    time_spent: int = 0
    estimate_hours: Money | None = None
    remaining_hours: Money | None = None
    start_by: date | None = None
    # The due date, or earlier when a blocked task must start sooner.
    finish_by: date | None = None
    # 1-based position in work order; only set when urgency was computed.
    work_order: int | None = None
    urgent: bool = False
    project_id: int | None = None
    project_name: str | None = None
    project_due_at: DayTime | None = None
    depends_on: list[TaskDepRef] = []
    dependent_hours: list[DepRemaining] = []
    blocks: list[TaskDepRef] = []
    blocked: bool = False


class PaceBreakdown(ApiModel):
    uniform_per_day_seconds: int = 0
    uniform_today_share_seconds: int = 0
    weighted_weekday_seconds: int = 0
    weighted_weekend_seconds: int = 0
    weighted_today_share_seconds: int = 0
    remaining_full_days: int = 0
    goal_reached: bool = False


class TimeEntrySummaryResponse(ApiModel):
    today: int
    week: int
    daily_target_seconds: int = 0
    weekly_target_seconds: int = 0
    pace: PaceBreakdown = PaceBreakdown()


class ActiveTreeNode(ApiModel):
    id: int
    type: str
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    task_type: str | None = None
    recurrence: int | None = None
    priority: int | None = None
    depends_on: list[TaskDepRef] | None = None
    blocks: list[TaskDepRef] | None = None
    blocked: bool = False
    children: list[ActiveTreeNode] | None = None


class ProjectDetailResponse(ApiModel):
    id: int
    parent_id: int | None = None
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    finished_at: Timestamp | None = None
    time_spent: int
    priority: int


class ProjectChildNode(ApiModel):
    id: int
    type: str
    name: str
    description: str | None = None
    due_at: DayTime | None = None
    started_at: Timestamp | None = None
    finished_at: Timestamp | None = None
    time_spent: int
    # projects only
    parent_id: int | None = None
    # tasks only
    project_id: int | None = None
    task_type: str | None = None
    recurrence: int | None = None
    priority: int | None = None
    depends_on: list[TaskDepRef] | None = None
    blocks: list[TaskDepRef] | None = None
    blocked: bool | None = None
    todos: list[TodoResponse] | None = None


class ProjectChildrenResponse(ApiModel):
    project: ProjectDetailResponse
    children: list[ProjectChildNode] = []


# Internal rows


class ActiveProject(ApiModel):
    id: int
    parent_id: int | None = None
    name: str
    due_at: date | None = None


class UnfinishedTask(ApiModel):
    id: int
    project_id: int | None = None
    name: str
    description: str | None = None
    due_at: date | None = None
    started_at: Timestamp | None = None
    task_type: str
    recurrence: int | None = None
    priority: int
    depends_on: list[TaskDepRef] = []
    blocks: list[TaskDepRef] = []
    blocked: bool = False


# Requests. Pointer-like fields (`str | None`) count as given when not None; the PATCH fields
# where null means "clear" are told apart from absent ones with `model_fields_set`.


class CreateProjectRequest(Body):
    name: str = ""
    description: str | None = None
    due_at: InTime | None = None
    parent_id: Int32 | None = None
    # The default for tasks created in this project without a priority of their own.
    priority: Int32 | None = None


class CreateTaskRequest(Body):
    project_id: Int32 | None = None
    name: str = ""
    description: str | None = None
    due_at: InTime | None = None
    depends_on: list[Int32] | None = None
    task_type: str | None = None
    recurrence: Int32 | None = None
    priority: Int32 | None = None
    estimate_hours: Decimal | None = None


class CreateTodoRequest(Body):
    task_id: Int32 = 0
    name: str = ""


class CreateTimeEntryRequest(Body):
    task_id: Int32 = 0
    started_at: InTime | None = None
    finished_at: InTime | None = None
    comment: str | None = None


class UpdateProjectRequest(Body):
    name: str | None = None
    description: str | None = None
    due_at: InTime | None = None  # null clears
    parent_id: Int32 | None = None  # null moves to the root
    started_at: InTime | None = None  # null clears
    finished_at: InTime | None = None  # null clears
    priority: Int32 | None = None


class UpdateTaskRequest(Body):
    name: str | None = None
    description: str | None = None
    due_at: InTime | None = None  # null clears
    project_id: Int32 | None = None
    started_at: InTime | None = None  # null clears
    finished_at: InTime | None = None  # null clears
    depends_on: list[Int32] | None = None
    blocks: list[Int32] | None = None
    task_type: str | None = None
    recurrence: Int32 | None = None
    priority: Int32 | None = None
    estimate_hours: Decimal | None = None  # null clears


class UpdateTodoRequest(Body):
    task_id: Int32 | None = None
    name: str | None = None
    is_done: bool | None = None


class UpdateTimeEntryRequest(Body):
    task_id: Int32 | None = None
    started_at: InTime | None = None
    finished_at: InTime | None = None  # null reopens the entry
    comment: str | None = None
