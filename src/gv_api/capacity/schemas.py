from gv_api.core.encoding import ApiModel, Day, Money


class DayFreeBusy(ApiModel):
    date: Day  # in the service's zone
    capacity_hours: Money  # the same every day
    busy_hours: Money
    free_hours: Money  # max(capacity - busy, 0)
