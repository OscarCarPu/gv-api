# Habits API

The Habits API allows you to create, manage, and track habits with support for different value types, frequencies, and target configurations.

## Authentication

All endpoints require an API key passed via the `X-API-Key` header.

## Base URL

```
/habits
```

## Enums

### ValueType

| Value | Description |
|-------|-------------|
| `boolean` | True/false habits (logged as 0 or 1) |
| `numeric` | Numeric value habits |

### TargetFrequency

| Value | Description |
|-------|-------------|
| `daily` | Habit is due every day |
| `weekly` | Habit is due once per week |
| `monthly` | Habit is due once per month |

### ComparisonType

| Value | Description |
|-------|-------------|
| `equals` | Value must equal target |
| `greater_than` | Value must be greater than target |
| `less_than` | Value must be less than target |
| `greater_equal_than` | Value must be greater than or equal to target |
| `less_equal_than` | Value must be less than or equal to target |
| `in_range` | Value must be between target_min and target_max |

---

## Endpoints

### Habits

#### List Habits

```
GET /habits
```

Retrieve a paginated list of all habits.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number (min: 1) |
| `page_size` | integer | 20 | Items per page (min: 1, max: 100) |

**Response:** `200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "name": "Exercise",
      "description": "Daily workout",
      "value_type": "boolean",
      "unit": null,
      "frequency": "daily",
      "target_value": null,
      "target_min": null,
      "target_max": null,
      "comparison_type": null,
      "start_date": "2024-01-01",
      "end_date": null,
      "is_required": true,
      "color": "#3B82F6",
      "icon": "fas fa-check",
      "created_at": "2024-01-01T00:00:00",
      "updated_at": "2024-01-01T00:00:00"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20,
  "total_pages": 1
}
```

---

#### Create Habit

```
POST /habits
```

Create a new habit with the specified configuration.

**Request Body:**

