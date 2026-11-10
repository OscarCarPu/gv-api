"""Request parsing and error responses with the Go service's messages (design D8)."""

import json
import logging
import re
from typing import Annotated, Any

from pydantic import AfterValidator, AwareDatetime, Field, ValidationError, model_validator
from starlette.requests import Request
from starlette.responses import Response

from gv_api.core.encoding import ApiModel, as_float32, respond

log = logging.getLogger("gv_api")

Int32 = Annotated[int, Field(ge=-(2**31), le=2**31 - 1)]
# encoding/json refuses a number outside float32's range for a float32 field.
F32 = Annotated[
    float, Field(ge=-3.4028234663852886e38, le=3.4028234663852886e38), AfterValidator(as_float32)
]
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
    """Request body decoded like Go's encoding/json: a JSON null leaves the zero value."""

    @model_validator(mode="before")
    @classmethod
    def _drop_nulls(cls, data: Any) -> Any:
        if isinstance(data, dict):
            return {k: v for k, v in data.items() if v is not None}
        return data


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
        data = json.JSONDecoder().raw_decode(raw.decode().lstrip())[0]
    except ValueError, UnicodeDecodeError:
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
    """strconv.ParseInt(s, 10, 32) and id > 0."""
    if _ID.fullmatch(raw):
        value = int(raw)
        if 0 < value <= 2**31 - 1:
            return value
    raise ApiError(400, f"invalid {entity} id")


def query(request: Request, key: str) -> str:
    """url.Values.Get: the first value, or "" when absent."""
    values = request.query_params.getlist(key)
    return values[0] if values else ""
