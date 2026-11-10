from gv_api.core.encoding import ApiModel, Day, Real


class Point(ApiModel):
    date: Day
    value: Real


class Response(ApiModel):
    start_at: Day
    end_at: Day
    data: list[Point] = []
