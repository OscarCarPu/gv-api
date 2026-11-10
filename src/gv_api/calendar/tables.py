from sqlalchemy import TIMESTAMP, Column, Integer, Table, Text, func, text

from gv_api.db.metadata import metadata

calendar_events = Table(
    "calendar_events",
    metadata,
    Column("id", Integer, primary_key=True),
    Column("name", Text, nullable=False, server_default=text("''")),
    Column("start_time", TIMESTAMP(timezone=True), nullable=False),
    Column("end_time", TIMESTAMP(timezone=True), nullable=False),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
    Column("updated_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
)
