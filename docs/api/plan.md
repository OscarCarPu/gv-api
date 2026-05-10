# Plan

CRUD for the user's day plan: time-boxed blocks scheduled across today. Each block is either linked to a task (`task_id` set) or stands alone as a free-time block (`label` only). The plan is read-only with respect to `tasks` and `time_entries` — the UI's ▶/✓ buttons on a linked block call the existing task / time-entry endpoints directly.

**Auth:** full-private. All endpoints require a `full` token (see [auth.md](auth.md)).

**Data model:** see [data_models/plan.md](../data_models/plan.md). Business rules and validations: see [business_logic/plan.md](../business_logic/plan.md).

### Plan Block fields

| Field              | Type             | Notes                                                                                |
|--------------------|------------------|--------------------------------------------------------------------------------------|
| `id`               | integer          | Server-assigned.                                                                     |
| `started_at`       | string           | RFC3339 timestamp.                                                                   |
| `ended_at`         | string           | RFC3339 timestamp. Must be strictly after `started_at`.                              |
| `task_id`          | integer \| null  | When set, the block is linked to a task.                                             |
| `task_name`        | string \| null   | Joined from `tasks.name`. `null` for free-time blocks (or if the task was deleted).  |
| `label`            | string           | Always set, 1–200 chars. Auto-filled with task name on Create when omitted.          |
| `note`             | string \| null   | Optional free-form note.                                                             |
| `task_type`        | string \| null   | Joined from `tasks.task_type` (`standard` / `continuous` / `recurring`). Read-only.  |
| `task_recurrence`  | integer \| null  | Joined from `tasks.recurrence`. Days between recurrences.                            |
| `task_started_at`  | string \| null   | Joined from `tasks.started_at`. RFC3339 timestamp.                                   |
| `task_finished_at` | string \| null   | Joined from `tasks.finished_at`. RFC3339 timestamp.                                  |

The `task_*` fields are always present on every `PlanBlock` response. They are `null` for free-time blocks (`task_id IS NULL`) and for any joined column whose task value is itself `NULL` (`recurrence`, `started_at`, or `finished_at` on a task that has not been started or has no recurrence). They are read-only — they reflect the joined task state at fetch time and never affect persistence.

---

## Get Today's Plan

- **Method:** `GET`
- **Endpoint:** `/plan/today`
- **Description:** Returns today's plan blocks (ordered by `started_at`), the totals split by linked vs. free-time, and the same time-budget payload returned by `GET /tasks/time-entries/summary`. "Today" is the server-timezone date.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "date": "2026-05-10",
      "blocks": [
        {
          "id": 1,
          "started_at": "2026-05-10T09:00:00Z",
          "ended_at": "2026-05-10T10:30:00Z",
          "task_id": 18,
          "task_name": "Fix server logs",
          "label": "Fix server logs",
          "note": null,
          "task_type": "standard",
          "task_recurrence": null,
          "task_started_at": null,
          "task_finished_at": null
        },
        {
          "id": 2,
          "started_at": "2026-05-10T12:30:00Z",
          "ended_at": "2026-05-10T14:00:00Z",
          "task_id": null,
          "task_name": null,
          "label": "comer",
          "note": null,
          "task_type": null,
          "task_recurrence": null,
          "task_started_at": null,
          "task_finished_at": null
        }
      ],
      "totals": {
        "task_seconds": 5400,
        "free_seconds": 5400
      },
      "budget": {
        "today": 0,
        "week": 32400,
        "daily_target_seconds": 42666,
        "weekly_target_seconds": 288000,
        "pace": {
          "uniform_per_day_seconds": 41492,
          "uniform_today_share_seconds": 39056,
          "weighted_weekday_seconds": 45333,
          "weighted_weekend_seconds": 32000,
          "weighted_today_share_seconds": 42666,
          "remaining_full_days": 6,
          "goal_reached": false
        }
      }
    }
    ```
  - **Fields:**
    - `date`: today's local date (`YYYY-MM-DD`).
    - `blocks`: today's blocks, ordered ascending by `started_at`. Empty array if none.
    - `totals.task_seconds`: sum of durations of blocks with `task_id != null`.
    - `totals.free_seconds`: sum of durations of blocks with `task_id == null`.
    - `budget`: identical shape to `GET /tasks/time-entries/summary` — see [tasks/time-entries.md](tasks/time-entries.md#get-time-entry-summary).
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get today's plan`

## Create Plan Block

- **Method:** `POST`
- **Endpoint:** `/plan/blocks`
- **Description:** Creates a plan block. The `plan_date` is derived from `started_at` (UTC date). Either `task_id` or `label` must be provided; when only `task_id` is given, the server auto-fills `label` with the task's current name.
- **Request Body:**
  ```json
  {
    "started_at": "2026-05-10T11:00:00Z",
    "ended_at": "2026-05-10T12:30:00Z",
    "task_id": 7,
    "label": "Sprint planning",
    "note": "Focus on the API v2 PRs"
  }
  ```
  - `started_at` (required): RFC3339 timestamp.
  - `ended_at` (required): RFC3339 timestamp, strictly after `started_at`.
  - `task_id` (optional): integer task id. The task must exist.
  - `label` (optional): 1–200 chars after trim. Required if `task_id` is omitted. When `task_id` is set and `label` is omitted, the server fills it with the task name.
  - `note` (optional): free-form string.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:** A `PlanBlock` (same shape as `blocks[]` items in `GET /plan/today`).
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `ended_at must be after started_at`, `label or task_id is required`, `label must be at most 200 characters`, `plan block overlaps with an existing one`, or `task not found` (when `task_id` references a row that does not exist).
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create plan block`

## Update Plan Block

- **Method:** `PUT`
- **Endpoint:** `/plan/blocks/{id}`
- **Description:** Partially updates a plan block. Only fields present in the body are modified. Two explicit clear-flags exist for nullable fields.
- **Request Body:**
  ```json
  {
    "started_at": "2026-05-10T11:00:00Z",
    "ended_at": "2026-05-10T13:00:00Z",
    "task_id": 7,
    "clear_task": false,
    "label": "Sprint planning extended",
    "note": null,
    "clear_note": true
  }
  ```
  - `started_at` (optional), `ended_at` (optional): RFC3339 timestamps. If only one is provided, it is validated against the persisted other side. Either change triggers an overlap recheck (excluding the row being updated).
  - `task_id` (optional): set to a new task id to relink. Cannot be combined with `clear_task: true`.
  - `clear_task` (optional, boolean): set to `true` to convert a linked block into a free-time block. Mutually exclusive with `task_id`.
  - `label` (optional): new label. Trimmed; must be 1–200 chars after trim.
  - `note` (optional), `clear_note` (optional, boolean): set or clear the note. Use `clear_note: true` to set to `null`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** The updated `PlanBlock`.
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid plan block id`, `Invalid Body`, `ended_at must be after started_at`, `label or task_id is required`, `label must be at most 200 characters`, or `plan block overlaps with an existing one`.
  - **Code:** `404 Not Found`
    - **Content:** `plan block not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update plan block`

## Delete Plan Block

- **Method:** `DELETE`
- **Endpoint:** `/plan/blocks/{id}`
- **Description:** Permanently deletes a plan block. Hard delete; no soft-delete column.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid plan block id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete plan block`
