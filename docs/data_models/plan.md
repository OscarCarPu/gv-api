# Plan - Data Models

## Tables

### plan_blocks

| Column     | Type        | Constraints                                          |
|------------|-------------|------------------------------------------------------|
| id         | SERIAL      | PRIMARY KEY                                          |
| plan_date  | DATE        | NOT NULL                                             |
| started_at | TIMESTAMPTZ | NOT NULL                                             |
| ended_at   | TIMESTAMPTZ | NOT NULL                                             |
| task_id    | INTEGER     | nullable, FK -> tasks.id ON DELETE SET NULL          |
| label      | TEXT        | NOT NULL, length 1–200                               |
| note       | TEXT        | nullable                                             |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now()                              |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() (touched by trigger)         |

**Indexes:**
- `idx_plan_blocks_date` on (`plan_date`, `started_at`)
- `idx_plan_blocks_task` on (`task_id`) WHERE `task_id IS NOT NULL`

**Checks:**
- `ended_at > started_at`
- `length(label) BETWEEN 1 AND 200`

**Triggers:**
- `plan_blocks_touch_updated_at` (BEFORE UPDATE) sets `updated_at = now()`.

## Relationships

```
tasks (1) --< (many) plan_blocks   [via task_id, nullable, ON DELETE SET NULL]
```

- `task_id` is nullable: a block with `task_id IS NULL` is a free-time block (e.g. "comer", "paseo") and stands alone with its `label`.
- A block with `task_id` set is a linked block; the UI uses the join with `tasks` to surface task state (`task_type`, `recurrence`, `started_at`, `finished_at`) for inline action buttons.
- `ON DELETE SET NULL` on the FK means deleting the linked task converts the block into a free-time block with the original `label` intact — the day's plan is never wiped by an unrelated task delete.
- Plan blocks never write to `tasks` or `time_entries`. Reads only.

## Notes

- `plan_date` is redundant with `started_at::date` but stored separately so the index can be a plain B-tree on `(plan_date, started_at)` without timezone gymnastics. The service derives `plan_date` from `started_at` (UTC date) on every insert/update.
- There is no overlap exclusion constraint at the DB level; the service performs a `COUNT(*)` overlap check on every Create/Update and returns `400` (`ErrOverlap`) if any other block on the same `plan_date` has an overlapping `[started_at, ended_at)` interval.
