# Tasks API

Endpoints are grouped by resource:

- [Projects](projects.md) — projects, tree, project children, parent candidates
- [Tasks](tasks.md) — tasks, list-fast, by-due-date, per-task time entries
- [Todos](todos.md) — todos under a task
- [Time Entries](time-entries.md) — time tracking, active entry, history, summary

Shared `task_type`, `recurrence` and `priority` semantics:

## Task Types

| Type | Description | `recurrence` |
|------|-------------|--------------|
| `standard` | Default. A one-off task with a clear start and finish. | Must be absent |
| `continuous` | A task that represents ongoing work (e.g. "quick fixes"). | Must be absent |
| `recurring` | A task that repeats on a fixed interval (e.g. "clean kitchen"). | Required (days) |

- `task_type` defaults to `"standard"` when not provided.
- `recurrence` is an integer representing the number of days between recurrences (e.g. `1` = daily, `7` = weekly, `30` = monthly).
- `recurrence` is required when `task_type` is `"recurring"` and must not be provided otherwise.
- The backend does nothing special on finish; the frontend handles recurrence (advancing `due_at`, clearing `finished_at`).
- `task_type` and `recurrence` are returned on all endpoints that include task information.
- `recurrence` is omitted from JSON responses when `null` (non-recurring tasks).

## Task Priority

`priority` is `1` (highest) to `5` (lowest), default `3`, returned everywhere.
