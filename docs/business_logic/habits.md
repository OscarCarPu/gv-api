# Habits - Business Logic

## 1. Frequency & Targets

Each habit has a **frequency** (`daily`, `weekly`, or `monthly`) that determines how progress is measured. Defaults to `daily` if not specified.

A habit may optionally have **target_min** and/or **target_max** — numeric bounds that define when a period is considered "met":

- **target_min only**: period sum must be >= target_min (e.g. "at least 3 sessions per week")
- **target_max only**: period sum must be <= target_max (e.g. "no more than 2000 calories")
- **Both set**: period sum must be within [target_min, target_max] (e.g. "weight between 60-80kg")
- **Neither set**: no streak tracking

## 2. Recording Required

The `recording_required` boolean (default `true`) controls how missing days affect streak calculation:

- **true**: a missing day (no log entry) breaks the streak (standard behavior)
- **false**: a missing day carries forward the last recorded value. This is useful for metrics like weight where the value persists even when not recorded.

Carry-forward only affects Go streak calculation, NOT the SQL `period_value`.

### Carry-forward examples (daily, target_min=1):
- `0, 1, null, null` → effective: `0, 1, 1, 1` → streak = 3
- `1, 1, 0, null, null` → effective: `1, 1, 0, 0, 0` → streak = 0

## 3. Period Boundaries

Periods are calendar-based:

- **Daily**: a single calendar day.
- **Weekly**: Monday through Sunday (ISO 8601 week).
- **Monthly**: 1st through the last day of the calendar month.

## 4. Period Value

The **period value** is the sum of all log entries for a habit within its current period. For example, if a weekly habit has logs on Monday (2), Wednesday (3), and Friday (1), the period value is 6.

The daily endpoint returns each habit's period value alongside the individual day's log value.

## 5. Streak Logic

A **streak** counts consecutive completed periods — periods where the sum of log values met the target criteria.

**Current streak** is calculated by walking backwards from the current period:
- If a past period meets the target → count it and continue to the previous period.
- If a past period doesn't meet the target → the streak is broken, stop.
- The **current (in-progress) period** is special: if the target is already met, it counts toward the streak. If not yet met, it is skipped without breaking the streak.

**Longest streak** is recalculated from all periods on every log upsert.

## 6. Streak Recalculation

Streaks are recalculated on every log upsert (create or update). The process:

1. Fetch the habit's frequency, targets, and recording_required flag.
2. If no targets are set (both target_min and target_max are null), skip (no streak tracking).
3. Fetch all logs for the habit.
4. Build effective period sums:
   - If recording_required=true: group logs by period and sum.
   - If recording_required=false: walk every day from earliest log to today, carrying forward the last recorded value, then group by period and sum.
5. Walk backwards from the current period counting consecutive hits.
6. Update `current_streak` and `longest_streak` on the habit.

This full recalculation ensures correctness even when logs are backdated or edited.

## 7. No-Target Habits

Habits without targets (both `target_min` and `target_max` are null) always have `current_streak = 0` and `longest_streak = 0`. Streak recalculation is skipped entirely for these habits.
