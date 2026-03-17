# Habits - Business Logic

## 1. Frequency & Objectives

Each habit has a **frequency** (`daily`, `weekly`, or `monthly`) that determines how progress is measured. Defaults to `daily` if not specified.

A habit may optionally have an **objective** — a positive numeric target. The objective represents the total value (sum of log entries) that should be reached within one period. For example, an objective of `5` on a weekly habit means the sum of all log values within that week must reach 5.

Habits without an objective have no streak tracking (streaks remain at 0).

## 2. Period Boundaries

Periods are calendar-based:

- **Daily**: a single calendar day.
- **Weekly**: Monday through Sunday (ISO 8601 week).
- **Monthly**: 1st through the last day of the calendar month.

## 3. Period Value

The **period value** is the sum of all log entries for a habit within its current period. For example, if a weekly habit has logs on Monday (2), Wednesday (3), and Friday (1), the period value is 6.

The daily endpoint returns each habit's period value alongside the individual day's log value.

## 4. Streak Logic

A **streak** counts consecutive completed periods — periods where the sum of log values met or exceeded the objective.

**Current streak** is calculated by walking backwards from the current period:
- If a past period's sum >= objective → count it and continue to the previous period.
- If a past period's sum < objective or has no logs → the streak is broken, stop.
- The **current (in-progress) period** is special: if the objective is already met, it counts toward the streak. If not yet met, it is skipped without breaking the streak.

**Longest streak** is stored on the habit and updated whenever the current streak exceeds it. This avoids needing to scan all history to compute the all-time best.

## 5. Streak Recalculation

Streaks are recalculated on every log upsert (create or update). The process:

1. Fetch the habit's frequency and objective.
2. If no objective is set, skip (no streak tracking).
3. Fetch all logs for the habit.
4. Group logs by period and sum values per period.
5. Walk backwards from the current period counting consecutive hits.
6. Update `current_streak` and `longest_streak` on the habit.

This full recalculation ensures correctness even when logs are backdated or edited.

## 6. No-Objective Habits

Habits without an objective (`objective IS NULL`) always have `current_streak = 0` and `longest_streak = 0`. Streak recalculation is skipped entirely for these habits.
