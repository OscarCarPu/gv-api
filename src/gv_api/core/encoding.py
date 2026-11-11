"""Wire encoding of API values: typed amounts, dates and times."""

from datetime import date, datetime
from decimal import Decimal
from typing import Annotated, Any

from pydantic import BaseModel, ConfigDict, PlainSerializer
from pydantic_core import to_json
from starlette.responses import Response


class ApiModel(BaseModel):
    model_config = ConfigDict(extra="ignore")


def real_number(x: float) -> float:
    """A SQL real read as a double (64.22000122070312) back to the digits a real holds (64.22)."""
    return float(f"{x:.7g}")


def midnight_utc(d: date) -> str:
    """A date column in a field the API types as a timestamp: midnight UTC."""
    return d.strftime("%Y-%m-%dT00:00:00Z")


# Amounts, instants and dates use Pydantic's compiled JSON encoding: a numeric is a string that
# keeps its scale ("12.50"), an instant is RFC 3339 with Z for UTC and six fractional digits when
# it has any, and a date is YYYY-MM-DD.
Money = Decimal
Timestamp = datetime
Day = date
Real = Annotated[float, PlainSerializer(real_number, return_type=float)]
DayTime = Annotated[date, PlainSerializer(midnight_utc, return_type=str)]


def respond(status: int, data: Any) -> Response:
    return Response(to_json(data), status_code=status, media_type="application/json")
