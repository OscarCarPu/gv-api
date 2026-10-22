-- name: ListCalendarEvents :many
-- Events overlapping the window.
SELECT * FROM calendar_events
WHERE start_time < @range_end
  AND (end_time > @range_start OR (end_time = start_time AND start_time >= @range_start))
ORDER BY start_time, id;

-- name: GetCalendarEvent :one
SELECT * FROM calendar_events WHERE id = $1;

-- name: CreateCalendarEvent :one
INSERT INTO calendar_events (name, start_time, end_time)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateCalendarEvent :one
UPDATE calendar_events SET
    name       = COALESCE(sqlc.narg('name'), name),
    start_time = COALESCE(sqlc.narg('start_time'), start_time),
    end_time   = COALESCE(sqlc.narg('end_time'), end_time)
WHERE id = @id
RETURNING *;

-- name: DeleteCalendarEvent :execrows
DELETE FROM calendar_events WHERE id = $1;
