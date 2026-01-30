-- name: GetHabitsWithLogs :many
SELECT h.id, h.name, h.description, hl.value
FROM habits h 
LEFT JOIN habit_logs hl ON h.id = hl.habit_id AND hl.log_date = $1;

-- name: UpsertLog :exec
INSERT INTO habit_logs (habit_id, log_date, value)
VALUES ($1, $2, $3)
ON CONFLICT (habit_id, log_date)
DO UPDATE SET value = excluded.value;
