-- name: ListPlanBlocksByDate :many
SELECT
    pb.id,
    pb.plan_date,
    pb.started_at,
    pb.ended_at,
    pb.task_id,
    pb.label,
    pb.note,
    pb.event_ref,
    pb.commitment_id,
    t.name        AS task_name,
    t.task_type   AS task_type,
    t.recurrence  AS task_recurrence,
    t.started_at  AS task_started_at,
    t.finished_at AS task_finished_at
FROM plan_blocks pb
LEFT JOIN tasks t ON t.id = pb.task_id
WHERE pb.plan_date = $1
ORDER BY pb.started_at;

-- name: ListPlanBlocksByDateRange :many
-- Any block whose interval touches [from, to), so multi-day blocks show on every day.
SELECT
    pb.id,
    pb.plan_date,
    pb.started_at,
    pb.ended_at,
    pb.task_id,
    pb.label,
    pb.note,
    pb.event_ref,
    pb.commitment_id,
    t.name        AS task_name,
    t.task_type   AS task_type,
    t.recurrence  AS task_recurrence,
    t.started_at  AS task_started_at,
    t.finished_at AS task_finished_at
FROM plan_blocks pb
LEFT JOIN tasks t ON t.id = pb.task_id
WHERE pb.started_at < @to_date::timestamptz AND pb.ended_at > @from_date::timestamptz
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
    pb.event_ref,
    pb.commitment_id,
    t.name        AS task_name,
    t.task_type   AS task_type,
    t.recurrence  AS task_recurrence,
    t.started_at  AS task_started_at,
    t.finished_at AS task_finished_at
FROM plan_blocks pb
LEFT JOIN tasks t ON t.id = pb.task_id
WHERE pb.id = $1;

-- name: GetPlanBlockByEventRef :one
SELECT id, plan_date, started_at, ended_at, task_id, label, note, event_ref, commitment_id
FROM plan_blocks WHERE event_ref = @event_ref;

-- name: GetTaskName :one
SELECT name FROM tasks WHERE id = $1;

