-- name: ListConcelloMarks :many
SELECT id, name, visited_on, description, created_at
FROM concello_marks
ORDER BY visited_on DESC, name ASC;

-- name: GetConcelloMark :one
SELECT id, name, visited_on, description, created_at
FROM concello_marks
WHERE id = $1;

-- name: CreateConcelloMark :one
INSERT INTO concello_marks (name, visited_on, description)
VALUES ($1, $2, $3)
RETURNING id, name, visited_on, description, created_at;

-- name: UpdateConcelloMark :one
UPDATE concello_marks
SET visited_on = $2, description = $3
WHERE id = $1
RETURNING id, name, visited_on, description, created_at;

-- name: DeleteConcelloMark :exec
DELETE FROM concello_marks WHERE id = $1;
