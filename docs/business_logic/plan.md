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
- No two blocks anywhere may overlap in time. Overlap is defined as `existing.started_at < new.ended_at AND existing.ended_at > new.started_at` — a plain interval check, not scoped to `plan_date`, so a multi-day block (see below) is checked against its whole span, not just its start day.
- Enforced at the service layer via `CountOverlappingPlanBlocks`. On Create the new block is checked against all other blocks. On Update the check excludes the row being updated (so shifting a block's bounds does not collide with itself).
- A violation returns `400 Bad Request` with the literal message `plan block overlaps with an existing one`.

**Multi-day blocks**
- A block's `[started_at, ended_at)` may span more than one calendar day (e.g. a 2-day festival). Nothing prevents this — `ended_at > started_at` is the only time constraint.
- Every computation that reasons about "which day(s) does this block touch" (busy-hours totals, `GET /plan/range`) does so against the real interval, splitting a multi-day block proportionally across every day it overlaps — never against the stored `plan_date`, which only ever reflects the start day.

**Linking to a calendar event (`event_ref`)**
- A plan_block may carry `event_ref` — the same `instance_id` a calendar event is addressed by (`12`, or `12@2026-08-20T07:00:00Z` for a recurring occurrence). At most one plan_block may reference a given `event_ref` (partial unique index).
- This is how "convert this event into a real commitment" works: the caller creates a task (or picks an existing one, or none at all — an event-linked block does not require a task), then creates a plan_block with `event_ref` set to the event's `instance_id` and the block's hours pre-filled from the event.
- Once linked, editing the event's `starts_at`/`ends_at` in the calendar domain re-synchronizes the linked block's times automatically (a passive follow, not a user edit of the plan — no overlap/label revalidation). If the event is deleted, or restructured such that its identity changes (a recurring series split via `scope=following`, or moved to another Google account), the link is dropped (`event_ref` cleared) rather than left dangling or the plan_block deleted — see [business_logic/calendar.md](calendar.md).
- The calendar event itself is never required to have a task or to become "real" — most events stay purely informational. Only a block that actually exists (via this link, or created directly) counts toward capacity (see below).

**Capacity (free/busy) and how it reads plan_blocks**
- The `capacity` domain (`GET /capacity/free-busy`) reports a flat daily capacity constant minus "busy" hours per day. A plan_block counts as busy for a day if it has `task_id IS NOT NULL` **or** `event_ref IS NOT NULL` — a block that is neither (a plain hand-written label, same as the free-time blocks above) never counts, matching how free-time blocks were already excluded from the daily target.
- A multi-day block's hours are split across every day it touches (`GREATEST`/`LEAST` clamp against each day's midnight-to-midnight bounds), not attributed entirely to its `plan_date`.
- See [business_logic/tasks.md](tasks.md#estimate-and-urgency-due-soon) for how this feeds task urgency, and the capacity API doc for the endpoint itself.

**Recurring commitments (generated blocks)**
- A `recurring_commitments` row (task + label + `days_of_week` + a daily `start_time`/`end_time`) is a *template*, not a recurrence engine: reading any date range (`GET /plan/range`, or internally when computing capacity) first materializes real `plan_blocks` rows for every date in that range whose weekday matches, that don't already have a row for that `(commitment_id, date)` pair and aren't in `recurring_commitment_skips`. This is idempotent — safe to run on every read, no background worker.
- That existing-row check is a plain read before the insert, so two range reads covering the same date racing against each other can both pass it before either commits. The actual guarantee is a partial unique index, `plan_blocks_commitment_date_uidx` on `(commitment_id, plan_date) WHERE commitment_id IS NOT NULL`, plus a dedicated insert (`CreateGeneratedPlanBlock`, `ON CONFLICT ... DO NOTHING`) used only by generation — a losing concurrent insert is silently skipped rather than erroring or duplicating. Manually-created blocks never set `commitment_id`, so this constraint never affects them.
- Generated blocks are ordinary `plan_blocks` rows (with `commitment_id` set) — they can be edited or deleted exactly like a manually-created block.
- **Deleting** a commitment-generated block registers a skip (`recurring_commitment_skips`) for that date, so the next range read does not recreate it.
- **Moving** a commitment-generated block to a different day (editing `started_at`/`ended_at` such that the resulting `plan_date` changes) detaches it from the commitment (`commitment_id` cleared) *and* registers a skip for the original date — otherwise the moved block and a freshly regenerated one for the original date would both exist.
- Generation calls the repository directly, bypassing the overlap check — two commitments with clashing hours, or a commitment landing on a pre-existing manual block, silently produce overlapping blocks. Accepted for now: these are auto-generated defaults, always editable afterward.
- Deleting a `recurring_commitments` row does not delete its already-generated blocks (`ON DELETE SET NULL` on `commitment_id`) — they simply stop being regenerated/tracked and become ordinary manual blocks.

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
- No body. 204 on success. Hard delete (no soft-delete column). Registers a commitment skip first if the block was commitment-generated (see above).

**Create commitment (`POST /plan/commitments`)**
- `task_id` required, must reference an existing task.
- `label` required after trim, 1–200 chars.
- `days_of_week` required, must not be empty (0=Sunday..6=Saturday, matching `time.Weekday`).
- `start_time`/`end_time` required (`HH:MM`), `end_time` must be after `start_time`.

**Update commitment (`PUT /plan/commitments/{id}`)**
- Same field rules as Create, all optional (PATCH semantics). Returns 404 if not found.

### Side Effects

- **On task delete** → plan_blocks pointing at that task have their `task_id` set to `NULL` (FK `ON DELETE SET NULL`). Blocks survive as free-time entries with the original label.
- **On plan_block update** → `updated_at` is bumped by the `plan_blocks_touch_updated_at` trigger. Not surfaced in the API but useful for debugging.

### Decisions / Why

- **Why a separate table instead of fields on `tasks`**: the user's example day-plan included free-time blocks (e.g. "comer", "thing 1") that don't map to any task, and a single task can be planned across multiple non-contiguous slots. Both rule out task-as-plan.
- **`ON DELETE SET NULL` rather than CASCADE**: the day's plan is *intent*, not an attribute of the task. Deleting an unrelated task should not silently delete a planned hour from your day. Conversion to free-time keeps the slot intact so the user notices and re-plans.
- **Independence from `tasks` / `time_entries`**: the goal of the plan is *visualization* — see the day at a glance, decide if you need less free time or more focus blocks. Mutating tasks/time_entries from plan blocks would conflate intent with execution and create surprising side effects when editing or deleting a block.
- **Overlap as a service-layer check, not a Postgres EXCLUDE constraint**: a `tstzrange` exclusion constraint would push the check into the DB but adds operational complexity (extension required, error surface less ergonomic). At the volumes of a personal-use app a `COUNT(*)` is fine. Revisit if blocks ever scale to multiple users or many days at once.
- **Budget computation lives in `tasks` package, reused by `plan`**: the daily/weekly target math (waking-hour-weighted distribution against an 80h/week goal) is a single source of truth in `tasks/budget.go`. The plan endpoint includes the same `TimeEntrySummaryResponse` it would get from `/tasks/time-entries/summary` rather than recomputing.
- **Calendar events stay purely informational; a plan_block is what's "real"**: an earlier design considered deriving busy hours straight from calendar events, but most events on a calendar are informational (a festival, a reminder), not committed time. Requiring an explicit "create plan" step — which may or may not involve a task — keeps that distinction: only a thing you deliberately scheduled counts against your capacity.
- **`recurring_commitments` as a materializing template, not a recurrence engine**: a full RRULE-style engine for plan_blocks would duplicate the machinery the calendar domain already has for Google events, for a much simpler need (a fixed weekly work/class schedule). Generating real rows on demand means every existing plan_block operation (edit, delete, overlap check) already works on them without new code paths.
