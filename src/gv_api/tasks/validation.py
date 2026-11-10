"""Request checks; each returns "" when valid, or the client-facing message."""

TASK_TYPES = ("standard", "continuous", "recurring")


def validate_name(name: str) -> str:
    if not name:
        return "name is required"
    return validate_optional_name(name)


def validate_optional_name(name: str | None) -> str:
    if name is not None and len(name) > 40:
        return "name must be at most 40 characters"
    return ""


def validate_task_type(task_type: str | None) -> str:
    if task_type is not None and task_type not in TASK_TYPES:
        return "task_type must be standard, continuous, or recurring"
    return ""


def validate_priority(priority: int | None) -> str:
    if priority is not None and not 1 <= priority <= 5:
        return "priority must be between 1 and 5"
    return ""


def validate_recurrence_value(recurrence: int | None) -> str:
    if recurrence is not None and recurrence <= 0:
        return "recurrence must be a positive number of days"
    return ""
