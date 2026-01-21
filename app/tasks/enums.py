from enum import Enum


class TaskStatus(str, Enum):
    pending = "pending"
    in_progress = "in_progress"
    done = "done"


class TaskCategory(str, Enum):
    project = "project"
    chore = "chore"
    inbox = "inbox"
