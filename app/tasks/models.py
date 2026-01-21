from datetime import datetime

from sqlalchemy import DateTime, ForeignKey, String
from sqlalchemy.orm import Mapped, mapped_column, relationship

from app.common.validations import NAME_MAX_LENGTH
from app.core.database import Base
from app.tasks.enums import TaskCategory, TaskStatus


class Project(Base):
    name: Mapped[str] = mapped_column(String(NAME_MAX_LENGTH))
    description: Mapped[str | None] = mapped_column(default=None)
    finished: Mapped[bool] = mapped_column(default=False)

    tasks: Mapped[list["Task"]] = relationship(back_populates="project")  # noqa: UP037

    def __str__(self) -> str:
        """Return LLM-friendly"""
        return (
            f"ID {self.id}: {self.name}\nDescription: {self.description}, finished: {self.finished}"
        )


class Task(Base):
    project_id: Mapped[int] = mapped_column(ForeignKey("project.id"), index=True)
    title: Mapped[str] = mapped_column(String(NAME_MAX_LENGTH))
    status: Mapped[TaskStatus] = mapped_column(default=TaskStatus.pending)
    category: Mapped[TaskCategory] = mapped_column(default=TaskCategory.inbox)

    schedules: Mapped[list["TaskSchedule"]] = relationship(back_populates="task")  # noqa: UP037

    project: Mapped["Project"] = relationship(back_populates="tasks")  # noqa: UP037

    def __str__(self) -> str:
        """Return LLM-friendly"""
        return f"ID {self.id} in {str(self.project)}: {self.title}, status: {self.status}"


class TaskSchedule(Base):
    task_id: Mapped[int] = mapped_column(ForeignKey("task.id"), index=True)
    datetime_start: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    datetime_end: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), default=None)
    done: Mapped[bool] = mapped_column(default=False)

    task: Mapped["Task"] = relationship(back_populates="schedules")  # noqa: UP037

    def __str__(self) -> str:
        """Return LLM-friendly"""
        return f"ID {self.id}: {self.datetime_start} - {self.datetime_end}, done: {self.done}"
