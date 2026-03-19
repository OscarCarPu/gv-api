
## Get Daily Habits

- **Method:** `GET`
- **Endpoint:** `/habits`
- **Description:** Retrieves all habits with their logged status for a specific date. Returns today's log value, the accumulated period value (based on each habit's frequency), targets, and streak information.
- **Query Parameters:**
  - `date` (optional): The date for which to retrieve habits, in `YYYY-MM-DD` format. Defaults to today.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** An array of habit objects.
    ```json
    [
      {
        "id": 1,
        "name": "Exercise",
        "description": "Go for a 30-minute run.",
        "frequency": "weekly",
        "target_min": 5,
        "target_max": null,
        "recording_required": true,
        "log_value": 1,
        "period_value": 3,
        "current_streak": 4,
        "longest_streak": 12
      },
      {
        "id": 2,
        "name": "Weight",
        "description": "Body weight in kg",
        "frequency": "daily",
        "target_min": 60,
        "target_max": 80,
        "recording_required": false,
        "log_value": 70.5,
        "period_value": 70.5,
        "current_streak": 14,
        "longest_streak": 14
      }
    ]
    ```
  - **Fields:**
    - `log_value`: The log value for the specific requested date (null if not logged).
    - `period_value`: Sum of all log values within the current period (daily/weekly/monthly depending on the habit's frequency).
    - `frequency`: The habit's tracking frequency (`daily`, `weekly`, or `monthly`).
    - `target_min`: The minimum target value per period (null if not set).
    - `target_max`: The maximum target value per period (null if not set).
    - `recording_required`: Whether missing days break the streak (true) or carry forward the last value (false).
    - `current_streak`: Number of consecutive completed periods (where period value meets target criteria).
    - `longest_streak`: All-time highest streak.
- **Error Response:**
  - **Code:** `500 Internal Server Error`
  - **Content:** `Internal Server Error`

---

## Upsert Habit Log

- **Method:** `POST`
- **Endpoint:** `/habits/log`
- **Description:** Creates or updates a log entry for a specific habit on a given date. After upserting, the habit's streak is recalculated.
- **Request Body:**
  ```json
  {
    "habit_id": 1,
    "date": "2023-10-27",
    "value": 1.5
  }
  ```
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "status": "ok"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to log`

---

## Create Habit

- **Method:** `POST`
- **Endpoint:** `/habits`
- **Description:** Creates a new habit with optional frequency and targets.
- **Request Body:**
  ```json
  {
    "name": "Exercise",
    "description": "Go for a 30-minute run.",
    "frequency": "weekly",
    "target_min": 5,
    "target_max": null,
    "recording_required": true
  }
  ```
  - `name` (required): The name of the habit.
  - `description` (optional): A description of the habit.
  - `frequency` (optional): `daily`, `weekly`, or `monthly`. Defaults to `daily`.
  - `target_min` (optional): Minimum target value per period (must be >= 0).
  - `target_max` (optional): Maximum target value per period (must be >= 0, must be >= target_min if both set).
  - `recording_required` (optional): Whether missing days break the streak. Defaults to `true`.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "name": "Exercise",
      "description": "Go for a 30-minute run.",
      "frequency": "weekly",
      "target_min": 5,
      "target_max": null,
      "recording_required": true,
      "current_streak": 0,
      "longest_streak": 0
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `name is required`, `frequency must be daily, weekly, or monthly`, `target_min must be >= 0`, `target_max must be >= 0`, or `target_min must be <= target_max`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create habit`

---

## Get Habit History

- **Method:** `GET`
- **Endpoint:** `/habits/{id}/history`
- **Description:** Returns aggregated habit log values over a date range, bucketed by a chosen frequency. When the requested frequency is coarser than the habit's native frequency (e.g. viewing a daily habit as weekly), values are averaged instead of summed. For habits with `recording_required=true`, missing periods are filled with zero values.
- **Path Parameters:**
  - `id` (required): The habit ID.
- **Query Parameters:**
  - `frequency` (optional): `daily`, `weekly`, or `monthly`. Defaults to the habit's own frequency.
  - `start_at` (optional): Start date in `YYYY-MM-DD` format. Default depends on frequency: daily = 1 month ago, weekly = 12 weeks ago, monthly = 12 months ago.
  - `end_at` (optional): End date in `YYYY-MM-DD` format. Defaults to today.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** A wrapper object containing the date range and history data. The `start_at` and `end_at` values are truncated to the period boundary (Monday for weekly, 1st of month for monthly).
    ```json
    {
      "start_at": "2026-02-23",
      "end_at": "2026-03-16",
      "data": [
        {
          "date": "2026-03-02",
          "value": 15.0
        },
        {
          "date": "2026-03-09",
          "value": 12.0
        }
      ]
    }
    ```
  - **Fields:**
    - `start_at`: Start of the date range, truncated to the period boundary (`YYYY-MM-DD`).
    - `end_at`: End of the date range, truncated to the period boundary (`YYYY-MM-DD`).
    - `data`: Array of history points.
    - `data[].date`: The start date of the aggregation period (`YYYY-MM-DD`).
    - `data[].value`: Sum of log values within the period (or average, when viewing at a coarser frequency than the habit's native one).
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid habit id` or `frequency must be daily, weekly, or monthly`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get history`

---

## Delete Habit

- **Method:** `DELETE`
- **Endpoint:** `/habits/{id}`
- **Description:** Deletes a habit and all its associated log entries.
- **Path Parameters:**
  - `id` (required): The habit ID.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid habit id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete habit`
