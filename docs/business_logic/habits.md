# Habits

### Description

Recurring personal metrics (exercise, weight, calories...). Each habit has a frequency that groups logs into periods and optional targets that decide whether a period is met; streaks count consecutive met periods.

### Business Rules

**Frequency & Periods**
- `daily` (default), `weekly` (Monday–Sunday) or `monthly`, all calendar-based.
- A period's value is the sum of its logs.

**Targets**
- `target_min` and/or `target_max` bound the period sum. With neither, streaks stay at 0.

**Logging**
- One log per habit per day; logging the same date replaces it. Logs can be backdated.

**Streaks**
- Computed by the SQL function `recalculate_habit_streak`, fully recalculated on every log.
- **Current streak**: walks back from the current period and stops at the first unmet one. The in-progress period counts if already met and is skipped otherwise.
- **Longest streak**: the longest run of met periods.
- Counting starts at the earliest log, or at `created_at` if there are none.
- `target_max = 0` habits (e.g. "times smoked") also get a live streak on read, since a day without a log is met.

**Carry-forward (`recording_required = false`)**
- With `true` (default) a missing day is 0. With `false` it carries the last recorded value, for metrics like weight.
- Carry-forward only affects streaks, not the reported `period_value`.

**History**
- Buckets logs by the requested frequency: **AVG** when coarser than the habit's own (so a daily habit viewed monthly is not ~30x inflated), **SUM** otherwise.
- Missing periods are zero-filled when `recording_required = true` and omitted otherwise.

### Validations

- `name` required; `frequency` one of `daily`, `weekly`, `monthly`.
- Targets `>= 0`, and `target_min <= target_max` when both are set.
- Log `date` and history `start_at`/`end_at` are `YYYY-MM-DD`. History defaults: 1 month (daily), 12 weeks (weekly), 1 year (monthly).

### Side Effects

- **Log upserted** → streaks recalculated on the habit row.
- **Habit deleted** → its logs are deleted (FK cascade).

### Decisions / Why

- **Full recalculation instead of incremental**: backdated or edited logs would make incremental updates drift.
- **Carry-forward is streak-only**: showing values on days the user did not log would be confusing.
- **`recording_required` defaults to true**: most habits expect active logging.