```json
{
  "name": "Exercise",
  "description": "Daily workout routine",
  "value_type": "boolean",
  "unit": null,
  "frequency": "daily",
  "target_value": null,
  "target_min": null,
  "target_max": null,
  "comparison_type": null,
  "start_date": "2024-01-01",
  "end_date": null,
  "is_required": true,
  "color": "#3B82F6",
  "icon": "fas fa-dumbbell"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Habit name |
| `value_type` | ValueType | Yes | Type of value to track |
| `description` | string | No | Habit description |
| `unit` | string | No | Unit of measurement (e.g., "minutes", "km") |
| `frequency` | TargetFrequency | No | How often the habit is due |
| `target_value` | decimal | No | Target value for comparisons |
| `target_min` | decimal | No | Minimum value for range comparison |
| `target_max` | decimal | No | Maximum value for range comparison |
| `comparison_type` | ComparisonType | No | How to compare logged values |
| `start_date` | date | No | When the habit becomes active |
| `end_date` | date | No | When the habit expires |
| `is_required` | boolean | No | Whether the habit is required (default: true) |
| `color` | string | No | Hex color code (default: "#3B82F6") |
| `icon` | string | No | FontAwesome icon class (default: "fas fa-check") |

**Response:** `201 Created`

Returns the created habit object.

**Errors:**
- `409 Conflict` - Habit with the same name already exists
- `422 Unprocessable Entity` - Validation error

---

#### Get Habit

```
GET /habits/{habit_id}
```

Retrieve a specific habit by its ID.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Response:** `200 OK`

Returns the habit object.

**Errors:**
- `404 Not Found` - Habit not found

---

#### Update Habit

```
PATCH /habits/{habit_id}
```

Update an existing habit's configuration. Only provided fields are updated.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Request Body (HabitUpdate schema):**

```json
{
  "name": "Updated Name",
  "description": "Updated description",
  "unit": "minutes",
  "frequency": "weekly",
  "target_value": 30,
  "target_min": null,
  "target_max": null,
  "comparison_type": "greater_equal_than",
  "start_date": "2024-01-01",
  "end_date": "2024-12-31",
  "is_required": false,
  "color": "#10B981",
  "icon": "fas fa-running",
  "active": true
}
```

All fields are optional. Only included fields will be updated.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Updated habit name |
| `description` | string | Updated description |
| `unit` | string | Updated unit of measurement |
| `frequency` | TargetFrequency | Updated frequency |
| `target_value` | decimal | Updated target value |
| `target_min` | decimal | Updated minimum value |
| `target_max` | decimal | Updated maximum value |
| `comparison_type` | ComparisonType | Updated comparison type |
| `start_date` | date | Updated start date |
| `end_date` | date | Updated end date |
| `is_required` | boolean | Updated required status |
| `color` | string | Updated color |
| `icon` | string | Updated icon |
| `active` | boolean | Whether the habit is active |

**Response:** `200 OK`

Returns the updated habit object.

**Errors:**
- `404 Not Found` - Habit not found
- `409 Conflict` - Name already taken by another habit
- `422 Unprocessable Entity` - Validation error

---

#### Delete Habit

```
DELETE /habits/{habit_id}
```

Delete a habit and all its associated logs.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Response:** `204 No Content`

**Errors:**
- `404 Not Found` - Habit not found

---

#### Get Daily Progress

```
GET /habits/daily
```

Get progress for all due habits on a specific date.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `target_date` | date | today | The date to check progress for |

**Response:** `200 OK`

```json
[
  {
    "habit_id": 1,
    "habit_name": "Exercise",
    "is_due": true,
    "is_logged": true,
    "is_target_met": true,
    "logged_value": 1
  }
]
```

---

#### Get Habit Stats

```
GET /habits/{habit_id}/stats
```

Get statistics for a habit over a specified period.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `days` | integer | 30 | Number of days to analyze (min: 1, max: 365) |

**Response:** `200 OK`

```json
{
  "total_logs": 25,
  "completion_rate": 83.3,
  "current_streak": 5,
  "longest_streak": 12,
  "average_value": 45.5
}
```

| Field | Description |
|-------|-------------|
| `total_logs` | Total number of log entries |
| `completion_rate` | Percentage of due dates with target met |
| `current_streak` | Current consecutive streak |
| `longest_streak` | Longest streak achieved |
| `average_value` | Average logged value (numeric habits only) |

**Errors:**
- `404 Not Found` - Habit not found

---

#### Get Habit Streak

```
GET /habits/{habit_id}/streak
```

Get streak information for a habit.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Response:** `200 OK` (HabitStreak schema)

```json
{
  "current": 5,
  "longest": 12,
  "last_completed": "2024-01-15"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `current` | integer | Current consecutive streak count |
| `longest` | integer | Longest streak ever achieved |
| `last_completed` | date | Date of most recent successful completion (null if never completed) |

**Errors:**
- `404 Not Found` - Habit not found

---

### Habit Logs

#### List Habit Logs

```
GET /habits/{habit_id}/logs
```

Retrieve a paginated list of logs for a specific habit.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `start_date` | date | null | Filter logs from this date |
| `end_date` | date | null | Filter logs until this date |
| `page` | integer | 1 | Page number |
| `page_size` | integer | 20 | Items per page (max: 100) |

**Response:** `200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "habit_id": 1,
      "log_date": "2024-01-15",
      "value": 1,
      "created_at": "2024-01-15T10:30:00",
      "updated_at": "2024-01-15T10:30:00"
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 20,
  "total_pages": 1
}
```

**Errors:**
- `404 Not Found` - Habit not found

---

#### Create Habit Log

```
POST /habits/{habit_id}/logs
```

Create a new log entry for a habit on a specific date.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Request Body:**

```json
{
  "log_date": "2024-01-15",
  "value": 1
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `log_date` | date | Yes | The date of the log entry |
| `value` | decimal | Yes | The logged value (0 or 1 for boolean, any positive number for numeric) |

**Response:** `201 Created`

Returns the created log object.

**Errors:**
- `404 Not Found` - Habit not found
- `409 Conflict` - Log already exists for this date
- `422 Unprocessable Entity` - Validation error

---

#### Upsert Habit Log

```
PATCH /habits/{habit_id}/logs
```

Create or update a log for a habit on a specific date. If a log exists for the date, it will be updated; otherwise, a new log is created.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Request Body:**

```json
{
  "log_date": "2024-01-15",
  "value": 1
}
```

**Response:** `200 OK`

Returns the created or updated log object.

**Errors:**
- `404 Not Found` - Habit not found
- `422 Unprocessable Entity` - Validation error

---

#### Quick Log Habit

```
POST /habits/{habit_id}/quick-log
```

Quick log with smart defaults. Uses today's date and the habit's target value (or 1 for boolean habits) if not specified.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |

**Request Body (optional):**

```json
{
  "value": 1,
  "log_date": "2024-01-15"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `value` | decimal | target_value or 1 | The value to log |
| `log_date` | date | today | The date to log for |

**Response:** `200 OK`

Returns the created or updated log object.

**Errors:**
- `404 Not Found` - Habit not found

---

#### Update Habit Log

```
PATCH /habits/{habit_id}/logs/{log_id}
```

Update an existing log entry's date or value.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |
| `log_id` | integer | The log ID |

**Request Body:**

```json
{
  "log_date": "2024-01-16",
  "value": 2
}
```

All fields are optional.

**Response:** `200 OK`

Returns the updated log object.

**Errors:**
- `404 Not Found` - Log not found
- `422 Unprocessable Entity` - Validation error

---

#### Delete Habit Log

```
DELETE /habits/{habit_id}/logs/{log_id}
```

Delete a specific log entry.

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `habit_id` | integer | The habit ID |
| `log_id` | integer | The log ID |

**Response:** `204 No Content`

**Errors:**
- `404 Not Found` - Log not found

---

## Schemas Reference

### HabitCreate

Used when creating a new habit.

```json
{
  "name": "string (required)",
  "description": "string | null",
  "value_type": "boolean | numeric (required)",
  "unit": "string | null",
  "frequency": "daily | weekly | monthly | null",
  "target_value": "decimal | null",
  "target_min": "decimal | null",
  "target_max": "decimal | null",
  "comparison_type": "equals | greater_than | less_than | greater_equal_than | less_equal_than | in_range | null",
  "start_date": "date | null",
  "end_date": "date | null",
  "is_required": "boolean (default: true)",
  "color": "string (default: #3B82F6)",
  "icon": "string (default: fas fa-check)"
}
```

### HabitUpdate

Used when updating an existing habit. All fields are optional.

```json
{
  "name": "string | null",
  "description": "string | null",
  "unit": "string | null",
  "frequency": "daily | weekly | monthly | null",
  "target_value": "decimal | null",
  "target_min": "decimal | null",
  "target_max": "decimal | null",
  "comparison_type": "equals | greater_than | less_than | greater_equal_than | less_equal_than | in_range | null",
  "start_date": "date | null",
  "end_date": "date | null",
  "is_required": "boolean | null",
  "color": "string | null",
  "icon": "string | null",
  "active": "boolean | null"
}
```

### HabitRead

Returned when reading habit data.

```json
{
  "id": "integer",
  "name": "string",
  "description": "string | null",
  "value_type": "boolean | numeric",
  "unit": "string | null",
  "frequency": "daily | weekly | monthly | null",
  "target_value": "decimal | null",
  "target_min": "decimal | null",
  "target_max": "decimal | null",
  "comparison_type": "string | null",
  "start_date": "date | null",
  "end_date": "date | null",
  "is_required": "boolean",
  "color": "string",
  "icon": "string",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### HabitStreak

Returned when getting streak information.

```json
{
  "current": "integer",
  "longest": "integer",
  "last_completed": "date | null"
}
```

### HabitStats

Returned when getting habit statistics.

```json
{
  "total_logs": "integer",
  "completion_rate": "decimal",
  "current_streak": "integer",
  "longest_streak": "integer",
  "average_value": "decimal | null"
}
```

### HabitLogRead

Returned when reading log data.

```json
{
  "id": "integer",
  "habit_id": "integer",
  "log_date": "date",
  "value": "decimal",
  "created_at": "datetime",
  "updated_at": "datetime"
}
```

### DailyProgress

Returned when getting daily progress.

```json
{
  "habit_id": "integer",
  "habit_name": "string",
  "is_due": "boolean",
  "is_logged": "boolean",
  "is_target_met": "boolean",
  "logged_value": "decimal | null"
}
```
