-- name: GetHabitsWithLogs :many
SELECT
    h.id, h.name, h.description,
    h.frequency, h.target_min, h.target_max, h.recording_required,
    h.current_streak, h.longest_streak,
    hl.value AS log_value,
    COALESCE(
        (SELECT SUM(hl2.value)
         FROM habit_logs hl2
         WHERE hl2.habit_id = h.id
           AND hl2.log_date BETWEEN
               CASE h.frequency
                   WHEN 'daily' THEN @target_date::date
                   WHEN 'weekly' THEN date_trunc('week', @target_date::date)::date
                   WHEN 'monthly' THEN date_trunc('month', @target_date::date)::date
               END
               AND
               CASE h.frequency
                   WHEN 'daily' THEN @target_date::date
                   WHEN 'weekly' THEN (date_trunc('week', @target_date::date) + INTERVAL '6 days')::date
                   WHEN 'monthly' THEN (date_trunc('month', @target_date::date) + INTERVAL '1 month - 1 day')::date
               END
        ), 0
    )::REAL AS period_value
FROM habits h
LEFT JOIN habit_logs hl ON h.id = hl.habit_id AND hl.log_date = @target_date
ORDER BY h.id;

-- name: UpsertLog :exec
INSERT INTO habit_logs (habit_id, log_date, value)
VALUES ($1, $2, $3)
ON CONFLICT (habit_id, log_date)
DO UPDATE SET value = excluded.value;

-- name: DeleteHabit :exec
DELETE FROM habits WHERE id = $1;

-- name: CreateHabit :one
INSERT INTO habits (name, description, frequency, target_min, target_max, recording_required)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, name, description, frequency, target_min, target_max, recording_required, current_streak, longest_streak;

-- name: GetHabitByID :one
SELECT id, name, description, frequency, target_min, target_max, recording_required, current_streak, longest_streak
FROM habits WHERE id = $1;

-- name: RecalculateHabitStreak :exec
-- Computes current_streak and longest_streak from this habit's logs (see
-- migration 012's recalculate_habit_streak function) and writes them back.
-- @today_in is the user's "today" snapped to UTC midnight (caller decides
-- the location), used to determine the current period.
UPDATE habits AS h
SET current_streak = r.current_streak, longest_streak = r.longest_streak
FROM recalculate_habit_streak($1, @today_in::date) AS r
WHERE h.id = $1;

-- name: GetHabitHistory :many
-- When @fill_zeros is true, every period in the inclusive [start_at, end_at]
-- range is returned with COALESCE(SUM, 0); otherwise only periods with logs
-- appear. Caller is responsible for snapping start/end to period boundaries.
SELECT s.date::date AS date,
    COALESCE((
        SELECT SUM(hl.value)::REAL
        FROM habit_logs hl
        WHERE hl.habit_id = @habit_id
          AND date_trunc(@frequency::text, hl.log_date::timestamp)::date = s.date
    ), 0)::REAL AS value
FROM generate_series(@start_at::date, @end_at::date, ('1 ' || @frequency::text)::interval) AS s(date)
WHERE @fill_zeros::bool
UNION ALL
SELECT date_trunc(@frequency::text, hl.log_date::timestamp)::date AS date,
    SUM(hl.value)::REAL AS value
FROM habit_logs hl
WHERE NOT @fill_zeros::bool
  AND hl.habit_id = @habit_id
  AND hl.log_date >= @start_at::date
  AND hl.log_date <= @end_at::date
GROUP BY date_trunc(@frequency::text, hl.log_date::timestamp)
ORDER BY date;

-- name: GetHabitHistoryAvg :many
SELECT s.date::date AS date,
    COALESCE((
        SELECT AVG(hl.value)::REAL
        FROM habit_logs hl
        WHERE hl.habit_id = @habit_id
          AND date_trunc(@frequency::text, hl.log_date::timestamp)::date = s.date
    ), 0)::REAL AS value
FROM generate_series(@start_at::date, @end_at::date, ('1 ' || @frequency::text)::interval) AS s(date)
WHERE @fill_zeros::bool
UNION ALL
SELECT date_trunc(@frequency::text, hl.log_date::timestamp)::date AS date,
    AVG(hl.value)::REAL AS value
FROM habit_logs hl
WHERE NOT @fill_zeros::bool
  AND hl.habit_id = @habit_id
  AND hl.log_date >= @start_at::date
  AND hl.log_date <= @end_at::date
GROUP BY date_trunc(@frequency::text, hl.log_date::timestamp)
ORDER BY date;
