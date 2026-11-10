from sqlalchemy import (
    TIMESTAMP,
    Boolean,
    CheckConstraint,
    Column,
    Date,
    ForeignKey,
    Integer,
    Numeric,
    PrimaryKeyConstraint,
    String,
    Table,
    Text,
    false,
    func,
    text,
)

from gv_api.db.metadata import metadata

projects = Table(
    "projects",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("parent_id", Integer, ForeignKey("projects.id"), index=True),
    Column("name", String(40), nullable=False),
    Column("description", Text),
    Column("due_at", Date),
    Column("started_at", TIMESTAMP(timezone=True)),
    Column("finished_at", TIMESTAMP(timezone=True)),
    Column("priority", Integer, nullable=False, server_default=text("3")),
    CheckConstraint("priority >= 1 AND priority <= 5", name="chk_project_priority_range"),
)

tasks = Table(
    "tasks",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("project_id", Integer, ForeignKey("projects.id")),
    Column("name", String(40), nullable=False),
    Column("description", Text),
    Column("due_at", Date),
    Column("started_at", TIMESTAMP(timezone=True)),
    Column("finished_at", TIMESTAMP(timezone=True)),
    Column("task_type", Text, nullable=False, server_default=text("'standard'")),
    Column("recurrence", Integer),
    Column("priority", Integer, nullable=False, server_default=text("3")),
    Column("estimate_hours", Numeric(5, 2)),
)

todos = Table(
    "todos",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("task_id", Integer, ForeignKey("tasks.id", ondelete="CASCADE"), nullable=False),
    Column("name", String(40), nullable=False),
    Column("is_done", Boolean, nullable=False, server_default=false()),
)

time_entries = Table(
    "time_entries",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("task_id", Integer, ForeignKey("tasks.id", ondelete="CASCADE"), nullable=False),
    Column(
        "started_at",
        TIMESTAMP(timezone=True),
        nullable=False,
        server_default=func.current_timestamp(),
    ),
    Column("finished_at", TIMESTAMP(timezone=True)),
    Column("comment", Text),
)

task_dependencies = Table(
    "task_dependencies",
    metadata,
    Column("task_id", Integer, ForeignKey("tasks.id", ondelete="CASCADE"), nullable=False),
    Column("depends_on", Integer, ForeignKey("tasks.id", ondelete="CASCADE"), nullable=False),
    PrimaryKeyConstraint("task_id", "depends_on"),
)