-- name: CreatePlanBlock :one
INSERT INTO plan_blocks (plan_date, started_at, ended_at, task_id, label, note, event_ref, commitment_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, plan_date, started_at, ended_at, task_id, label, note, event_ref, commitment_id;

-- name: CreateGeneratedPlanBlock :one
-- Returns no row when a concurrent call already generated the occurrence.
INSERT INTO plan_blocks (plan_date, started_at, ended_at, task_id, label, commitment_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (commitment_id, plan_date) WHERE commitment_id IS NOT NULL DO NOTHING
RETURNING id, plan_date, started_at, ended_at, task_id, label, note, event_ref, commitment_id;

-- name: UpdatePlanBlock :one
UPDATE plan_blocks SET
    started_at    = CASE WHEN @set_started_at::bool THEN @started_at::timestamptz ELSE started_at END,
    ended_at      = CASE WHEN @set_ended_at::bool   THEN @ended_at::timestamptz   ELSE ended_at   END,
    plan_date     = CASE WHEN @set_plan_date::bool  THEN @plan_date::date         ELSE plan_date  END,
    task_id       = CASE WHEN @clear_task_id::bool  THEN NULL
                        WHEN @set_task_id::bool    THEN @task_id::int            ELSE task_id    END,
    label         = CASE WHEN @set_label::bool      THEN @label::text             ELSE label      END,
    note          = CASE WHEN @clear_note::bool     THEN NULL
                        WHEN @set_note::bool       THEN @note::text              ELSE note       END,
    commitment_id = CASE WHEN @clear_commitment_id::bool THEN NULL ELSE commitment_id END
WHERE id = @id
RETURNING id, plan_date, started_at, ended_at, task_id, label, note, event_ref, commitment_id;

-- name: UpdatePlanBlockTimes :exec
UPDATE plan_blocks SET started_at = @started_at, ended_at = @ended_at, plan_date = @plan_date
WHERE id = @id;

-- name: ClearPlanBlockEventRef :exec
UPDATE plan_blocks SET event_ref = NULL WHERE id = @id;

-- name: CountOverlappingPlanBlocks :one
-- Plain interval overlap, no plan_date filter, so multi-day blocks conflict on every day.
SELECT COUNT(*) FROM plan_blocks
WHERE started_at < @ended_at
  AND ended_at   > @started_at
  AND (NOT @has_exclude_id::bool OR id <> @exclude_id::int);

-- name: DeletePlanBlock :exec
DELETE FROM plan_blocks WHERE id = $1;

-- name: DeletePlanBlocksEndingAfter :exec
DELETE FROM plan_blocks WHERE ended_at >= $1;

-- name: SumBusyHoursByDate :many
-- Busy hours per local day, splitting multi-day blocks. Blocks with a task_id or event_ref
-- count; label-only blocks do not. @to_date is inclusive: `- interval '1 day'` inline trips
-- sqlc's rewriter, so the caller decrements.
WITH tz_blocks AS (
    SELECT
        (pb.started_at AT TIME ZONE @timezone::text) AS local_start,
        (pb.ended_at AT TIME ZONE @timezone::text) AS local_end
    FROM plan_blocks pb
    WHERE pb.task_id IS NOT NULL OR pb.event_ref IS NOT NULL
),
days AS (
    SELECT generate_series(@from_date::date, @to_date::date, interval '1 day') AS day
)
SELECT
    days.day::date AS day,
    SUM(
        GREATEST(0, EXTRACT(EPOCH FROM (
            LEAST(tz.local_end, days.day + interval '1 day') - GREATEST(tz.local_start, days.day)
        )) / 3600.0)
    )::numeric AS hours
FROM days
JOIN tz_blocks tz ON tz.local_start < days.day + interval '1 day' AND tz.local_end > days.day
GROUP BY days.day
ORDER BY days.day;

-- name: SumPlannedHoursByTask :many
-- Hours still ahead in the plan for each task (ended_at > @from_ts).
SELECT task_id,
       SUM(EXTRACT(EPOCH FROM (ended_at - started_at)) / 3600.0)::numeric AS hours
FROM plan_blocks
WHERE task_id = ANY(@task_ids::int[]) AND ended_at > @from_ts::timestamptz
GROUP BY task_id;

-- name: ListPlanBlocksByCommitment :many
SELECT plan_date FROM plan_blocks
WHERE commitment_id = @commitment_id AND plan_date >= @from_date::date AND plan_date < @to_date::date;

-- name: ListRecurringCommitmentSkips :many
SELECT skip_date FROM recurring_commitment_skips
WHERE commitment_id = @commitment_id AND skip_date >= @from_date::date AND skip_date < @to_date::date;

-- name: InsertRecurringCommitmentSkip :exec
INSERT INTO recurring_commitment_skips (commitment_id, skip_date)
VALUES (@commitment_id, @skip_date) ON CONFLICT DO NOTHING;

-- name: ListActiveCommitments :many
SELECT * FROM recurring_commitments WHERE active ORDER BY id;

-- name: ListCommitments :many
SELECT rc.*, t.name AS task_name FROM recurring_commitments rc
JOIN tasks t ON t.id = rc.task_id
ORDER BY rc.id;

-- name: CreateCommitment :one
INSERT INTO recurring_commitments (task_id, label, days_of_week, start_time, end_time)
VALUES (@task_id, @label, @days_of_week, @start_time, @end_time)
RETURNING *;

-- name: UpdateCommitment :one
UPDATE recurring_commitments SET
    label        = CASE WHEN @set_label::bool        THEN @label::text        ELSE label        END,
    days_of_week = CASE WHEN @set_days_of_week::bool THEN @days_of_week::smallint[] ELSE days_of_week END,
    start_time   = CASE WHEN @set_start_time::bool   THEN @start_time::time   ELSE start_time   END,
    end_time     = CASE WHEN @set_end_time::bool     THEN @end_time::time     ELSE end_time     END,
    active       = CASE WHEN @set_active::bool       THEN @active::bool       ELSE active       END
WHERE id = @id
RETURNING *;

-- name: DeleteCommitment :exec
DELETE FROM recurring_commitments WHERE id = $1;

-- name: RescheduleCommitmentBlocks :exec
-- Re-applies a commitment's start/end, as wall-clock time in @timezone, to its generated
-- blocks from @from_date on.
UPDATE plan_blocks SET
    started_at = (plan_date + @start_time::time) AT TIME ZONE @timezone::text,
    ended_at   = (plan_date + @end_time::time)   AT TIME ZONE @timezone::text
WHERE commitment_id = @commitment_id AND plan_date >= @from_date::date;

-- name: DeleteCommitmentBlocksOffDays :exec
-- Generated blocks from @from_date on whose weekday is no longer in the commitment's days.
DELETE FROM plan_blocks
WHERE commitment_id = @commitment_id
  AND plan_date >= @from_date::date
  AND NOT (EXTRACT(DOW FROM plan_date)::smallint = ANY(@days_of_week::smallint[]));

-- name: DeleteFutureCommitmentBlocks :exec
-- Generated blocks that have not started yet. Used when a commitment is paused or deleted.
DELETE FROM plan_blocks
WHERE commitment_id = @commitment_id AND started_at >= @from_time::timestamptz;
