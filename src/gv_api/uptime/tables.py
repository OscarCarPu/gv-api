from sqlalchemy import TIMESTAMP, BigInteger, Column, Enum, Identity, Table

from gv_api.db.metadata import metadata

outage_kind = Enum("start", "up", "down", "failover", name="outage_kind", create_type=False)

# A timeline: each row ends a range of its kind that began where the previous row ended. The
# first row, kind start, is where tracking began.
outages = Table(
    "outages",
    metadata,
    Column("id", BigInteger, Identity(always=True), primary_key=True),
    Column("ends_at", TIMESTAMP(timezone=True), nullable=False, unique=True),
    Column("kind", outage_kind, nullable=False),
)
