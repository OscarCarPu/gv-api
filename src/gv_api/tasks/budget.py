"""The weekly time budget: how much of what is left of it today should take."""

from datetime import datetime

from gv_api.tasks.schemas import PaceBreakdown

WEEKLY_TASK_TARGET_SECONDS = 288_000
WEEKDAY_WAKING_HOURS = 17
WEEKEND_WAKING_HOURS = 12


def waking_hours(iso_weekday: int) -> int:
    return WEEKEND_WAKING_HOURS if iso_weekday >= 6 else WEEKDAY_WAKING_HOURS


def calc_pace(now: datetime, week_seconds: int, today_seconds: int) -> PaceBreakdown:
    """Today's goal from the time logged on earlier days and the waking hours left this week,
    independent of the time of day."""
    if week_seconds >= WEEKLY_TASK_TARGET_SECONDS:
        return PaceBreakdown(goal_reached=True)
    remaining = WEEKLY_TASK_TARGET_SECONDS - max(week_seconds - today_seconds, 0)
    iso_day = now.isoweekday()
    remaining_full_days = 7 - iso_day
    if remaining <= 0:
        return PaceBreakdown(remaining_full_days=remaining_full_days)

    today_share = waking_hours(iso_day)
    weighted_total = sum(waking_hours(d) for d in range(iso_day, 8))
    uniform_per_day = int(remaining / (1 + remaining_full_days))
    return PaceBreakdown(
        uniform_per_day_seconds=uniform_per_day,
        uniform_today_share_seconds=uniform_per_day,
        weighted_weekday_seconds=int(remaining * WEEKDAY_WAKING_HOURS / weighted_total),
        weighted_weekend_seconds=int(remaining * WEEKEND_WAKING_HOURS / weighted_total),
        weighted_today_share_seconds=int(remaining * today_share / weighted_total),
        remaining_full_days=remaining_full_days,
    )


def daily_target_seconds(pace: PaceBreakdown) -> int:
    return 0 if pace.goal_reached else pace.weighted_today_share_seconds
