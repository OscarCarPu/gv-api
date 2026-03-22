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

-- name: UpdateTimeEntry :one
UPDATE time_entries SET
    task_id     = CASE WHEN @set_task_id::bool     THEN @task_id::int           ELSE task_id END,
    started_at  = CASE WHEN @set_started_at::bool  THEN @started_at::timestamptz  ELSE started_at END,
    finished_at = CASE WHEN @set_finished_at::bool  THEN @finished_at::timestamptz ELSE finished_at END,
    comment     = CASE WHEN @set_comment::bool      THEN @comment::text          ELSE comment END
WHERE id = @id
RETURNING id, task_id, started_at, finished_at, comment;

-- name: UpdateTask :one
UPDATE tasks SET
    name        = CASE WHEN @set_name::bool        THEN @name::text             ELSE name END,
    description = CASE WHEN @set_description::bool  THEN @description::text      ELSE description END,
    due_at      = CASE WHEN @set_due_at::bool       THEN @due_at::date           ELSE due_at END,
    project_id  = CASE WHEN @set_project_id::bool   THEN @project_id::int        ELSE project_id END,
    started_at  = CASE WHEN @set_started_at::bool   THEN @started_at::timestamptz  ELSE started_at END,
    finished_at = CASE WHEN @set_finished_at::bool  THEN @finished_at::timestamptz ELSE finished_at END
WHERE id = @id
RETURNING id, project_id, name, description, due_at, started_at, finished_at;

-- name: UpdateProject :one
UPDATE projects SET
    name        = CASE WHEN @set_name::bool        THEN @name::text             ELSE name END,
    description = CASE WHEN @set_description::bool  THEN @description::text      ELSE description END,
    due_at      = CASE WHEN @set_due_at::bool       THEN @due_at::date           ELSE due_at END,
    parent_id   = CASE WHEN @set_parent_id::bool    THEN @parent_id::int         ELSE parent_id END,
    started_at  = CASE WHEN @set_started_at::bool   THEN @started_at::timestamptz  ELSE started_at END,
    finished_at = CASE WHEN @set_finished_at::bool  THEN @finished_at::timestamptz ELSE finished_at END
WHERE id = @id
RETURNING id, parent_id, name, description, due_at, started_at, finished_at;

-- name: ListProjectsFast :many
SELECT id, name FROM projects WHERE finished_at IS NULL ORDER BY name;

-- name: GetRootProjects :many
SELECT id, name, description, due_at, parent_id, started_at
FROM projects
WHERE parent_id IS NULL AND finished_at IS NULL
ORDER BY name;

-- name: GetActiveProjects :many
SELECT id, parent_id, name, due_at
FROM projects
WHERE started_at IS NOT NULL AND finished_at IS NULL
ORDER BY name;

-- name: GetUnfinishedTasks :many
SELECT id, project_id, name, description, due_at, started_at
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
    td.id AS todo_id, td.name AS todo_name, td.is_done AS todo_is_done
FROM task_times tt
LEFT JOIN todos td ON td.task_id = tt.id
ORDER BY tt.finished_at NULLS FIRST, tt.name, td.is_done ASC NULLS LAST, todo_id;

-- name: UpdateTodo :one
UPDATE todos SET
    task_id = CASE WHEN @set_task_id::bool THEN @task_id::int ELSE task_id END,
    name    = CASE WHEN @set_name::bool    THEN @name::text   ELSE name END,
    is_done = CASE WHEN @set_is_done::bool THEN @is_done::bool ELSE is_done END
WHERE id = @id
RETURNING id, task_id, name, is_done;

-- name: GetTasksByDueDate :many
SELECT
    t.id, t.name, t.description, t.due_at, t.started_at,
    p.id AS project_id, p.name AS project_name, p.due_at AS project_due_at,
    COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent
FROM tasks t
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.finished_at IS NULL
  AND (t.due_at IS NOT NULL OR p.due_at IS NOT NULL)
GROUP BY t.id, p.id
ORDER BY t.due_at ASC NULLS LAST, p.due_at ASC NULLS LAST, t.name;

-- name: GetTaskByID :many
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.id = $1
    GROUP BY t.id
)
SELECT
    ti.id, ti.project_id, ti.name, ti.description, ti.due_at, ti.started_at, ti.finished_at, ti.time_spent,
    td.id AS todo_id, td.name AS todo_name, td.is_done AS todo_is_done
FROM task_info ti
LEFT JOIN todos td ON td.task_id = ti.id
ORDER BY td.is_done ASC NULLS LAST, td.id;

-- name: FinishDescendantProjects :exec
WITH RECURSIVE project_tree AS (
    SELECT p.id FROM projects p WHERE p.id = $1
    UNION ALL
    SELECT c.id FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
)
UPDATE projects p2 SET finished_at = NOW()
FROM project_tree pt
WHERE p2.id = pt.id AND p2.id != $1 AND p2.finished_at IS NULL;

-- name: FinishTasksByProjectTree :exec
WITH RECURSIVE project_tree AS (
    SELECT p.id FROM projects p WHERE p.id = $1
    UNION ALL
    SELECT c.id FROM projects c JOIN project_tree pt ON c.parent_id = pt.id
)
UPDATE tasks t SET finished_at = NOW()
FROM project_tree pt
WHERE t.project_id = pt.id AND t.finished_at IS NULL;

-- name: GetActiveTimeEntry :one
SELECT id, task_id, started_at, finished_at, comment
FROM time_entries
WHERE finished_at IS NULL;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;

-- name: DeleteTodo :exec
DELETE FROM todos WHERE id = $1;

-- name: DeleteTimeEntry :exec
DELETE FROM time_entries WHERE id = $1;

-- name: GetTimeEntrySummary :one
SELECT
    COALESCE(SUM(CASE WHEN finished_at >= @today_start::timestamptz
        THEN EXTRACT(EPOCH FROM (finished_at - GREATEST(started_at, @today_start::timestamptz)))::bigint ELSE 0 END), 0)::bigint AS today,
    COALESCE(SUM(EXTRACT(EPOCH FROM (finished_at - GREATEST(started_at, @week_start::timestamptz)))::bigint), 0)::bigint AS week
FROM time_entries
WHERE finished_at IS NOT NULL
  AND finished_at >= @week_start::timestamptz;

-- name: GetTimeEntryHistory :many
SELECT
    date_trunc(@frequency::text, started_at AT TIME ZONE @timezone::text)::date AS date,
    (SUM(EXTRACT(EPOCH FROM (finished_at - started_at))) / 3600)::REAL AS value
FROM time_entries
WHERE finished_at IS NOT NULL
  AND (started_at AT TIME ZONE @timezone::text)::date >= @start_at::date
  AND (started_at AT TIME ZONE @timezone::text)::date <= @end_at::date
GROUP BY date_trunc(@frequency::text, started_at AT TIME ZONE @timezone::text)
ORDER BY date;

-- name: GetTimeEntriesByTaskID :many
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.id = $1
    GROUP BY t.id
)
SELECT
    ti.id AS task_id, ti.project_id, ti.name, ti.description, ti.due_at,
    ti.started_at AS task_started_at, ti.finished_at AS task_finished_at, ti.time_spent,
    te.id AS time_entry_id, te.started_at AS entry_started_at, te.finished_at AS entry_finished_at, te.comment
FROM task_info ti
LEFT JOIN time_entries te ON te.task_id = ti.id
ORDER BY te.started_at;
