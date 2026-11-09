"""JSON encoding that matches the Go service's encoding/json output (design D7)."""

import json
from datetime import UTC, date, datetime
from decimal import Decimal
from typing import Annotated, Any
from zoneinfo import ZoneInfo

from pydantic import BaseModel, ConfigDict, PlainSerializer
from starlette.responses import JSONResponse, Response


class ApiModel(BaseModel):
    model_config = ConfigDict(extra="ignore")


def go_decimal(d: Decimal) -> str:
    """shopspring/decimal: plain notation, trailing fractional zeros trimmed, quoted."""
    s = format(d, "f")
    if "." in s:
        s = s.rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


def _is_utc(t: datetime) -> bool:
    tz = t.tzinfo
    return tz is None or tz is UTC or (isinstance(tz, ZoneInfo) and tz.key in ("UTC", "Etc/UTC"))


def go_time(t: datetime) -> str:
    """time.RFC3339Nano: fraction only when non-zero, trailing zeros trimmed, Z only for UTC."""
    base = t.strftime("%Y-%m-%dT%H:%M:%S")
    if t.microsecond:
        base += "." + f"{t.microsecond:06d}".rstrip("0")
    if _is_utc(t):
        return base + "Z"
    offset = t.strftime("%z")
    return f"{base}{offset[:3]}:{offset[3:5]}"


def go_date(d: date) -> str:
    return d.strftime("%Y-%m-%d")


Money = Annotated[Decimal, PlainSerializer(go_decimal, return_type=str)]
Timestamp = Annotated[datetime, PlainSerializer(go_time, return_type=str)]
Day = Annotated[date, PlainSerializer(go_date, return_type=str)]


def jsonable(value: Any) -> Any:
    if isinstance(value, BaseModel):
        return value.model_dump(mode="json")
    if isinstance(value, list | tuple):
        return [jsonable(v) for v in value]
    if isinstance(value, dict):
        return {k: jsonable(v) for k, v in value.items()}
    if isinstance(value, Decimal):
        return go_decimal(value)
    if isinstance(value, datetime):
        return go_time(value)
    if isinstance(value, date):
        return go_date(value)
    return value


class GoJSONResponse(JSONResponse):
    def render(self, content: Any) -> bytes:
        return (json.dumps(content, ensure_ascii=False, separators=(",", ":")) + "\n").encode()


def respond(status: int, data: Any) -> Response:
    return GoJSONResponse(jsonable(data), status_code=status)
