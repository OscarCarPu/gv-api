"""Request parsing and error responses (design D8)."""

import json
import logging
import re
from functools import cache
from typing import Annotated, Any, get_args

from pydantic import AwareDatetime, BaseModel, Field, ValidationError, model_validator
from starlette.requests import Request
from starlette.responses import Response

from gv_api.core.encoding import ApiModel, respond

log = logging.getLogger("gv_api")

Int32 = Annotated[int, Field(ge=-(2**31), le=2**31 - 1)]
InTime = AwareDatetime

_ID = re.compile(r"[+-]?[0-9]+")


class ApiError(Exception):
    """Raised anywhere in a request; rendered as {"error": message[, "code": code]}."""

    def __init__(self, status: int, message: str, code: str | None = None) -> None:
        super().__init__(message)
        self.status = status
        self.message = message
        self.code = code


class Body(ApiModel):
    """A JSON request body: unknown keys are ignored, and a null leaves the default of a field
    that cannot hold None (a nullable field takes the null and counts as set)."""

    @model_validator(mode="before")
    @classmethod
    def _drop_nulls(cls, data: Any) -> Any:
        if isinstance(data, dict):
            nullable = _nullable_fields(cls)
            return {k: v for k, v in data.items() if v is not None or k in nullable}
        return data


@cache
def _nullable_fields(model: type[BaseModel]) -> frozenset[str]:
    return frozenset(
        name
        for name, field in model.model_fields.items()
        if type(None) in get_args(field.annotation)
    )


def error(status: int, message: str) -> Response:
    return respond(status, {"error": message})


def error_code(status: int, code: str, message: str) -> Response:
    return respond(status, {"error": message, "code": code})


def internal_error(err: BaseException, message: str) -> Response:
    log.error(message, extra={"error": str(err)})
    return error(500, message)


async def parse_body[M: Body](request: Request, model: type[M], message: str) -> M:
    raw = await request.body()
    try:
        data = json.loads(raw)
    except ValueError:
        raise ApiError(400, message) from None
    if data is None:
        data = {}
    if not isinstance(data, dict):
        raise ApiError(400, message)
    try:
        return model.model_validate(data)
    except ValidationError:
        raise ApiError(400, message) from None


def parse_id(raw: str, entity: str) -> int:
    """A positive id that fits in 32 bits (the SQL integer columns)."""
    if _ID.fullmatch(raw):
        value = int(raw)
        if 0 < value <= 2**31 - 1:
            return value
    raise ApiError(400, f"invalid {entity} id")


def query(request: Request, key: str) -> str:
    """The first value of a query parameter, or "" when absent."""
    values = request.query_params.getlist(key)
    return values[0] if values else ""
