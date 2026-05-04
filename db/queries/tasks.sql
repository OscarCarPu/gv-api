-- name: CreateProject :one
INSERT INTO projects (name, description, due_at, parent_id)
VALUES ($1, $2, $3, $4)
RETURNING id, name, description, due_at, parent_id;

-- name: CreateTask :one
INSERT INTO tasks (project_id, name, description, due_at, task_type, recurrence, priority)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, project_id, name, description, due_at, task_type, recurrence, priority;

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
    finished_at = CASE WHEN @set_finished_at::bool  THEN @finished_at::timestamptz ELSE finished_at END,
    task_type   = CASE WHEN @set_task_type::bool     THEN @task_type::text         ELSE task_type END,
    recurrence  = CASE WHEN @clear_recurrence::bool THEN NULL WHEN @set_recurrence::bool THEN @recurrence::int ELSE recurrence END,
    priority    = CASE WHEN @set_priority::bool      THEN @priority::int           ELSE priority END
WHERE id = @id
RETURNING id, project_id, name, description, due_at, started_at, finished_at, task_type, recurrence, priority;

-- name: UpdateProject :one
UPDATE projects SET
    name        = CASE WHEN @set_name::bool        THEN @name::text             ELSE name END,
    description = CASE WHEN @set_description::bool  THEN @description::text      ELSE description END,
    due_at      = CASE WHEN @clear_due_at::bool THEN NULL WHEN @set_due_at::bool THEN @due_at::date ELSE due_at END,
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
SELECT t.id, t.name, t.project_id, pt.name AS project_name, t.task_type, t.recurrence, t.priority
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
-- effective_due_at = MIN(self.due_at, due_at of every transitive blocks-descendant).
-- A task is "hidden" iff it has at least one unfinished dep AND every unfinished dep is itself blocked.
WITH RECURSIVE
unfinished AS (
    SELECT t.id, t.due_at FROM tasks t WHERE t.finished_at IS NULL
),
blocks_closure(root_id, descendant_id, descendant_due) AS (
    SELECT u.id, u.id, u.due_at FROM unfinished u
    UNION
    SELECT bc.root_id, td.task_id, t2.due_at
    FROM blocks_closure bc
    JOIN task_dependencies td ON td.depends_on = bc.descendant_id
    JOIN tasks t2 ON t2.id = td.task_id AND t2.finished_at IS NULL
),
effective AS (
    SELECT root_id AS id, MIN(descendant_due) AS effective_due_at
    FROM blocks_closure GROUP BY root_id
),
task_blocked AS (
    SELECT u.id,
        EXISTS(SELECT 1 FROM task_dependencies td
               JOIN tasks t2 ON t2.id = td.depends_on
               WHERE td.task_id = u.id AND t2.finished_at IS NULL) AS blocked
    FROM unfinished u
),
task_hidden AS (
    SELECT u.id,
        EXISTS(SELECT 1 FROM task_dependencies td
               JOIN tasks t2 ON t2.id = td.depends_on
               WHERE td.task_id = u.id AND t2.finished_at IS NULL)
        AND NOT EXISTS(
            SELECT 1 FROM task_dependencies td
            JOIN tasks t2 ON t2.id = td.depends_on
            JOIN task_blocked tb ON tb.id = t2.id
            WHERE td.task_id = u.id AND t2.finished_at IS NULL AND NOT tb.blocked
        ) AS hidden
    FROM unfinished u
)
SELECT t.id, t.project_id, t.name, t.description,
    e.effective_due_at AS due_at,
    t.started_at, t.task_type, t.recurrence, t.priority,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks,
    tb.blocked
FROM tasks t
JOIN effective e ON e.id = t.id
JOIN task_blocked tb ON tb.id = t.id
JOIN task_hidden th ON th.id = t.id
WHERE t.finished_at IS NULL
  AND (sqlc.narg('min_priority')::int IS NULL OR t.priority <= sqlc.narg('min_priority')::int)
  AND NOT th.hidden
ORDER BY t.name;

-- name: GetProjectWithDescendants :many
WITH RECURSIVE project_tree AS (
    SELECT p.id, p.parent_id, p.name, p.description, p.due_at, p.started_at, p.finished_at,
        0 AS depth, ARRAY[p.id]::int[] AS path
    FROM projects p WHERE p.id = $1
    UNION ALL
    SELECT c.id, c.parent_id, c.name, c.description, c.due_at, c.started_at, c.finished_at,
        pt.depth + 1, pt.path || c.id
    FROM projects c
    JOIN project_tree pt ON c.parent_id = pt.id
),
project_direct_time AS (
    SELECT t.project_id, COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS direct_time
    FROM tasks t
    LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
    WHERE t.project_id IN (SELECT id FROM project_tree)
    GROUP BY t.project_id
)
SELECT pt.id, pt.parent_id, pt.name, pt.description, pt.due_at, pt.started_at, pt.finished_at, pt.depth,
    COALESCE((
        SELECT SUM(pdt.direct_time)::bigint
        FROM project_direct_time pdt
        JOIN project_tree d ON d.id = pdt.project_id
        WHERE pt.id = ANY(d.path)
    ), 0)::bigint AS time_spent
