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

-- name: FinishTask :one
UPDATE tasks SET finished_at = $2
WHERE id = $1
RETURNING id, project_id, name, description, due_at, started_at, finished_at;

-- name: FinishProject :one
UPDATE projects SET finished_at = $2
WHERE id = $1
RETURNING id, parent_id, name, description, due_at, started_at, finished_at;

-- name: GetRootProjects :many
SELECT id, name, description, due_at, parent_id, started_at
FROM projects
WHERE parent_id IS NULL AND finished_at IS NULL
ORDER BY name;

-- name: GetActiveProjects :many
SELECT id, parent_id, name
FROM projects
WHERE started_at IS NOT NULL AND finished_at IS NULL
ORDER BY name;

-- name: GetUnfinishedTasks :many
SELECT id, project_id, name, started_at
FROM tasks
WHERE finished_at IS NULL
ORDER BY name;

-- name: GetProjectWithDescendants :many
WITH RECURSIVE project_tree AS (
    SELECT p.id, p.parent_id, p.name, p.description, p.due_at, p.started_at, p.finished_at, 0 AS depth
    FROM projects p WHERE p.id = $1
    UNION ALL
    SELECT c.id, c.parent_id, c.name, c.description, c.due_at, c.started_at, c.finished_at, pt.depth + 1
    FROM projects c
    JOIN project_tree pt ON c.parent_id = pt.id
)
SELECT id, parent_id, name, description, due_at, started_at, finished_at, depth
FROM project_tree
ORDER BY depth, name;

-- name: GetTasksByProjectIDs :many
WITH task_times AS (
    SELECT
        t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.project_id = ANY(@project_ids::int[])
    GROUP BY t.id
)
SELECT
    tt.id, tt.project_id, tt.name, tt.description, tt.due_at, tt.started_at, tt.finished_at,
    tt.time_spent,
    td.id AS todo_id, td.name AS todo_name
FROM task_times tt
LEFT JOIN todos td ON td.task_id = tt.id
ORDER BY tt.finished_at NULLS FIRST, tt.name, todo_id;

-- name: GetTimeEntriesByTaskID :many
SELECT id, task_id, started_at, finished_at, comment
FROM time_entries
WHERE task_id = $1
ORDER BY started_at;

-- name: TaskExists :one
SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1);
