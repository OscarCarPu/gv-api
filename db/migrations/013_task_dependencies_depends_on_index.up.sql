-- The PK on task_dependencies(task_id, depends_on) covers task_id-prefixed
-- lookups, but several subqueries in tasks.sql filter by depends_on alone:
--   * GetUnfinishedTasks / GetTasksByDueDate / GetTaskByID / GetTasksByProjectIDs
--     all run "json_agg(... blocks)" via JOIN ... WHERE td.depends_on = t.id
--     and "EXISTS blocked" via WHERE td.depends_on = t.id.
-- Without this index those subqueries seq-scan task_dependencies on every row.
CREATE INDEX IF NOT EXISTS idx_task_dependencies_depends_on
    ON task_dependencies(depends_on);
