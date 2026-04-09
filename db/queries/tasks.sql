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
    due_at      = CASE WHEN @clear_due_at::bool THEN NULL WHEN @set_due_at::bool THEN @due_at::date ELSE due_at END,
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

-- name: ListTasksFast :many
WITH RECURSIVE project_tree AS (
    SELECT id, name, ARRAY[name::text] AS sort_path
    FROM projects
    WHERE parent_id IS NULL AND finished_at IS NULL
    UNION ALL
    SELECT c.id, c.name, pt.sort_path || c.name::text
    FROM projects c
    JOIN project_tree pt ON c.parent_id = pt.id
    WHERE c.finished_at IS NULL
)
SELECT t.id, t.name, t.project_id, pt.name AS project_name
FROM tasks t
LEFT JOIN project_tree pt ON t.project_id = pt.id
WHERE t.finished_at IS NULL
ORDER BY
    CASE WHEN t.project_id IS NULL THEN 1 ELSE 0 END,
    pt.sort_path,
    t.name;

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
SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks
FROM tasks t
WHERE t.finished_at IS NULL
ORDER BY t.name;

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
ORDER BY depth, due_at ASC NULLS LAST, name;

-- name: GetTasksByProjectIDs :many
WITH task_times AS (
    SELECT
        t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td2 JOIN tasks t2 ON t2.id = td2.depends_on WHERE td2.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td2 JOIN tasks t2 ON t2.id = td2.task_id WHERE td2.depends_on = t.id), '[]')::json AS blocks,
        EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.project_id = ANY(@project_ids::int[])
    GROUP BY t.id
)
SELECT
    tt.id, tt.project_id, tt.name, tt.description, tt.due_at, tt.started_at, tt.finished_at,
    tt.time_spent, tt.depends_on, tt.blocks, tt.blocked,
    td.id AS todo_id, td.name AS todo_name, td.is_done AS todo_is_done
FROM task_times tt
LEFT JOIN todos td ON td.task_id = tt.id
ORDER BY
    CASE
        WHEN tt.finished_at IS NOT NULL THEN 2
        WHEN tt.started_at IS NOT NULL THEN 0
        ELSE 1
    END,
    tt.due_at ASC NULLS LAST,
    tt.name,
    td.is_done ASC NULLS LAST, todo_id;

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
    COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks
FROM tasks t
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.finished_at IS NULL
GROUP BY t.id, p.id
ORDER BY t.due_at ASC NULLS LAST, p.due_at ASC NULLS LAST, t.name;

-- name: GetTaskByID :many
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks,
        EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.id = $1
    GROUP BY t.id
)
SELECT
    ti.id, ti.project_id, ti.name, ti.description, ti.due_at, ti.started_at, ti.finished_at, ti.time_spent,
    ti.depends_on, ti.blocks, ti.blocked,
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
SELECT te.id, te.task_id, te.started_at, te.finished_at, te.comment,
       t.name AS task_name, p.name AS project_name
FROM time_entries te
JOIN tasks t ON t.id = te.task_id
LEFT JOIN projects p ON p.id = t.project_id
WHERE te.finished_at IS NULL;

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
WITH entry_periods AS (
    SELECT
        te.started_at,
        te.finished_at,
        gs.period_start
    FROM time_entries te,
    LATERAL generate_series(
        date_trunc(@frequency::text, te.started_at AT TIME ZONE @timezone::text),
        date_trunc(@frequency::text, te.finished_at AT TIME ZONE @timezone::text),
        ('1 ' || @frequency::text)::interval
    ) AS gs(period_start)
    WHERE te.finished_at IS NOT NULL
      AND (te.started_at AT TIME ZONE @timezone::text)::date >= @start_at::date
      AND (te.started_at AT TIME ZONE @timezone::text)::date <= @end_at::date
)
SELECT
    ep.period_start::date AS date,
    (SUM(EXTRACT(EPOCH FROM (
        LEAST(ep.finished_at, (ep.period_start + ('1 ' || @frequency::text)::interval) AT TIME ZONE @timezone::text)
        - GREATEST(ep.started_at, ep.period_start AT TIME ZONE @timezone::text)
    ))) / 3600)::REAL AS value
FROM entry_periods ep
GROUP BY ep.period_start
ORDER BY date;

-- name: GetTimeEntriesByTaskID :many
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at,
        COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
        COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks,
        EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.id = $1
    GROUP BY t.id
)
SELECT
    ti.id AS task_id, ti.project_id, ti.name, ti.description, ti.due_at,
    ti.started_at AS task_started_at, ti.finished_at AS task_finished_at, ti.time_spent,
    ti.depends_on, ti.blocks, ti.blocked,
    te.id AS time_entry_id, te.started_at AS entry_started_at, te.finished_at AS entry_finished_at, te.comment
FROM task_info ti
LEFT JOIN time_entries te ON te.task_id = ti.id
ORDER BY te.started_at;

-- name: DeleteRemovedTaskDependencies :exec
DELETE FROM task_dependencies
WHERE task_id = $1 AND NOT (depends_on = ANY(@keep::int[]));

-- name: UpsertTaskDependencies :exec
INSERT INTO task_dependencies (task_id, depends_on)
SELECT $1, t.id
FROM unnest(@depends_on::int[]) AS dep_id
JOIN tasks t ON t.id = dep_id AND t.finished_at IS NULL
WHERE array_length(@depends_on::int[], 1) IS NOT NULL
ON CONFLICT (task_id, depends_on) DO NOTHING;

-- name: GetTaskDependencies :one
SELECT
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = $1 AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = $1), '[]')::json AS blocks,
    EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = $1 AND t3.finished_at IS NULL) AS blocked;

-- name: GetTimeEntriesByDateRange :many
SELECT
    te.id, te.task_id,
    t.name AS task_name,
    t.finished_at AS task_finished_at,
    p.id AS project_id,
    p.name AS project_name,
    te.started_at, te.finished_at, te.comment,
    EXTRACT(EPOCH FROM (COALESCE(te.finished_at, NOW()) - te.started_at))::bigint AS time_spent
FROM time_entries te
JOIN tasks t ON t.id = te.task_id
LEFT JOIN projects p ON p.id = t.project_id
WHERE te.started_at <= @end_time::timestamptz
  AND (te.finished_at >= @start_time::timestamptz OR te.finished_at IS NULL)
ORDER BY te.started_at DESC;
