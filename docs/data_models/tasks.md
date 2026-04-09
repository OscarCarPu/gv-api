# Tasks - Data Models

## Tables

### projects

| Column      | Type        | Constraints                                  |
|-------------|-------------|----------------------------------------------|
| id          | SERIAL      | PRIMARY KEY                                  |
| parent_id   | INTEGER     | nullable, FK -> projects.id (self-reference) |
| name        | TEXT        | NOT NULL                                     |
| description | TEXT        | nullable                                     |
| due_at      | DATE        | nullable                                     |
| started_at  | TIMESTAMPTZ | nullable                                     |
| finished_at | TIMESTAMPTZ | nullable                                     |

**Indexes:** idx_projects_parent_id

### tasks

| Column      | Type        | Constraints                   |
|-------------|-------------|-------------------------------|
| id          | SERIAL      | PRIMARY KEY                   |
| project_id  | INTEGER     | nullable, FK -> projects.id   |
| name        | TEXT        | NOT NULL                      |
| description | TEXT        | nullable                      |
| due_at      | DATE        | nullable                      |
| started_at  | TIMESTAMPTZ | nullable                      |
| finished_at | TIMESTAMPTZ | nullable                      |
| task_type   | TEXT        | NOT NULL, DEFAULT 'standard'  |
| recurrence  | INTEGER     | nullable                      |

**Indexes:** idx_tasks_project_id, idx_tasks_unfinished (WHERE finished_at IS NULL), idx_tasks_due_at (WHERE due_at IS NOT NULL)

**Checks:**
- `task_type` must be one of: `'standard'`, `'continuous'`, `'recurring'`
- `recurrence` must be a positive integer (number of days) or NULL
- `recurrence` is required when `task_type = 'recurring'` and must be NULL otherwise

### task_dependencies

| Column     | Type    | Constraints                               |
|------------|---------|-------------------------------------------|
| task_id    | INTEGER | NOT NULL, FK -> tasks.id ON DELETE CASCADE |
| depends_on | INTEGER | NOT NULL, FK -> tasks.id ON DELETE CASCADE |

**Primary Key:** (task_id, depends_on)
**Checks:** (task_id != depends_on)

### todos

| Column  | Type    | Constraints                                 |
|---------|---------|---------------------------------------------|
| id      | SERIAL  | PRIMARY KEY                                 |
| task_id | INTEGER | NOT NULL, FK -> tasks.id ON DELETE CASCADE  |
| name    | TEXT    | NOT NULL                                    |
| is_done | BOOLEAN | NOT NULL, DEFAULT false                     |

**Indexes:** idx_todos_task_id

### time_entries

| Column      | Type        | Constraints                                 |
|-------------|-------------|---------------------------------------------|
| id          | SERIAL      | PRIMARY KEY                                 |
| task_id     | INTEGER     | NOT NULL, FK -> tasks.id ON DELETE CASCADE  |
| started_at  | TIMESTAMPTZ | NOT NULL, DEFAULT CURRENT_TIMESTAMP         |
| finished_at | TIMESTAMPTZ | nullable                                    |
| comment     | TEXT        | nullable                                    |

**Indexes:** idx_time_entries_task_id, idx_time_entries_started_at

## Relationships

```
projects (1) --< (many) projects           [self-referencing via parent_id]
projects (1) --< (many) tasks              [via project_id, nullable]
tasks    (1) --< (many) todos              [cascade delete]
tasks    (1) --< (many) time_entries       [cascade delete]
tasks    (many) >--< (many) tasks          [via task_dependencies, cascade delete]
```

- Deleting a task cascades to its todos and time entries.
- Deleting a project does **not** cascade — finishing handles descendant cleanup at the application layer.
- Tasks with `project_id = NULL` are orphan tasks (not assigned to any project).
- Projects with `parent_id = NULL` are root projects.
