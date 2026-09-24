-- A task's effective due date: the earliest due_at among itself and every unfinished task that
-- transitively depends on it (a task blocking something due sooner inherits that deadline).
-- One definition for every query that needs it, instead of the same recursive CTE in each.
CREATE OR REPLACE VIEW task_effective_due AS
WITH RECURSIVE blocks_closure(root_id, descendant_due, descendant_id) AS (
    SELECT t.id, t.due_at, t.id FROM tasks t WHERE t.finished_at IS NULL
    UNION
    SELECT bc.root_id, t2.due_at, td.task_id
    FROM blocks_closure bc
    JOIN task_dependencies td ON td.depends_on = bc.descendant_id
    JOIN tasks t2 ON t2.id = td.task_id AND t2.finished_at IS NULL
)
SELECT root_id AS task_id, MIN(descendant_due) AS effective_due_at
FROM blocks_closure
GROUP BY root_id;
