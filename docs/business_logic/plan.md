# Plan

### Description

Domain for the user's day plan: a list of time-boxed blocks scheduled across a single day. Each block is either a **linked block** (points at an existing task) or a **free-time block** (carries only a label such as "comer", "thing 1"). The plan is read-only with respect to `tasks` and `time_entries` — it never mutates them — but it surfaces task state so the UI can render the same Empezar/Acabar/Renovar and Iniciar/Asignar shortcuts that exist on the task lists.

### States / Lifecycle

A plan block has no lifecycle of its own. It is created, optionally edited, and deleted. Whether the underlying task has been started or finished is read live from `tasks` on every fetch — the block does not cache it.

### Business Rules

**Block kind (linked vs. free)**
- `task_id` set → linked block. The block "points at" a task. UI uses the joined task state to render action buttons.
- `task_id` unset → free-time block. Renders only its `label`. No action buttons.
- `label` is always required (CHECK `length(label) BETWEEN 1 AND 200`). For linked blocks, the service auto-fills it with the task's name on create when the caller omits it; the caller can also override with a custom label that does not affect the task.
- A linked block whose task is later deleted survives as a free-time block (`task_id` becomes NULL via `ON DELETE SET NULL`); the original `label` is preserved.

**Independence from tasks**
- The plan service only reads from `tasks` (joins for `task_name`/`task_type`/`recurrence`/`started_at`/`finished_at`, plus `GetTaskName` for label hydration on Create).
- The plan service never writes to `tasks` or `time_entries`.
- The UI's ▶ Empezar / ✓ Acabar / 🔄 Renovar buttons on a linked block call the existing `/tasks/{id}` PATCH endpoint directly — the plan layer is not involved.
- The UI's ▶ Iniciar / Asignar timer button calls the existing `/tasks/time-entries` POST endpoint directly.

**Overlap**
- Two blocks on the same `plan_date` may not overlap in time. Overlap is defined as `existing.started_at < new.ended_at AND existing.ended_at > new.started_at`.
- Enforced at the service layer via `CountOverlappingPlanBlocks`. On Create the new block is checked against all other blocks. On Update the check excludes the row being updated (so shifting a block's bounds does not collide with itself).
- A violation returns `400 Bad Request` with the literal message `plan block overlaps with an existing one`.

**Time bounds**
- `ended_at > started_at` (CHECK at DB level + early-return validation in the service).
- On Update, when only one of `started_at` / `ended_at` is provided, the service fetches the persisted side and validates the combined interval before issuing the UPDATE.

**Today's totals and budget**
- `GET /plan/today` returns three things:
  1. `blocks`: today's plan_blocks ordered by `started_at`.
  2. `totals`: `task_seconds` (sum of durations of linked blocks) and `free_seconds` (sum of free blocks).
  3. `budget`: the same payload `GET /tasks/time-entries/summary` returns — `today`, `week`, `daily_target_seconds`, `weekly_target_seconds`, `pace`. The plan service delegates to `tasks.Service.GetTimeEntrySummary`, so the plan and the rest of the app share a single budget calculation.
- "Today" is the local-tz date of `now()` (server-configured timezone, same convention as the task summary).

**Free-time and the daily target**
- The daily target compares against actual time-entries (and, in the UI, the planned-future-tasks estimate). Free-time blocks are deliberately excluded from the target — they are visualized in their own row.

### Validations

**Create plan block (`POST /plan/blocks`)**
- `started_at` and `ended_at` required, `ended_at` must be strictly after `started_at`.
- Either `task_id` or `label` must be present (the service auto-fills `label` from the task name when only `task_id` is given).
- `label`, when provided, is trimmed and must be 1–200 chars after trim.
- The new block must not overlap any other block on the same `plan_date`.
- Returns 400 on validation errors (`ErrInvalidTimeRange`, `ErrLabelRequired`, `ErrLabelTooLong`, `ErrOverlap`, `ErrTaskNotFound`).

**Update plan block (`PUT /plan/blocks/{id}`)**
- Same time/label/overlap rules as Create. Time bounds are validated against the persisted state when only one side is provided.
- `clear_task: true` removes the link (the block becomes free-time); `task_id` cannot also be set in the same request.
- `clear_note: true` clears the note; `note` cannot also be set in the same request.
- Returns 404 if the block does not exist.

**Delete plan block (`DELETE /plan/blocks/{id}`)**
- No body. 204 on success. Hard delete (no soft-delete column).

### Side Effects

- **On task delete** → plan_blocks pointing at that task have their `task_id` set to `NULL` (FK `ON DELETE SET NULL`). Blocks survive as free-time entries with the original label.
- **On plan_block update** → `updated_at` is bumped by the `plan_blocks_touch_updated_at` trigger. Not surfaced in the API but useful for debugging.

### Decisions / Why

- **Why a separate table instead of fields on `tasks`**: the user's example day-plan included free-time blocks (e.g. "comer", "thing 1") that don't map to any task, and a single task can be planned across multiple non-contiguous slots. Both rule out task-as-plan.
- **`ON DELETE SET NULL` rather than CASCADE**: the day's plan is *intent*, not an attribute of the task. Deleting an unrelated task should not silently delete a planned hour from your day. Conversion to free-time keeps the slot intact so the user notices and re-plans.
- **Independence from `tasks` / `time_entries`**: the goal of the plan is *visualization* — see the day at a glance, decide if you need less free time or more focus blocks. Mutating tasks/time_entries from plan blocks would conflate intent with execution and create surprising side effects when editing or deleting a block.
- **Overlap as a service-layer check, not a Postgres EXCLUDE constraint**: a `tstzrange` exclusion constraint would push the check into the DB but adds operational complexity (extension required, error surface less ergonomic). At the volumes of a personal-use app a `COUNT(*)` is fine. Revisit if blocks ever scale to multiple users or many days at once.
- **Budget computation lives in `tasks` package, reused by `plan`**: the daily/weekly target math (waking-hour-weighted distribution against an 80h/week goal) is a single source of truth in `tasks/budget.go`. The plan endpoint includes the same `TimeEntrySummaryResponse` it would get from `/tasks/time-entries/summary` rather than recomputing.
