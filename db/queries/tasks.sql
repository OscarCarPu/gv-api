-- name: CreateProject :one
INSERT INTO projects (name, description, due_at, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, due_at, parent_id;

-- name: CreateTask :one
INSERT INTO tasks (project_id, name, description, due_at)
VALUES ($1, $2, $3, $4)
RETURNING id, project_id, name, description, due_at;

-- name: CreateTodo :one
INSERT INTO todos (task_id, name)
VALUES ($1, $2)
RETURNING id, task_id, name;

-- name: CreateTimeEntry :one
INSERT INTO time_entries (task_id, started_at, finished_at, comment)
VALUES ($1, $2, $3, $4)
RETURNING id, task_id, started_at, finished_at, comment;

-- name: FinishTimeEntry :one
UPDATE time_entries SET finished_at = $2
WHERE id = $1
RETURNING id, task_id, started_at, finished_at, comment;

-- name: GetRootProjects :many
SELECT id, name, description, due_at, parent_id, started_at
FROM projects
WHERE parent_id IS NULL AND finished_at IS NULL
ORDER BY name;
