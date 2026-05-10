-- name: ListPlanBlocksByDate :many
SELECT
    pb.id,
    pb.plan_date,
    pb.started_at,
    pb.ended_at,
    pb.task_id,
    pb.label,
    pb.note,
    t.name        AS task_name,
    t.task_type   AS task_type,
    t.recurrence  AS task_recurrence,
    t.started_at  AS task_started_at,
    t.finished_at AS task_finished_at
FROM plan_blocks pb
LEFT JOIN tasks t ON t.id = pb.task_id
WHERE pb.plan_date = $1
ORDER BY pb.started_at;

-- name: GetPlanBlock :one
SELECT
    pb.id,
    pb.plan_date,
    pb.started_at,
    pb.ended_at,
    pb.task_id,
    pb.label,
    pb.note,
    t.name        AS task_name,
    t.task_type   AS task_type,
    t.recurrence  AS task_recurrence,
    t.started_at  AS task_started_at,
    t.finished_at AS task_finished_at
FROM plan_blocks pb
LEFT JOIN tasks t ON t.id = pb.task_id
WHERE pb.id = $1;

-- name: GetTaskName :one
SELECT name FROM tasks WHERE id = $1;

-- name: CreatePlanBlock :one
INSERT INTO plan_blocks (plan_date, started_at, ended_at, task_id, label, note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, plan_date, started_at, ended_at, task_id, label, note;

-- name: UpdatePlanBlock :one
UPDATE plan_blocks SET
    started_at = CASE WHEN @set_started_at::bool THEN @started_at::timestamptz ELSE started_at END,
    ended_at   = CASE WHEN @set_ended_at::bool   THEN @ended_at::timestamptz   ELSE ended_at   END,
    plan_date  = CASE WHEN @set_plan_date::bool  THEN @plan_date::date         ELSE plan_date  END,
    task_id    = CASE WHEN @clear_task_id::bool  THEN NULL
                      WHEN @set_task_id::bool    THEN @task_id::int            ELSE task_id    END,
    label      = CASE WHEN @set_label::bool      THEN @label::text             ELSE label      END,
    note       = CASE WHEN @clear_note::bool     THEN NULL
                      WHEN @set_note::bool       THEN @note::text              ELSE note       END
WHERE id = @id
RETURNING id, plan_date, started_at, ended_at, task_id, label, note;

-- name: CountOverlappingPlanBlocks :one
SELECT COUNT(*) FROM plan_blocks
WHERE plan_date = @plan_date
  AND started_at < @ended_at
  AND ended_at   > @started_at
  AND (NOT @has_exclude_id::bool OR id <> @exclude_id::int);

-- name: DeletePlanBlock :exec
DELETE FROM plan_blocks WHERE id = $1;
