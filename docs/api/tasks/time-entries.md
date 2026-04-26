# Time Entries

## Create Time Entry

- **Method:** `POST`
- **Endpoint:** `/tasks/time-entries`
- **Description:** Creates a new time entry for a task.
- **Request Body:**
  ```json
  {
    "task_id": 1,
    "started_at": "2025-03-01T09:00:00Z",
    "finished_at": "2025-03-01T10:30:00Z",
    "comment": "Worked on feature X"
  }
  ```
  - `task_id` (required): The ID of the task.
  - `started_at` (required): The start time in RFC 3339 format.
  - `finished_at` (optional): The end time in RFC 3339 format.
  - `comment` (optional): A comment describing the work done.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "task_id": 1,
      "started_at": "2025-03-01T09:00:00Z",
      "finished_at": "2025-03-01T10:30:00Z",
      "comment": "Worked on feature X"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `task_id is required`, or `started_at is required`
  - **Code:** `409 Conflict`
    - **Content:** `an active time entry already exists` — returned when creating a time entry without `finished_at` while another unfinished entry already exists. Only one active (unfinished) time entry can exist at a time (enforced by `idx_time_entries_one_active`).
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create time entry`

## Get Active Time Entry

- **Method:** `GET`
- **Endpoint:** `/tasks/time-entries/active`
- **Description:** Returns the currently running (unfinished) time entry, including the task name and project name. At most one active entry can exist at a time (enforced by a partial unique index).
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "task_id": 5,
      "started_at": "2025-03-01T09:00:00Z",
      "finished_at": null,
      "comment": "Working on feature X",
      "task_name": "Implement feature X",
      "task_type": "standard",
      "priority": 3,
      "project_name": "My Project"
    }
    ```
  - `task_name`: Name of the associated task.
  - `task_type`: Task type of the associated task.
  - `recurrence`: Recurrence interval in days. Only present when `task_type` is `"recurring"`.
  - `priority`: Priority of the associated task (1 = highest, 5 = lowest).
  - `project_name`: Name of the task's project, or `null` if the task has no project.
- **Error Responses:**
  - **Code:** `404 Not Found`
    - **Content:** `no active time entry`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get active time entry`

## Update Time Entry

- **Method:** `PATCH`
- **Endpoint:** `/tasks/time-entries/{id}`
- **Description:** Partially updates a time entry. Only provided fields are modified.
- **Request Body:**
  ```json
  {
    "started_at": "2025-03-01T09:00:00Z",
    "finished_at": "2025-03-01T10:30:00Z",
    "comment": "Updated comment"
  }
  ```
  - `started_at` (optional): New start time.
  - `finished_at` (optional): New end time.
  - `comment` (optional): New comment.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "task_id": 1,
      "started_at": "2025-03-01T09:00:00Z",
      "finished_at": "2025-03-01T10:30:00Z",
      "comment": "Updated comment"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid time entry id` or `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `time entry not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update time entry`

## Delete Time Entry

- **Method:** `DELETE`
- **Endpoint:** `/tasks/time-entries/{id}`
- **Description:** Deletes a time entry.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid time entry id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete time entry`

## Get Time Entries by Date Range

- **Method:** `GET`
- **Endpoint:** `/tasks/time-entries`
- **Description:** Returns time entries that overlap with the given date range. An entry overlaps if it started on or before the end date AND has not finished before the start date (or is still running). Includes task and project metadata. Results are ordered by `started_at` descending.
- **Query Parameters:**
  - `start_time` (required): Start date in `YYYY-MM-DD` format.
  - `end_time` (optional): End date in `YYYY-MM-DD` format. Defaults to today.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "task_id": 5,
        "task_name": "Implement feature X",
        "task_type": "standard",
        "priority": 3,
        "project_id": 2,
        "project_name": "My Project",
        "started_at": "2026-03-15T09:00:00Z",
        "finished_at": "2026-03-15T10:30:00Z",
        "comment": "Worked on feature X",
        "task_finished_at": null,
        "time_spent": 5400
      },
      {
        "id": 2,
        "task_id": 8,
        "task_name": "Orphan task",
        "task_type": "recurring",
        "recurrence": 1,
        "priority": 1,
        "project_id": null,
        "project_name": null,
        "started_at": "2026-03-14T14:00:00Z",
        "finished_at": null,
        "comment": null,
        "task_finished_at": null,
        "time_spent": 3600
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `start_time is required`, `invalid start_time format, expected YYYY-MM-DD`, or `invalid end_time format, expected YYYY-MM-DD`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get time entries`

## Get Time Entry Summary

- **Method:** `GET`
- **Endpoint:** `/tasks/time-entries/summary`
- **Description:** Returns total seconds worked today and over the current week. Only completed (finished) time entries contribute. Time spent before the period start is excluded — for an entry that started yesterday and finished today, only the portion after midnight counts toward `today`. The week starts on Monday in the server's configured timezone.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "today": 5400,
      "week": 36000
    }
    ```
  - `today`: Total seconds worked since today's midnight (server timezone).
  - `week`: Total seconds worked since the current week's Monday (server timezone).
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get time entry summary`

## Get Time Entry History

- **Method:** `GET`
- **Endpoint:** `/tasks/time-entries/history`
- **Description:** Returns aggregated time entry durations over a date range, bucketed by a chosen frequency. Values represent total hours worked (as decimal). Missing periods within the range are filled with zero values. Timezone-aware: uses the server's configured timezone to determine which calendar day each time entry belongs to.
- **Query Parameters:**
  - `frequency` (required): `daily`, `weekly`, or `monthly`.
  - `start_at` (optional): Start date in `YYYY-MM-DD` format. Default depends on frequency: daily = 1 month ago, weekly = 12 weeks ago, monthly = 12 months ago.
  - `end_at` (optional): End date in `YYYY-MM-DD` format. Defaults to today.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** A wrapper object containing the date range and history data. The `start_at` and `end_at` values are snapped to the period boundary (Monday for weekly, 1st of month for monthly).
    ```json
    {
      "start_at": "2026-01-01",
      "end_at": "2026-03-23",
      "data": [
        { "date": "2026-01-01", "value": 10.5 },
        { "date": "2026-01-02", "value": 0 },
        { "date": "2026-01-03", "value": 12.25 }
      ]
    }
    ```
  - **Fields:**
    - `start_at`: Start of the date range, snapped to period boundary (`YYYY-MM-DD`).
    - `end_at`: End of the date range, snapped to period boundary (`YYYY-MM-DD`).
    - `data`: Array of history points, one per period. Missing periods are filled with 0.
    - `data[].date`: The start date of the aggregation period (`YYYY-MM-DD`).
    - `data[].value`: Total hours worked in the period (decimal float, e.g. 10.5 = 10h 30m).
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `frequency is required` or `frequency must be daily, weekly, or monthly`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get time entry history`
