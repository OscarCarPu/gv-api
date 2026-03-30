# Habits

### Description

Domain for tracking recurring personal metrics. A habit represents something the user wants to measure over time (exercise sessions, weight, calories, etc.). Each habit has a frequency that defines how progress is grouped into periods, and optional numeric targets that define when a period is considered "met". The system tracks streaks of consecutive successful periods.

### States / Lifecycle

A habit itself has no state machine — it exists from creation until deletion. The interesting lifecycle is at the **log + streak** level:

```
Habit created (streak = 0)
  → Logs recorded (upsert per date)
    → Streak recalculated on every log
      → Habit deleted (cascade deletes all logs)
```

### Business Rules

**Frequency & Periods**
- Each habit has a frequency: `daily`, `weekly`, or `monthly` (default: `daily`).
- Periods are calendar-based: daily = single day, weekly = Monday–Sunday (ISO 8601), monthly = 1st–last day of month.
- The **period value** is the sum of all log entries within the current period.

**Targets**
- A habit may have `target_min` and/or `target_max` — numeric bounds on the period sum.
- `target_min` only → period sum must be >= target_min.
- `target_max` only → period sum must be <= target_max.
- Both set → period sum must be within [target_min, target_max].
- Neither set → no streak tracking, streaks stay at 0.

**Logging**
- One log per habit per day (unique constraint on habit_id + log_date).
- Upsert behavior: logging the same date replaces the previous value.
- Logs can be backdated or edited — streak recalculation handles this correctly.

**Streak Calculation**
- A streak counts consecutive periods where the period sum met the target criteria.
- **Current streak**: walk backwards from the current period.
  - Past period meets target → count it, continue backwards.
  - Past period doesn't meet target → streak broken, stop.
  - Current (in-progress) period: if target already met, count it. If not yet met, skip it without breaking the streak.
- **Longest streak**: scan all historical periods, find the longest consecutive run meeting target.
- Full recalculation from all logs on every upsert — no incremental updates.

**Carry-forward (recording_required = false)**
- When `recording_required = true` (default): a missing day means no value — breaks the streak.
- When `recording_required = false`: a missing day carries forward the last recorded value. Useful for metrics like weight that persist between measurements.
- Carry-forward walks every day from the earliest log to today, using the last recorded value for unrecorded days, then groups by period and sums.
- Carry-forward only affects streak calculation, NOT the SQL-computed `period_value`.

**History Aggregation**
- The history endpoint aggregates logs into periodic buckets with a configurable frequency.
- If the view frequency is coarser than the habit's native frequency (e.g. daily habit viewed weekly): uses **AVG** to prevent inflated numbers.
- Otherwise: uses **SUM**.
- When `recording_required = true`: missing periods are zero-filled. When `false`: missing periods are omitted.

### Validations

**Create habit**
- `name` required (non-empty).
- `frequency` if provided must be `daily`, `weekly`, or `monthly`.
- `target_min` if provided must be >= 0.
- `target_max` if provided must be >= 0.
- If both targets provided: `target_min` must be <= `target_max`.

**Log upsert**
- `date` must be a valid `YYYY-MM-DD` string.

**History**
- `frequency` if provided must be `daily`, `weekly`, or `monthly`. Defaults to the habit's native frequency.
- `start_at` / `end_at` if provided must be valid `YYYY-MM-DD`. Defaults: daily = 1 month back, weekly = 12 weeks back, monthly = 1 year back.

### Side Effects

- **On log upsert** → full streak recalculation (current_streak + longest_streak updated on the habit row).
- **On habit delete** → cascade deletes all habit_logs (FK ON DELETE CASCADE).

### Decisions / Why

- **Full recalculation on every log instead of incremental**: logs can be backdated or edited, so incremental updates would drift. The full scan is cheap (one habit's logs fit in memory).
- **AVG for coarser history views**: a daily habit viewed monthly would show ~30x inflated values with SUM. AVG normalizes this to a representative daily value for the period.
- **Carry-forward only in Go, not SQL**: the `period_value` in the daily endpoint is always the raw sum from SQL. Carry-forward is a streak-only concept — mixing it into the period value would confuse the UI (showing values on days the user didn't actually log).
- **recording_required defaults to true**: most habits expect active daily logging. The `false` mode is opt-in for passive metrics.
