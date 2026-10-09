# Tasks

### Description

Projects (a hierarchy), tasks (in a project or orphan), todos (checklists under a task) and time entries (rolled up through the hierarchy).

### States / Lifecycle

Projects and tasks: `created` → `started` (`started_at`) → `finished` (`finished_at`).
Todos toggle `is_done`. Time entries are open until `finished_at` is set; only finished entries count.

### Business Rules

**Project Hierarchy**
- Root projects have `parent_id = NULL`; no depth limit.
- PATCH `parent_id` moves a project with its subtree (`null` = root, omitted = unchanged).
- A project cannot move under itself or a descendant (409); the parent must exist (400). The check and write share a transaction under an advisory lock.
- `GET /projects/{id}/parent-candidates` lists valid new parents: not itself, a descendant or finished (the current parent is always listed).

**Orphan Tasks**
- `project_id = NULL`; shown at the root of the active tree and assignable later.

**Task Types**
- `standard` (default, one-off), `continuous` (no natural end) or `recurring` (every `recurrence` days, a positive integer).
- The backend does nothing special on finish; the frontend handles recurrence (advancing `due_at`, clearing `finished_at`).
- Changing away from `recurring` clears `recurrence`.

**Active Tree**
- Active projects (started, unfinished) and unfinished tasks.
- Within a project: sub-projects, then started tasks, then unstarted. Root: projects, then started orphans, then unstarted orphans.

**Time Tracking**
- `time_spent = SUM(finished_at - started_at)` over finished entries. A project's includes all descendants.

**Time Entry Summary**
- Seconds for today and the current week (Monday-based); entries crossing the boundary only count the part inside it.

**Finish Cascade**
- Finishing a project finishes every unfinished descendant project and task (in the service, not the DB).

**Partial Updates**
- Updates only touch provided fields (`CASE WHEN @set_field ...` in SQL).

**Todos**
- Incomplete first, then by ID. Deleted with their task.

**History**
- Finished time entries in decimal hours per day/week/month in the server timezone, zero-filled, with entries split at period boundaries.

**Estimate and urgency (Due Soon)**
- Only `standard` tasks with `estimate_hours` get urgency. Recurring tasks are done on their due day, not started ahead, so they get none.
- `remaining_hours = max(estimate_hours − time_spent − planned_hours, 0)`, where `planned_hours` are the task's plan blocks not yet past ([plan](plan.md)).
- Due dates (own or the project's) are stored as midnight UTC and re-anchored to the server timezone before comparing with today.
- Free hours per day come from capacity (`GET /capacity/free-busy`) in one batched call. Day `n` from today is capped at capacity − 0.5h·n, never below 6h, to absorb unplanned work.
- Each task's **effective priority** is its own, raised to the highest priority of anything it transitively blocks. It drives scheduling and the `min_priority` filter, which runs after urgency.
- Tasks are back-filled from the end of each dependency chain: a task's last usable day is the day before its due date, or the day a dependent starts. In A → B → C, A must fit A+B+C's hours before C's deadline.
- All tasks draw from one shared pool of free hours. Work order is priority, then soonest last usable day, then soonest effective due date, then more remaining hours first, then higher ID; filling backwards reverses it, so lower priority and later deadlines claim first and the most important work lands closest to today.
- When the pool runs out, a task's start is squeezed to today and so is its dependencies' last usable day, which can tie unrelated chains. The effective due date keeps them apart (a chain due the 12th goes before one due the 18th); only then do remaining hours decide.
- Each task gets `start_by` (the day its hours are covered), `urgent = start_by <= today`, `finish_by` and `work_order` (1 = first).

**Task dependencies**
- `depends_on` (task IDs) on create/update replaces all dependencies; omitted leaves them. Finished tasks are silently ignored.
- The effective due date is the minimum of the task's own and those of everything it blocks (recursive).
- A task with an unfinished dependency is `blocked`, but is still listed everywhere.
- Responses carry `depends_on` and `blocks` as `{id, name}`, plus `blocked`; project children include them for tasks only.
- Project children order: sub-projects, started tasks, unstarted tasks, finished tasks; ties by `due_at` (nulls last), then name.

### Validations

- Projects, tasks and todos need a `name`; todos and time entries a `task_id`; time entries a `started_at`.
- `task_type` must be `standard`, `continuous` or `recurring`; `recurrence` is required for `recurring` and rejected otherwise.
- History `frequency` must be `daily`, `weekly` or `monthly`.
- Updates return 400 for a bad id and 404 if the entity does not exist.

### Side Effects

- **Project finished** → descendants finished.
- **Project or task moved** → `time_spent` of old and new ancestors changes.
- **Task deleted** → todos and time entries deleted (FK cascade).

### Decisions / Why

- **Only finishing cascades, not deleting**: deleting a project must not silently destroy nested work; finishing is the safe archive.
- **Time computed on read**: no stale totals; project trees are small.
- **One active time entry**: a partial unique index (`idx_time_entries_one_active`); a second one returns 409.
- **Orphans stay visible** at the root so nothing gets lost.
- **Entries split at period boundaries**: Sunday 23:00–Monday 02:00 counts 1h to one week and 2h to the next.
