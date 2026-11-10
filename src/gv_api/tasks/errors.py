class TasksError(Exception):
    message = ""

    def __init__(self) -> None:
        super().__init__(self.message)


class NotFoundError(TasksError):
    message = "not found"


class ActiveTimeEntryExistsError(TasksError):
    message = "an active time entry already exists"


class CircularDependencyError(TasksError):
    message = "circular task dependency"


class ProjectCycleError(TasksError):
    message = "a project cannot be moved under itself or one of its sub-projects"


class ParentNotFoundError(TasksError):
    message = "parent project not found"
