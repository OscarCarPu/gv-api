from sqlalchemy import (
    REAL,
    TIMESTAMP,
    Boolean,
    Column,
    Date,
    ForeignKey,
    Identity,
    Integer,
    String,
    Table,
    Text,
    UniqueConstraint,
    func,
    text,
    true,
)

from gv_api.db.metadata import metadata

habits = Table(
    "habits",
    metadata,
    Column("id", Integer, Identity(always=True), primary_key=True),
    Column("name", String(40), nullable=False),
    Column("description", Text),
    Column("frequency", Text, nullable=False, server_default=text("'daily'")),
    Column("current_streak", Integer, nullable=False, server_default=text("0")),
    Column("longest_streak", Integer, nullable=False, server_default=text("0")),
    Column("target_min", REAL),
    Column("target_max", REAL),
    Column("recording_required", Boolean, nullable=False, server_default=true()),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
)

habit_logs = Table(
    "habit_logs",
    metadata,
    Column(
        "habit_id",
        Integer,
        ForeignKey("habits.id", name="fk_habit", ondelete="CASCADE"),
        nullable=False,
    ),
    Column("log_date", Date, nullable=False, server_default=func.current_date()),
    Column("value", REAL, nullable=False),
    UniqueConstraint("habit_id", "log_date", name="habit_logs_habit_id_log_date_key"),
)
