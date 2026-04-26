-- task_dependency_would_cycle returns true if adding edges
-- (task_id_in -> d for each d in new_deps) would create a cycle in the
-- task dependency graph. Walks existing edges starting from each new dep,
-- excluding task_id_in's own outgoing edges (which are about to be replaced
-- by the caller). A cycle exists iff task_id_in is reachable from any new dep.
CREATE OR REPLACE FUNCTION task_dependency_would_cycle(
    task_id_in integer,
    new_deps integer[]
) RETURNS boolean
LANGUAGE sql
STABLE
AS $$
    WITH RECURSIVE reach(node) AS (
        SELECT * FROM unnest(new_deps)
        UNION
        SELECT td.depends_on
        FROM task_dependencies td, reach r
        WHERE td.task_id = r.node AND td.task_id != task_id_in
    )
    SELECT EXISTS(SELECT 1 FROM reach r WHERE r.node = task_id_in);
$$;
