"""Urgency for /tasks/tasks/by-due-date: back-fill each deadline into the free hours before it.

Each standard task with an estimate gets the latest day it can start (start_by) and still
finish by its deadline. A dependency chain A -> B -> C is filled from its end, so A has to fit
A+B+C's hours before C's deadline. Recurring tasks are left out.
"""

from datetime import date, timedelta
from decimal import ROUND_HALF_UP, Decimal

from gv_api.tasks.schemas import DepRemaining, TaskByDueDateResponse

# Future days are capped at capacity minus half an hour per day from today, never below the
# floor, to absorb everything not planned yet.
DECAY_PER_DAY = Decimal("0.5")
FLOOR_HOURS = Decimal(6)
HOURS_PLACES = Decimal("1e-16")


def hours(seconds: int | float) -> Decimal:
    return (Decimal(seconds) / 3600).quantize(HOURS_PLACES, ROUND_HALF_UP)


def day_cap(capacity_hours: Decimal, days_from_today: int) -> Decimal:
    return max(capacity_hours - DECAY_PER_DAY * days_from_today, FLOOR_HOURS)


def is_estimated(task: TaskByDueDateResponse) -> bool:
    return task.task_type == "standard" and task.estimate_hours is not None


def remaining_hours(task: TaskByDueDateResponse, planned: Decimal) -> Decimal:
    if task.estimate_hours is None or not is_estimated(task):
        return Decimal(0)
    return max(task.estimate_hours - hours(task.time_spent) - planned, Decimal(0))


def chain_priorities(rows: list[TaskByDueDateResponse]) -> list[int]:
    """Each row's priority raised to the highest (lowest number) of anything it transitively
    blocks."""
    index = {t.id: i for i, t in enumerate(rows)}
    prio = [0] * len(rows)
    state = [0] * len(rows)  # 0 unvisited, 1 in progress, 2 done

    def visit(i: int) -> int:
        if state[i]:
            return prio[i]  # a cycle (rejected on write): the partial value will do
        state[i] = 1
        prio[i] = rows[i].priority
        for b in rows[i].blocks:
            if b.id in index:
                prio[i] = min(prio[i], visit(index[b.id]))
        state[i] = 2
        return prio[i]

    for i in range(len(rows)):
        visit(i)
    return prio


def backfill(
    rows: list[TaskByDueDateResponse],
    dues: list[date],
    in_set: dict[int, int],
    rem_hours: dict[int, Decimal],
    priority: list[int],
    free_by_date: dict[date, Decimal],
    today: date,
    high_first: bool,
) -> tuple[dict[date, Decimal], Decimal]:
    """Place every dated task, latest work first, and return the free hours left per day and
    the hours that fit nowhere. Writes work_order, finish_by, remaining_hours, start_by and
    urgent on `rows`."""
    free = dict(free_by_date)
    pending_dependents = {
        i: sum(1 for b in rows[i].blocks if b.id in in_set) for i in in_set.values()
    }
    chain_start: dict[int, date] = {}

    def last_day(i: int) -> date:
        # The due date itself, or the day a dependent starts (the work may share it).
        last = dues[i]
        for b in rows[i].blocks:
            if b.id in in_set:
                last = min(last, chain_start.get(in_set[b.id], date.min))
        return last

    def claim(i: int) -> tuple[int, int, int, Decimal, int]:
        # The smallest claims first: the lowest priority (the highest with high_first), then
        # the later deadline, the later last day, fewer remaining hours and the lower id.
        p = -priority[i] if high_first else priority[i]
        return (-p, -dues[i].toordinal(), -last_day(i).toordinal(), rem_hours[i], rows[i].id)

    remaining = set(in_set.values())
    overflow = Decimal(0)
    while remaining:
        # Ready tasks (no unplaced dependent) first; members of a cycle that slipped through
        # are placed ignoring the edges.
        ready = [i for i in remaining if pending_dependents[i] == 0] or list(remaining)
        pick = min(ready, key=claim)
        last = last_day(pick)
        # Claims run backwards, so the last task placed is the first to do.
        task = rows[pick]
        task.work_order = len(remaining)
        remaining.discard(pick)
        for d in task.depends_on:
            if d.id in in_set:
                pending_dependents[in_set[d.id]] -= 1
        task.finish_by = last
        chain_start[pick] = last
        if not is_estimated(task):
            continue
        rem = rem_hours[pick]
        task.remaining_hours = rem
        if rem == 0:
            continue

        taken = Decimal(0)
        start_by = today
        day = last
        while day >= today:
            if (avail := free.get(day, Decimal(0))) > 0:
                take = min(avail, rem - taken)
                taken += take
                free[day] = avail - take
            if taken >= rem:
                start_by = day
                break
            day -= timedelta(days=1)
        if taken < rem:
            overflow += rem - taken
        chain_start[pick] = start_by
        task.start_by = start_by
        task.urgent = start_by <= today
    return free, overflow


def apply_dependent_hours(rows: list[TaskByDueDateResponse]) -> None:
    """For each task, the tasks downstream of it with their own and cumulative remaining hours,
    in work order."""
    index = {t.id: i for i, t in enumerate(rows)}

    def own(i: int) -> Decimal:
        return rows[i].remaining_hours or Decimal(0)

    def cumulative(start: int) -> Decimal:
        seen: set[int] = set()
        stack = [start]
        total = Decimal(0)
        while stack:
            i = stack.pop()
            if i in seen:
                continue
            seen.add(i)
            total += own(i)
            stack += [index[d.id] for d in rows[i].depends_on if d.id in index]
        return total

    def downstream(i: int) -> list[int]:
        found: list[int] = []

        def walk(k: int) -> None:
            for b in rows[k].blocks:
                j = index.get(b.id)
                if j is not None and j != i and j not in found:
                    found.append(j)
                    walk(j)

        walk(i)
        return found

    for i, task in enumerate(rows):
        after = downstream(i)
        if all(rows[j].work_order is not None for j in after):
            after.sort(key=lambda j: rows[j].work_order or 0)
        task.dependent_hours = [
            DepRemaining(
                id=rows[j].id,
                name=rows[j].name,
                remaining_hours=own(j),
                cumulative_hours=cumulative(j),
            )
            for j in after
        ]
