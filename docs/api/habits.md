
## Get Daily Habits

- **Method:** `GET`
- **Endpoint:** `/habits`
- **Description:** Retrieves all habits with their logged status for a specific date. Returns today's log value, the accumulated period value (based on each habit's frequency), objective, and streak information.
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
        "objective": 5,
        "log_value": 1,
        "period_value": 3,
        "current_streak": 4,
        "longest_streak": 12
      },
      {
        "id": 2,
        "name": "Read",
        "description": "Read for 15 minutes.",
        "frequency": "daily",
        "objective": null,
        "log_value": null,
        "period_value": 0,
        "current_streak": 0,
        "longest_streak": 0
      }
    ]
    ```
  - **Fields:**
    - `log_value`: The log value for the specific requested date (null if not logged).
    - `period_value`: Sum of all log values within the current period (daily/weekly/monthly depending on the habit's frequency).
    - `frequency`: The habit's tracking frequency (`daily`, `weekly`, or `monthly`).
    - `objective`: The target value to reach per period (null if no objective set).
    - `current_streak`: Number of consecutive completed periods (where period value >= objective).
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
- **Description:** Creates a new habit with optional frequency and objective.
- **Request Body:**
  ```json
  {
    "name": "Exercise",
    "description": "Go for a 30-minute run.",
    "frequency": "weekly",
    "objective": 5
  }
  ```
  - `name` (required): The name of the habit.
  - `description` (optional): A description of the habit.
  - `frequency` (optional): `daily`, `weekly`, or `monthly`. Defaults to `daily`.
  - `objective` (optional): A positive number representing the target value per period. If omitted, no streak tracking is performed.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "name": "Exercise",
      "description": "Go for a 30-minute run.",
      "frequency": "weekly",
      "objective": 5,
      "current_streak": 0,
      "longest_streak": 0
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `name is required`, `frequency must be daily, weekly, or monthly`, or `objective must be a positive number`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create habit`

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
