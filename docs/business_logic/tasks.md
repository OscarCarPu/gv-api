# Task Management - User Flows

## 1. Planning a Project Hierarchy

A user organizes work into projects, sub-projects, and tasks.

- **Root projects** have no `parent_id`.
- **Sub-projects** reference a `parent_id` pointing to another project.
- **Tasks** reference a `project_id` to belong to a project, or have `project_id = null` (orphan tasks).

`GetRootProjects` returns only root-level, unfinished projects (`WHERE parent_id IS NULL AND finished_at IS NULL`).
`GetProjectChildren` returns a project's direct sub-projects and tasks (with their todos).

## 2. Working on a Task with Time Tracking

A user starts working on a project and its tasks, then logs time.

1. Create a project and start it via `PATCH started_at`.
2. Create a task under the project and start it via `PATCH started_at`.
3. Create time entries against the task with `started_at` and optionally `finished_at`.
4. Update open time entries with `finished_at` when done.

**Time calculation:** `time_spent = SUM(finished_at - started_at)` for finished entries only. Open entries (no `finished_at`) are excluded from the sum.

Finish a task by setting `finished_at` via PATCH. Finished tasks no longer appear in the active tree.

## 3. Managing Subtasks (Todos)

Todos are lightweight checklist items under a task.

1. Create todos under a task (`POST /tasks/todos`).
2. Toggle `is_done` via `PATCH` on the todo.
3. Todos are visible in the `GetProjectChildren` response nested under their parent task.

Todos have no time tracking of their own; they serve as a breakdown of work within a task.

## 4. Active Tree View (Dashboard)

The active tree shows the user's current work at a glance.

- **Active projects**: started but not finished (`started_at IS NOT NULL AND finished_at IS NULL`).
- **Unfinished tasks**: tasks without `finished_at`.
- Finished tasks and finished projects are excluded.

**Ordering rules:**
- Sub-projects are prepended before tasks within their parent project.
- Started tasks appear before unstarted tasks.
- Root level: projects first, then orphan started tasks, then orphan unstarted tasks.

**Nesting:** Sub-projects nest under their parent project. Tasks without a project (or whose project is not active) appear at the root level as orphans.

## 5. Time Accumulation Across Hierarchy

Time rolls up recursively from tasks through the project hierarchy.

- Each task's `time_spent` = sum of its finished time entries.
- A project's `time_spent` = sum of its direct tasks' `time_spent` + sum of all descendant sub-projects' `time_spent`.
- Accumulation is bottom-up: leaf projects first, then their parents.
- Open time entries (no `finished_at`) are excluded at every level.

This is computed in `GetProjectChildren` which uses a recursive descendant query.

## 6. Reorganizing Work

Users can move tasks between projects and make partial updates.

- Move a task to a different project via `PATCH project_id`.
- Partial updates only modify the fields sent in the request; other fields remain unchanged.
- Orphan tasks (no `project_id`) can be assigned to a project later.
- When a task moves, its time entries move with it, affecting `time_spent` on both source and destination projects.

## 7. Time Entry History

The history endpoint aggregates finished time entries into periodic buckets (daily, weekly, monthly).

**Aggregation logic:**
- Groups time entries by `date_trunc(frequency, started_at AT TIME ZONE tz)` — the started_at timestamp is converted to the server's configured timezone before truncating.
- Sums `finished_at - started_at` duration for each period and converts to hours (decimal).
- Only finished entries (`finished_at IS NOT NULL`) are included.

**Date range defaults:**
- Daily: last 30 days
- Weekly: last 12 weeks
- Monthly: last 12 months

Start and end dates are snapped to period boundaries (Monday for weekly, 1st of month for monthly).

**Zero-filling:** All periods within the range are included in the response. Periods with no tracked time have `value: 0`.

**Shared logic:** Period boundary calculations (PeriodStart, PeriodCeil, NextPeriodStart, FillMissingPeriods, ParseDateRange) live in `internal/history/` and are shared with the habits domain.
