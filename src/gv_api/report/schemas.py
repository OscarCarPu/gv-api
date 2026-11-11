from datetime import datetime
from typing import Any

from gv_api.core.encoding import ApiModel, Timestamp

Row = dict[str, Any]


class Related(ApiModel):
    table: str
    row: Row


class Item(ApiModel):
    table: str
    key: str
    op: str  # insert, update or delete, of home against base
    # The row in the dump both sides started from, at home now, and as restored from EC2.
    base: Row | None = None
    home: Row | None = None
    ec2: Row | None = None
    # Per foreign-key column, the row it points at.
    related: dict[str, Related] | None = None


class Report(ApiModel):
    at: Timestamp
    items: list[Item] = []
    skipped: list[str] | None = None  # tables without a key, not compared
    error: str | None = None

    @classmethod
    def failed(cls, at: datetime, err: Exception) -> Report:
        return cls(at=at, items=[], error=str(err))
