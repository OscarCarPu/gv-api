-- name: CreateProject :one
INSERT INTO projects (name, description, due_at, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, due_at, parent_id;