FROM project_tree pt
ORDER BY pt.depth, pt.due_at ASC NULLS LAST, pt.name;

-- name: GetTasksByProjectIDs :many
SELECT
    t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at, t.task_type, t.recurrence, t.priority,
    COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td2 JOIN tasks t2 ON t2.id = td2.depends_on WHERE td2.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td2 JOIN tasks t2 ON t2.id = td2.task_id WHERE td2.depends_on = t.id), '[]')::json AS blocks,
    EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked,
    COALESCE((SELECT json_agg(json_build_object('id', td.id, 'name', td.name, 'is_done', td.is_done) ORDER BY td.is_done ASC NULLS LAST, td.id) FROM todos td WHERE td.task_id = t.id), '[]')::json AS todos
FROM tasks t
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.project_id = ANY(@project_ids::int[])
GROUP BY t.id
ORDER BY
    CASE
        WHEN t.finished_at IS NOT NULL THEN 2
        WHEN t.started_at IS NOT NULL THEN 0
        ELSE 1
    END,
    t.due_at ASC NULLS LAST,
    t.name;

-- name: UpdateTodo :one
UPDATE todos SET
    task_id = CASE WHEN @set_task_id::bool THEN @task_id::int ELSE task_id END,
    name    = CASE WHEN @set_name::bool    THEN @name::text   ELSE name END,
    is_done = CASE WHEN @set_is_done::bool THEN @is_done::bool ELSE is_done END
WHERE id = @id
RETURNING id, task_id, name, is_done;

-- name: GetTasksByDueDate :many
-- Returns unfinished tasks that have a due_at (own or inherited from a blocked task) or whose project has one.
-- effective_due_at propagates backward via the "blocks" relation; hidden tasks are filtered out.
WITH RECURSIVE
unfinished AS (
    SELECT t.id, t.due_at FROM tasks t WHERE t.finished_at IS NULL
),
blocks_closure(root_id, descendant_id, descendant_due) AS (
    SELECT u.id, u.id, u.due_at FROM unfinished u
    UNION
    SELECT bc.root_id, td.task_id, t2.due_at
    FROM blocks_closure bc
    JOIN task_dependencies td ON td.depends_on = bc.descendant_id
    JOIN tasks t2 ON t2.id = td.task_id AND t2.finished_at IS NULL
),
effective AS (
    SELECT root_id AS id, MIN(descendant_due) AS effective_due_at
    FROM blocks_closure GROUP BY root_id
),
task_blocked AS (
    SELECT u.id,
        EXISTS(SELECT 1 FROM task_dependencies td
               JOIN tasks t2 ON t2.id = td.depends_on
               WHERE td.task_id = u.id AND t2.finished_at IS NULL) AS blocked
    FROM unfinished u
),
task_hidden AS (
    SELECT u.id,
        EXISTS(SELECT 1 FROM task_dependencies td
               JOIN tasks t2 ON t2.id = td.depends_on
               WHERE td.task_id = u.id AND t2.finished_at IS NULL)
        AND NOT EXISTS(
            SELECT 1 FROM task_dependencies td
            JOIN tasks t2 ON t2.id = td.depends_on
            JOIN task_blocked tb ON tb.id = t2.id
            WHERE td.task_id = u.id AND t2.finished_at IS NULL AND NOT tb.blocked
        ) AS hidden
    FROM unfinished u
)
SELECT
    t.id, t.name, t.description,
    e.effective_due_at AS due_at,
    t.started_at, t.task_type, t.recurrence, t.priority,
    p.id AS project_id, p.name AS project_name, p.due_at AS project_due_at,
    COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks,
    tb.blocked
FROM tasks t
JOIN effective e ON e.id = t.id
JOIN task_blocked tb ON tb.id = t.id
JOIN task_hidden th ON th.id = t.id
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.finished_at IS NULL
  AND (sqlc.narg('min_priority')::int IS NULL OR t.priority <= sqlc.narg('min_priority')::int)
  AND NOT th.hidden
  AND (e.effective_due_at IS NOT NULL OR p.due_at IS NOT NULL)
GROUP BY t.id, p.id, e.effective_due_at, tb.blocked
ORDER BY e.effective_due_at ASC NULLS LAST, p.due_at ASC NULLS LAST, t.name;

