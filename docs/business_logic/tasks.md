# Tasks

### Description

Domain for managing projects, tasks, todos, and time tracking. Projects form a hierarchy (self-referencing parent/child). Tasks belong to a project or exist as orphans. Todos are lightweight checklist items under a task. Time entries track work duration per task and roll up through the project hierarchy.

### States / Lifecycle

**Project**
```
created (started_at=NULL, finished_at=NULL)
  → started (started_at=timestamp, finished_at=NULL)
    → finished (finished_at=timestamp)
```

**Task**
```
created (started_at=NULL, finished_at=NULL)
  → started (started_at=timestamp, finished_at=NULL)
    → finished (finished_at=timestamp)
```

**Todo**
```
created (is_done=false) → done (is_done=true)
```
Togglable — can go back to false.

**Time Entry**
```
open (finished_at=NULL) → finished (finished_at=timestamp)
```
Only finished entries count toward time calculations.

### Business Rules

**Project Hierarchy**
- Root projects have `parent_id = NULL`.
- Sub-projects reference a parent project via `parent_id`.
- No depth limit enforced. No cycle prevention.

**Orphan Tasks**
- Tasks with `project_id = NULL` are orphans — not assigned to any project.
- Orphans appear at the root level in the active tree.
- Can be assigned to a project later via PATCH.

**Active Tree**
- Shows the user's current work: active projects + unfinished tasks.
- Active projects: `started_at IS NOT NULL AND finished_at IS NULL`.
- Unfinished tasks: `finished_at IS NULL`.
- Ordering: sub-projects prepended before tasks within a project. Started tasks before unstarted. Root level: projects → orphan started → orphan unstarted.

**Time Tracking**
- Time entries are created against a task with `started_at` and optionally `finished_at`.
- `time_spent = SUM(finished_at - started_at)` for finished entries only. Open entries excluded.
- A task's `time_spent` is computed from its time entries.
- A project's `time_spent` = its direct tasks' time + all descendant sub-projects' time (recursive, bottom-up accumulation).

**Time Entry Summary**
- Returns total seconds for two rolling windows: today and current week (Monday-based).
- Entries that span a boundary are clamped: `finished_at - GREATEST(started_at, boundary_start)`. An entry started yesterday but finished today only counts today's portion.

**Finish Cascade**
- When a project is finished (via PATCH `finished_at`):
  1. All descendant projects get `finished_at = NOW()`.
  2. All tasks under the project and its descendants get `finished_at = NOW()`.
- Application-layer logic, not database cascades.
- Only affects unfinished items (`finished_at IS NULL`).

**Partial Updates (PATCH)**
- All update endpoints use PATCH semantics: only provided fields are modified.
- Implemented via SQL `CASE WHEN @set_field THEN @value ELSE field END`.
- Omitted fields are untouched.

**Todos**
- Checklist items under a task. No time tracking.
- Ordered by completion status (incomplete first), then by ID.
- Cascade deleted when their parent task is deleted.

**History**
- Aggregates finished time entries into periodic buckets (daily/weekly/monthly).
- Values are in decimal hours (seconds / 3600).
- Missing periods within the range are zero-filled.
- Timezone-aware: uses the server's configured timezone (Europe/Madrid) for period boundaries.
- Entries spanning a period boundary are split: the portion before midnight (or Monday, or 1st of month) counts toward the earlier period, the portion after counts toward the later one.

**Task dependencies**
- A task can depend on other tasks. Dependencies are managed via the `depends_on` field on create/update task (list of task IDs). Setting `depends_on` replaces all existing dependencies. Omitting it leaves them unchanged.
- The effective due date of a task is the minimum of its own `due_at` and the effective `due_at` of all tasks it depends on (recursive). Finished dependencies contribute their `due_at` directly (carried in the `depends_on` JSON ref).
- A task that has at least one unfinished dependency is considered "blocked". All task responses include a `blocked` boolean.
- A task is hidden from the active tree and due-date list if all of its dependencies are themselves blocked.
- A task that inherits a due date from its dependencies is returned in the due-date list.
- Dependency responses include `id` and `name` of the referenced task (not just the ID).
- The reverse relationship (tasks that depend on this task) is returned as `blocks` in all task responses.
- Project children (GET /projects/{id}/children) include `depends_on`, `blocks`, and `blocked` for task-type children (omitted for sub-project children).
- Project children are ordered: sub-projects first, then started tasks, then unstarted tasks, then finished tasks. Ties within each group are broken by `due_at` ascending (nulls last), then name.

### Validations

**Create project**
- `name` required.

**Create task**
- `name` required.

**Create todo**
- `task_id` required.
- `name` required.

**Create time entry**
- `task_id` required.
- `started_at` required.

**Time entry history**
- `frequency` required, must be `daily`, `weekly`, or `monthly`.

**All update endpoints**
- `id` (URL param) must parse as int.
- Returns 404 if entity not found.

### Side Effects

- **On project finish** → cascade finishes all descendant projects and their tasks.
- **On task delete** → cascade deletes todos and time entries (FK ON DELETE CASCADE).
- **On task move** (PATCH `project_id`) → time entries move with the task, affecting `time_spent` on both source and destination projects.

### Decisions / Why

- **Finish cascade at application layer, not DB**: deletion does NOT cascade to child projects or tasks. Only finishing does. This is intentional — deleting a project shouldn't silently destroy all nested work. Finishing is the safe "archive" operation.
- **Time accumulation computed at query time, not stored**: avoids stale denormalized totals. The recursive CTE + bottom-up accumulation in GetProjectChildren is fast enough since project trees are small.
- **Time entry summary clamps with GREATEST**: an entry started at 23:00 yesterday and finished at 02:00 today should count 2 hours for today, not 3. GREATEST(started_at, today_start) handles this.
- **No constraint on multiple active time entries**: the system doesn't prevent timing two tasks simultaneously. The active entry endpoint returns a single row, but the DB allows multiple open entries.
- **Orphan tasks in active tree**: tasks without a project still appear so nothing gets lost. They sit at the root level as a visual cue to organize them.
- **History splits entries at period boundaries**: an entry from Sunday 23:00 to Monday 02:00 (Madrid) counts 1h toward the Sunday's week and 2h toward Monday's week. Without this, the entire 3h would land on the week of `started_at`, misrepresenting which week the work actually happened in. Uses `generate_series` + `GREATEST`/`LEAST` to clip each entry to its period segments.
