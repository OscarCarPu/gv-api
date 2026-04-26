# Tasks API

Endpoints are grouped by resource:

- [Projects](projects.md) — projects, tree, project children
- [Tasks](tasks.md) — tasks, list-fast, by-due-date, per-task time entries
- [Todos](todos.md) — todos under a task
- [Time Entries](time-entries.md) — time tracking, active entry, history, summary

The shared `task_type`, `recurrence`, and `priority` semantics are documented below and apply to every endpoint that returns task data.

## Task Types

Tasks have a `task_type` field that determines their behavior:

| Type | Description | `recurrence` |
|------|-------------|--------------|
| `standard` | Default. A one-off task with a clear start and finish. | Must be absent |
| `continuous` | A task that represents ongoing work (e.g. "quick fixes"). | Must be absent |
| `recurring` | A task that repeats on a fixed interval (e.g. "clean kitchen"). | Required (days) |

- `task_type` defaults to `"standard"` when not provided.
- `recurrence` is an integer representing the number of days between recurrences (e.g. `1` = daily, `7` = weekly, `30` = monthly).
- `recurrence` is required when `task_type` is `"recurring"` and must not be provided otherwise.
- The backend does not enforce any special behavior on finish for any task type. `finished_at` can be set freely on all types. The frontend is responsible for handling recurrence logic (e.g. advancing `due_at` and clearing `finished_at`).
- `task_type` and `recurrence` are returned on all endpoints that include task information.
- `recurrence` is omitted from JSON responses when `null` (non-recurring tasks).

## Task Priority

Tasks have a `priority` field with values from `1` (highest) to `5` (lowest). It defaults to `3` when not provided. Priority is returned on all endpoints that include task information.
