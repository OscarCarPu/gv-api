"""JSON encoding that matches the Go service's encoding/json output (design D7)."""

import json
import math
import struct
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


_F32_MAX = 3.4028234663852886e38
_F32_LIMIT = 2.0**128 - 2.0**103  # halfway past the largest float32: rounds up to inf


def as_float32(x: float) -> float:
    """x rounded to the nearest float32 (struct refuses values that round down to the max)."""
    try:
        return float(struct.unpack("f", struct.pack("f", x))[0])
    except OverflowError:
        return math.copysign(_F32_MAX if abs(x) < _F32_LIMIT else math.inf, x)


def go_float32(x: float) -> float | int:
    """A float32 as encoding/json writes it: the shortest decimal that reads back as the same
    float32 (64.22, not 64.22000122070312), and no ".0" on whole numbers."""
    x32 = as_float32(x)
    for digits in range(1, 10):
        short = float(f"{x32:.{digits}g}")
        if as_float32(short) == x32:
            break
    if short.is_integer() and abs(short) < 1e21:
        return int(short)
    return short


Money = Annotated[Decimal, PlainSerializer(go_decimal, return_type=str)]
Float32 = Annotated[float, PlainSerializer(go_float32, return_type=float | int)]
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
