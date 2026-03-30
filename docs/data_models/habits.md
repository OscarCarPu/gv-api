# Habits - Data Models

## Tables

### habits

| Column             | Type    | Constraints                                      |
|--------------------|---------|--------------------------------------------------|
| id                 | INTEGER | PRIMARY KEY, GENERATED ALWAYS AS IDENTITY        |
| name               | TEXT    | NOT NULL                                         |
| description        | TEXT    | nullable                                         |
| frequency          | TEXT    | NOT NULL, DEFAULT 'daily', CHECK (daily/weekly/monthly) |
| target_min         | REAL    | nullable                                         |
| target_max         | REAL    | nullable                                         |
| recording_required | BOOLEAN | NOT NULL, DEFAULT true                           |
| current_streak     | INTEGER | NOT NULL, DEFAULT 0                              |
| longest_streak     | INTEGER | NOT NULL, DEFAULT 0                              |

### habit_logs

| Column   | Type    | Constraints                                    |
|----------|---------|------------------------------------------------|
| habit_id | INTEGER | NOT NULL, FK -> habits.id ON DELETE CASCADE    |
| log_date | DATE    | NOT NULL, DEFAULT CURRENT_DATE                 |
| value    | REAL    | NOT NULL                                       |

**Unique constraint:** (habit_id, log_date) -- one log per habit per day.

## Relationships

```
habits (1) --< (many) habit_logs
```

- Deleting a habit cascades to all its logs.
- No primary key on habit_logs beyond the unique constraint.
