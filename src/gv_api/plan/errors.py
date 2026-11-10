class PlanError(Exception):
    message = ""

    def __init__(self) -> None:
        super().__init__(self.message)


class NotFoundError(PlanError):
    message = "not found"


class TaskNotFoundError(PlanError):
    message = "task not found"


class InvalidTimeRangeError(PlanError):
    message = "ended_at must be after started_at"


class LabelRequiredError(PlanError):
    message = "label or task_id is required"


class LabelTooLongError(PlanError):
    message = "label must be at most 200 characters"


class OverlapError(PlanError):
    message = "plan block overlaps with an existing one"


class DaysOfWeekRequiredError(PlanError):
    message = "days_of_week must not be empty"
