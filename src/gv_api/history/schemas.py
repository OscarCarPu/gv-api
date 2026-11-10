from gv_api.core.encoding import ApiModel, Day, Float32


class Point(ApiModel):
    date: Day
    value: Float32


class Response(ApiModel):
    start_at: Day
    end_at: Day
    data: list[Point] = []
