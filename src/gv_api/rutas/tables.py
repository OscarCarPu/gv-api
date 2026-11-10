from sqlalchemy import TIMESTAMP, Column, Date, Identity, Integer, Table, Text, func, text

from gv_api.db.metadata import metadata

concello_marks = Table(
    "concello_marks",
    metadata,
    Column("id", Integer, Identity(always=True), primary_key=True),
    Column("name", Text, nullable=False),
    Column("visited_on", Date, nullable=False),
    Column("description", Text, nullable=False, server_default=text("''")),
    Column("created_at", TIMESTAMP(timezone=True), nullable=False, server_default=func.now()),
)
