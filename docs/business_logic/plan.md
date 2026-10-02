# Plan

### Description

The user's day plan: time-boxed blocks. A block is **linked** (points at a task), **event-linked** (`event_ref`), or **free-time** (only a label, e.g. "comer"). The plan never writes to `tasks` or `time_entries`; it only reads task state so the UI can show the task shortcuts.

### Business Rules

**Block kind**
- `task_id` set → linked block; the UI renders the joined task state and its action buttons, which call the task endpoints directly.
- `task_id` unset → free-time block, just its `label`.
- `label` is always required (1–200 chars). On create, a linked block without one gets the task's name.
- If the task is deleted, the block survives as free-time (`ON DELETE SET NULL`) with its label.

**Overlap**
- No two blocks may overlap: `existing.started_at < new.ended_at AND existing.ended_at > new.started_at`, over the real interval, not `plan_date`.
- Checked in the service (`CountOverlappingPlanBlocks`); updates exclude the row itself. Violations answer `400 plan block overlaps with an existing one`.

**Multi-day blocks**
- `[started_at, ended_at)` may span several days. `plan_date` is only the start day; busy hours and `GET /plan/range` use the real interval, split across every day it touches.

**Linking to a calendar event (`event_ref`)**
- `event_ref` is a calendar event's `instance_id` (`12`, or `12@2026-08-20T07:00:00Z` for an occurrence). At most one block per event.
- Editing the event's times moves the linked block (a passive follow, without overlap or label validation). Deleting the event, or changing its identity (a `scope=following` split, a move to another account), clears `event_ref`; the block is kept. See [calendar](calendar.md).
- Events themselves never count toward capacity; only blocks do.

**Capacity**
- `GET /capacity/free-busy` is a daily capacity constant minus busy hours. A block is busy if it has a `task_id` or an `event_ref`; label-only blocks never count.
- Feeds task urgency; see [tasks](tasks.md#estimate-and-urgency-due-soon).

**Recurring commitments**
- A `recurring_commitments` row (task, label, `days_of_week`, daily `start_time`/`end_time`) is a template. Reading a range (`GET /plan/range`, or capacity) first generates `plan_blocks` for each matching date that has no block for that commitment and no skip. Idempotent, no worker.
- Concurrent reads are made safe by the partial unique index `plan_blocks_commitment_date_uidx` and `ON CONFLICT DO NOTHING` in `CreateGeneratedPlanBlock`.
- Generated blocks are ordinary blocks and can be edited or deleted.
- **Deleting** a generated block records a skip for its date so it is not regenerated.
- **Moving** a generated block to another day detaches it (`commitment_id` cleared) and records a skip for the original date.
- Generation skips the overlap check, so clashing commitments or manual blocks can overlap.
- **Changing** a commitment's days or times re-applies them to its generated blocks from today on: blocks on dropped weekdays are deleted, the rest are moved.
- **Pausing** (`active: false`) or **deleting** a commitment removes its blocks that have not started yet. Past blocks stay as history. Resuming regenerates them on the next read; skips are kept.

**Time bounds**
- `ended_at > started_at` (DB CHECK and service). On update with only one side, the persisted other side is used.

**Today's totals and budget**
- `GET /plan/today` returns today's `blocks` (by `started_at`), `totals` (`task_seconds` for linked blocks, `free_seconds` for free ones) and `budget`, which is `tasks.Service.GetTimeEntrySummary` (same as `GET /tasks/time-entries/summary`).
- "Today" is the server timezone's date. Free-time blocks are excluded from the daily target.

### Validations

**Create plan block (`POST /plan/blocks`)**
- `started_at` and `ended_at` required, `ended_at` after `started_at`.
- `task_id` or `label` required; `label` is trimmed, 1–200 chars.
- Must not overlap any other block.
- 400 on `ErrInvalidTimeRange`, `ErrLabelRequired`, `ErrLabelTooLong`, `ErrOverlap`, `ErrTaskNotFound`.

**Update plan block (`PUT /plan/blocks/{id}`)**
- Same rules as Create.
- `clear_task: true` makes it free-time (not combinable with `task_id`); `clear_note: true` clears the note (not combinable with `note`).
- 404 if the block does not exist.

**Delete plan block (`DELETE /plan/blocks/{id}`)**
- Hard delete, 204. Records a skip first if the block was generated.

**Create commitment (`POST /plan/commitments`)**
- `task_id` required and must exist; `label` 1–200 chars after trim.
- `days_of_week` non-empty (0=Sunday..6=Saturday).
- `start_time`/`end_time` required (`HH:MM`), end after start.

**Update commitment (`PUT /plan/commitments/{id}`)**
- Same rules, all fields optional. 404 if not found.

### Side Effects

- **Task deleted** → its blocks become free-time.
- **Block updated** → `updated_at` bumped by `plan_blocks_touch_updated_at`.

### Decisions / Why

- **Separate table, not task fields**: free-time blocks have no task, and one task can be planned in several slots.
- **`ON DELETE SET NULL`**: deleting a task should not silently remove planned time from the day.
- **Plan never mutates tasks**: the plan is intent, not execution.
- **Overlap in the service, not an EXCLUDE constraint**: a `COUNT(*)` is enough at this scale.
- **Budget reused from `tasks/budget.go`**: one source of truth for the daily/weekly targets.
- **Only blocks count as busy, not events**: most calendar events are informational; only deliberately scheduled time reduces capacity.
- **Commitments materialize rows instead of a recurrence engine**: generated blocks reuse every existing block operation.
