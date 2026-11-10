from datetime import date

from sqlalchemy import TextClause, delete, select, text, update
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncEngine

from gv_api.habits.errors import NotFoundError
from gv_api.habits.schemas import Habit, HabitWithLog
from gv_api.habits.tables import habit_logs, habits
from gv_api.history import Point

_HABIT_COLUMNS = (
    habits.c.id,
    habits.c.name,
    habits.c.description,
    habits.c.frequency,
    habits.c.target_min,
    habits.c.target_max,
    habits.c.recording_required,
    habits.c.current_streak,
    habits.c.longest_streak,
)

# A habit with target_max = 0 ("never do it") has its streak computed live, since it grows on
# days nothing is logged; the others read the columns recalculate_streak keeps.
_HABITS_WITH_LOGS = text("""
SELECT
  h.id, h.name, h.description, h.frequency, h.target_min, h.target_max, h.recording_required,
  CASE WHEN h.target_max = 0 THEN COALESCE(zs.current_streak, 0)
       ELSE h.current_streak END::int AS current_streak,
  CASE WHEN h.target_max = 0 THEN COALESCE(zs.longest_streak, 0)
       ELSE h.longest_streak END::int AS longest_streak,
  hl.value AS log_value,
  COALESCE((
    SELECT SUM(hl2.value) FROM habit_logs hl2
    WHERE hl2.habit_id = h.id
      AND hl2.log_date BETWEEN
        CASE h.frequency
          WHEN 'daily' THEN CAST(:target_date AS date)
          WHEN 'weekly' THEN date_trunc('week', CAST(:target_date AS date))::date
          WHEN 'monthly' THEN date_trunc('month', CAST(:target_date AS date))::date
        END
      AND
        CASE h.frequency
          WHEN 'daily' THEN CAST(:target_date AS date)
          WHEN 'weekly' THEN (date_trunc('week', CAST(:target_date AS date))
                              + INTERVAL '6 days')::date
          WHEN 'monthly' THEN (date_trunc('month', CAST(:target_date AS date))
                               + INTERVAL '1 month - 1 day')::date
        END
  ), 0)::real AS period_value
FROM habits h
LEFT JOIN habit_logs hl ON h.id = hl.habit_id AND hl.log_date = CAST(:target_date AS date)
LEFT JOIN LATERAL recalculate_habit_streak(h.id, CAST(:streak_today AS date)) AS zs
  ON h.target_max = 0
ORDER BY h.id
""")

_RECALCULATE_STREAK = text("""
UPDATE habits AS h
SET current_streak = r.current_streak, longest_streak = r.longest_streak
FROM recalculate_habit_streak(:id, CAST(:today AS date)) AS r
WHERE h.id = :id
""")


def _history_query(agg: str) -> str:
    # fill_zeros returns every period in [start_at, end_at], not only the logged ones; the
    # caller snaps both ends to period boundaries.
    # agg is SUM or AVG
    return f"""
SELECT s.date::date AS date,
  COALESCE((
    SELECT {agg}(hl.value)::real FROM habit_logs hl
    WHERE hl.habit_id = :habit_id
      AND date_trunc(CAST(:frequency AS text), hl.log_date::timestamp)::date = s.date
  ), 0)::real AS value
FROM generate_series(
  CAST(:start_at AS date), CAST(:end_at AS date), ('1 ' || CAST(:frequency AS text))::interval
) AS s (date)
WHERE CAST(:fill_zeros AS bool)
UNION ALL
SELECT date_trunc(CAST(:frequency AS text), hl.log_date::timestamp)::date AS date,
  {agg}(hl.value)::real AS value
FROM habit_logs hl
WHERE NOT CAST(:fill_zeros AS bool)
  AND hl.habit_id = :habit_id
  AND hl.log_date >= CAST(:start_at AS date)
  AND hl.log_date <= CAST(:end_at AS date)
GROUP BY date_trunc(CAST(:frequency AS text), hl.log_date::timestamp)
ORDER BY date
"""  # noqa: S608


_HISTORY_SUM = text(_history_query("SUM"))
_HISTORY_AVG = text(_history_query("AVG"))


class HabitsRepository:
    def __init__(self, engine: AsyncEngine) -> None:
        self._engine = engine

    async def get_habits_with_logs(self, view_date: date, streak_today: date) -> list[HabitWithLog]:
        async with self._engine.connect() as conn:
            rows = await conn.execute(
                _HABITS_WITH_LOGS, {"target_date": view_date, "streak_today": streak_today}
            )
        return [HabitWithLog.model_construct(**r._mapping) for r in rows]

    async def upsert_log(self, habit_id: int, day: date, value: float) -> None:
        stmt = insert(habit_logs).values(habit_id=habit_id, log_date=day, value=value)
        stmt = stmt.on_conflict_do_update(
            index_elements=[habit_logs.c.habit_id, habit_logs.c.log_date],
            set_={"value": stmt.excluded.value},
        )
        async with self._engine.begin() as conn:
            await conn.execute(stmt)

    async def create_habit(
        self,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit:
        stmt = (
            habits.insert()
            .values(
                name=name,
                description=description,
                frequency=frequency,
                target_min=target_min,
                target_max=target_max,
                recording_required=recording_required,
            )
            .returning(*_HABIT_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one()
        return Habit.model_construct(**row._mapping)

    async def update_habit(
        self,
        id: int,
        name: str,
        description: str | None,
        frequency: str,
        target_min: float | None,
        target_max: float | None,
        recording_required: bool,
    ) -> Habit:
        stmt = (
            update(habits)
            .where(habits.c.id == id)
            .values(
                name=name,
                description=description,
                frequency=frequency,
                target_min=target_min,
                target_max=target_max,
                recording_required=recording_required,
            )
            .returning(*_HABIT_COLUMNS)
        )
        async with self._engine.begin() as conn:
            row = (await conn.execute(stmt)).one_or_none()
        if row is None:
            raise NotFoundError
        return Habit.model_construct(**row._mapping)

    async def delete_habit(self, id: int) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(delete(habits).where(habits.c.id == id))

    async def get_habit_by_id(self, id: int) -> Habit:
        async with self._engine.connect() as conn:
            row = (
                await conn.execute(select(*_HABIT_COLUMNS).where(habits.c.id == id))
            ).one_or_none()
        if row is None:
            raise NotFoundError
        return Habit.model_construct(**row._mapping)

    async def recalculate_streak(self, habit_id: int, today: date) -> None:
        async with self._engine.begin() as conn:
            await conn.execute(_RECALCULATE_STREAK, {"id": habit_id, "today": today})

    async def get_habit_history(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[Point]:
        return await self._history(_HISTORY_SUM, habit_id, frequency, start_at, end_at, fill_zeros)

    async def get_habit_history_avg(
        self, habit_id: int, frequency: str, start_at: date, end_at: date, fill_zeros: bool
    ) -> list[Point]:
        return await self._history(_HISTORY_AVG, habit_id, frequency, start_at, end_at, fill_zeros)

    async def _history(
        self,
        query: TextClause,
        habit_id: int,
        frequency: str,
        start_at: date,
        end_at: date,
        fill_zeros: bool,
    ) -> list[Point]:
        params = {
            "habit_id": habit_id,
            "frequency": frequency,
            "start_at": start_at,
            "end_at": end_at,
            "fill_zeros": fill_zeros,
        }
        async with self._engine.connect() as conn:
            rows = await conn.execute(query, params)
        return [Point.model_construct(date=r.date, value=r.value) for r in rows]
