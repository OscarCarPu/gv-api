from sqlalchemy import (
    ARRAY,
    TIMESTAMP,
    Boolean,
    CheckConstraint,
    Column,
    Date,
    ForeignKey,
    Integer,
    PrimaryKeyConstraint,
    SmallInteger,
    Table,
    Text,
    Time,
    func,
    true,
)

from gv_api.db.metadata import metadata

recurring_commitments = Table(
    "recurring_commitments",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("task_id", Integer, ForeignKey("tasks.id", ondelete="CASCADE"), nullable=False),
    Column("label", Text, nullable=False),
    Column("days_of_week", ARRAY(SmallInteger), nullable=False),
    Column("start_time", Time, nullable=False),
    Column("end_time", Time, nullable=False),
    Column("active", Boolean, nullable=False, server_default=true()),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("updated_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    CheckConstraint("end_time > start_time"),
    CheckConstraint("length(label) >= 1 AND length(label) <= 200"),
)

recurring_commitment_skips = Table(
    "recurring_commitment_skips",
    metadata,
    Column(
        "commitment_id",
        Integer,
        ForeignKey("recurring_commitments.id", ondelete="CASCADE"),
        nullable=False,
    ),
    Column("skip_date", Date, nullable=False),
    PrimaryKeyConstraint("commitment_id", "skip_date"),
)

plan_blocks = Table(
    "plan_blocks",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("plan_date", Date, nullable=False),
    Column("started_at", TIMESTAMP(timezone=True), nullable=False),
    Column("ended_at", TIMESTAMP(timezone=True), nullable=False),
    Column("task_id", Integer, ForeignKey("tasks.id", ondelete="SET NULL")),
    Column("label", Text, nullable=False),
    Column("note", Text),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("updated_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("event_ref", Text),
    Column(
        "commitment_id",
        Integer,
        ForeignKey("recurring_commitments.id", ondelete="SET NULL"),
    ),
    CheckConstraint("ended_at > started_at"),
    CheckConstraint("length(label) >= 1 AND length(label) <= 200"),
)
