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
| commitment_id | INTEGER  | nullable, FK -> recurring_commitments.id ON DELETE SET NULL, unique with plan_date (partial, WHERE NOT NULL) |
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

- `task_id IS NULL` is a free-time block, standing alone with its `label`.
- A linked block's task state (`task_type`, `recurrence`, `started_at`, `finished_at`) is joined for the UI.
- Deleting the task turns the block into a free-time block with its `label`.
- `event_ref` links a block to a calendar event's `instance_id` ([calendar](calendar.md)). No FK: an occurrence may have no row of its own.
- `commitment_id` marks a block generated from a commitment. Pausing or deleting the commitment removes its unstarted blocks; past ones stay (the FK then nulls `commitment_id`). The partial unique index on `(commitment_id, plan_date)` stops concurrent reads generating an occurrence twice.

## Notes

- `plan_date` is the start day (UTC date of `started_at`), stored for a plain B-tree index on `(plan_date, started_at)`. Overlap and busy hours use `[started_at, ended_at)`, not `plan_date`.
- Overlap is a service-side `COUNT(*)` check, not a DB constraint.
- `recurring_commitment_skips` keeps a deleted or moved generated block from being regenerated ([business logic](../business_logic/plan.md)).
