from gv_api.core.encoding import ApiModel, DayTime
from gv_api.core.http import Body


class ConcelloMark(ApiModel):
    id: int
    name: str
    visited_on: DayTime
    description: str


class CreateMarkRequest(Body):
    name: str = ""
    visited_on: str = ""  # YYYY-MM-DD
    description: str = ""


class UpdateMarkRequest(Body):
    visited_on: str = ""  # YYYY-MM-DD
    description: str = ""
