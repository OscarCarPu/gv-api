"""Wire encoding of API values (design D7): typed amounts, dates and times."""

from datetime import UTC, date, datetime
from decimal import Decimal
from typing import Annotated, Any
from zoneinfo import ZoneInfo

from pydantic import BaseModel, ConfigDict, PlainSerializer
from starlette.responses import JSONResponse, Response


class ApiModel(BaseModel):
    model_config = ConfigDict(extra="ignore")


def decimal_str(d: Decimal) -> str:
    """Plain notation, trailing fractional zeros trimmed: 12.50 -> "12.5", 1E+2 -> "100"."""
    s = format(d, "f")
    if "." in s:
        s = s.rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


def _is_utc(t: datetime) -> bool:
    tz = t.tzinfo
    return tz is None or tz is UTC or (isinstance(tz, ZoneInfo) and tz.key in ("UTC", "Etc/UTC"))


def time_str(t: datetime) -> str:
    """RFC 3339: fraction only when non-zero, trailing zeros trimmed, Z for UTC."""
    base = t.strftime("%Y-%m-%dT%H:%M:%S")
    if t.microsecond:
        base += "." + f"{t.microsecond:06d}".rstrip("0")
    if _is_utc(t):
        return base + "Z"
    offset = t.strftime("%z")
    return f"{base}{offset[:3]}:{offset[3:5]}"


def date_str(d: date) -> str:
    return d.strftime("%Y-%m-%d")


def real_number(x: float) -> float:
    """A SQL real read as a double (64.22000122070312) back to the digits a real holds (64.22)."""
    return float(f"{x:.7g}")


def midnight_utc(d: date) -> str:
    """A date column in a field the API types as a timestamp: midnight UTC."""
    return d.strftime("%Y-%m-%dT00:00:00Z")


Money = Annotated[Decimal, PlainSerializer(decimal_str, return_type=str)]
Real = Annotated[float, PlainSerializer(real_number, return_type=float)]
Timestamp = Annotated[datetime, PlainSerializer(time_str, return_type=str)]
Day = Annotated[date, PlainSerializer(date_str, return_type=str)]
DayTime = Annotated[date, PlainSerializer(midnight_utc, return_type=str)]


def jsonable(value: Any) -> Any:
    if isinstance(value, BaseModel):
        return value.model_dump(mode="json")
    if isinstance(value, list | tuple):
        return [jsonable(v) for v in value]
    if isinstance(value, dict):
        return {k: jsonable(v) for k, v in value.items()}
    if isinstance(value, Decimal):
        return decimal_str(value)
    if isinstance(value, datetime):
        return time_str(value)
    if isinstance(value, date):
        return date_str(value)
    return value


def respond(status: int, data: Any) -> Response:
    return JSONResponse(jsonable(data), status_code=status)