-- name: GetTaskByID :one
SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at, t.task_type, t.recurrence, t.priority,
    p.name AS project_name,
    COALESCE(SUM(EXTRACT(EPOCH FROM (te.finished_at - te.started_at)))::bigint, 0)::bigint AS time_spent,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name, 'due_at', t2.due_at) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.depends_on WHERE td.task_id = t.id AND t2.finished_at IS NULL), '[]')::json AS depends_on,
    COALESCE((SELECT json_agg(json_build_object('id', t2.id, 'name', t2.name) ORDER BY t2.name) FROM task_dependencies td JOIN tasks t2 ON t2.id = td.task_id WHERE td.depends_on = t.id), '[]')::json AS blocks,
    EXISTS(SELECT 1 FROM task_dependencies td3 JOIN tasks t3 ON t3.id = td3.depends_on WHERE td3.task_id = t.id AND t3.finished_at IS NULL) AS blocked,
    COALESCE((SELECT json_agg(json_build_object('id', td.id, 'name', td.name, 'is_done', td.is_done) ORDER BY td.is_done ASC NULLS LAST, td.id) FROM todos td WHERE td.task_id = t.id), '[]')::json AS todos
FROM tasks t
LEFT JOIN projects p ON p.id = t.project_id
LEFT JOIN time_entries te ON te.task_id = t.id AND te.finished_at IS NOT NULL
WHERE t.id = $1
GROUP BY t.id, p.name;

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
       t.name AS task_name, t.task_type, t.recurrence, t.priority, p.name AS project_name
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
-- Each entry is split at period boundaries (in @timezone) and the resulting
-- segments are summed per period. Empty periods in [start_at, end_at] are
-- zero-filled via the outer LEFT JOIN against generate_series.
WITH all_periods AS (
    SELECT period_start::date AS date
    FROM generate_series(@start_at::date, @end_at::date, ('1 ' || @frequency::text)::interval) AS period_start
),
entry_periods AS (
    SELECT te.started_at, te.finished_at, gs.period_start
    FROM time_entries te,
    LATERAL generate_series(
        date_trunc(@frequency::text, te.started_at AT TIME ZONE @timezone::text),
        date_trunc(@frequency::text, te.finished_at AT TIME ZONE @timezone::text),
        ('1 ' || @frequency::text)::interval
    ) AS gs(period_start)
    WHERE te.finished_at IS NOT NULL
      AND (te.started_at AT TIME ZONE @timezone::text)::date >= @start_at::date
      AND (te.started_at AT TIME ZONE @timezone::text)::date <= @end_at::date
),
sums AS (
    SELECT
        ep.period_start::date AS date,
        (SUM(EXTRACT(EPOCH FROM (
            LEAST(ep.finished_at, (ep.period_start + ('1 ' || @frequency::text)::interval) AT TIME ZONE @timezone::text)
            - GREATEST(ep.started_at, ep.period_start AT TIME ZONE @timezone::text)
        ))) / 3600)::REAL AS value
    FROM entry_periods ep
    GROUP BY ep.period_start
)
SELECT ap.date, COALESCE(s.value, 0)::REAL AS value
FROM all_periods ap
LEFT JOIN sums s ON s.date = ap.date
ORDER BY ap.date;

-- name: GetTimeEntriesByTaskID :many
WITH task_info AS (
    SELECT t.id, t.project_id, t.name, t.description, t.due_at, t.started_at, t.finished_at, t.task_type, t.recurrence, t.priority,
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
    ti.started_at AS task_started_at, ti.finished_at AS task_finished_at, ti.task_type, ti.recurrence, ti.priority, ti.time_spent,
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

-- name: TaskDependencyWouldCycle :one
-- Wraps the task_dependency_would_cycle SQL function (see migration 011).
-- Returns true if replacing @task_id's outgoing dep edges with @new_deps
-- would create a cycle.
SELECT task_dependency_would_cycle(@task_id::int, @new_deps::int[]) AS has_cycle;

-- name: TaskBlocksWouldCycle :one
-- Returns true if any of the @blocks tasks depending on @task_id would
-- create a cycle. Reuses task_dependency_would_cycle by checking each
-- (block -> task_id) edge in a single query.
SELECT EXISTS(
    SELECT 1 FROM unnest(@blocks::int[]) AS b(id)
    WHERE task_dependency_would_cycle(b.id, ARRAY[@task_id::int]::int[])
) AS has_cycle;

-- name: DeleteRemovedTaskBlocks :exec
DELETE FROM task_dependencies
WHERE depends_on = $1 AND NOT (task_id = ANY(@keep::int[]));

-- name: UpsertTaskBlocks :exec
INSERT INTO task_dependencies (task_id, depends_on)
SELECT t.id, $1
FROM unnest(@blocks::int[]) AS block_id
JOIN tasks t ON t.id = block_id AND t.finished_at IS NULL
WHERE array_length(@blocks::int[], 1) IS NOT NULL
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
    t.task_type, t.recurrence, t.priority,
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
