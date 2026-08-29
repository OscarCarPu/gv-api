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
| event_ref  | TEXT        | nullable, unique (partial, WHERE NOT NULL)           |
| commitment_id | INTEGER  | nullable, FK -> recurring_commitments.id ON DELETE SET NULL |
| created_at | TIMESTAMPTZ | NOT NULL, DEFAULT now()                              |
| updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() (touched by trigger)         |

**Indexes:**
- `idx_plan_blocks_date` on (`plan_date`, `started_at`)
- `idx_plan_blocks_task` on (`task_id`) WHERE `task_id IS NOT NULL`
- `idx_plan_blocks_event_ref` (UNIQUE) on (`event_ref`) WHERE `event_ref IS NOT NULL`

**Checks:**
- `ended_at > started_at`
- `length(label) BETWEEN 1 AND 200`

**Triggers:**
- `plan_blocks_touch_updated_at` (BEFORE UPDATE) sets `updated_at = now()`.

### recurring_commitments

| Column       | Type        | Constraints                                  |
|--------------|-------------|-----------------------------------------------|
| id           | SERIAL      | PRIMARY KEY                                    |
| task_id      | INTEGER     | NOT NULL, FK -> tasks.id ON DELETE CASCADE     |
| label        | TEXT        | NOT NULL, length 1–200                         |
| days_of_week | SMALLINT[]  | NOT NULL — values 0 (Sunday) to 6 (Saturday)   |
| start_time   | TIME        | NOT NULL                                       |
| end_time     | TIME        | NOT NULL, must be after `start_time`           |
| active       | BOOLEAN     | NOT NULL, DEFAULT true                         |
| created_at   | TIMESTAMPTZ | NOT NULL, DEFAULT now()                        |
| updated_at   | TIMESTAMPTZ | NOT NULL, DEFAULT now() (touched by trigger)   |

### recurring_commitment_skips

| Column        | Type    | Constraints                                                |
|---------------|---------|-------------------------------------------------------------|
| commitment_id | INTEGER | NOT NULL, FK -> recurring_commitments.id ON DELETE CASCADE  |
| skip_date     | DATE    | NOT NULL                                                     |

**Primary Key:** (`commitment_id`, `skip_date`)

## Relationships

```
tasks (1) --< (many) plan_blocks               [via task_id, nullable, ON DELETE SET NULL]
tasks (1) --< (many) recurring_commitments      [via task_id, NOT NULL, ON DELETE CASCADE]
recurring_commitments (1) --< (many) plan_blocks         [via commitment_id, nullable, ON DELETE SET NULL]
recurring_commitments (1) --< (many) recurring_commitment_skips  [ON DELETE CASCADE]
```

- `task_id` is nullable: a block with `task_id IS NULL` is a free-time block (e.g. "comer", "paseo") and stands alone with its `label`.
- A block with `task_id` set is a linked block; the UI uses the join with `tasks` to surface task state (`task_type`, `recurrence`, `started_at`, `finished_at`) for inline action buttons.
- `ON DELETE SET NULL` on the FK means deleting the linked task converts the block into a free-time block with the original `label` intact — the day's plan is never wiped by an unrelated task delete.
- `event_ref` links a plan_block to a calendar event (`instance_id` — see [data_models/calendar.md](calendar.md)). Purely a local reference: no FK to any calendar table, since a recurring event occurrence may never have a row of its own there.
- `commitment_id` marks a plan_block as generated from a `recurring_commitments` row. `ON DELETE SET NULL` means deleting the commitment leaves already-generated blocks in place as ordinary manual blocks.
- Plan blocks never write to `tasks` or `time_entries`. Reads only.

## Notes

- `plan_date` is redundant with `started_at::date` but stored separately so the index can be a plain B-tree on `(plan_date, started_at)` without timezone gymnastics. The service derives `plan_date` from `started_at` (UTC date) on every insert/update. It only reflects the **start** day — a multi-day block's overlap and busy-hours computations use the `[started_at, ended_at)` interval directly, not `plan_date`, precisely because a multi-day block's later days aren't `plan_date`.
- There is no overlap exclusion constraint at the DB level; the service performs a `COUNT(*)` overlap check on every Create/Update and returns `400` (`ErrOverlap`) if any other block anywhere has an overlapping `[started_at, ended_at)` interval (not scoped to `plan_date`, so a multi-day block is checked against its whole span).
- `recurring_commitment_skips` exists so that deleting (or moving to a different day) a commitment-generated block doesn't get silently recreated the next time the range is regenerated — see [business_logic/plan.md](../business_logic/plan.md).
