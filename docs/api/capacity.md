# Capacity

How many hours are free on each of the next few days, so Due Soon can tell a task that
genuinely needs to start today from one that has weeks of slack. **Auth:** full-private.

**Business rules:** see [business_logic/plan.md](business_logic/plan.md) (capacity reads
`plan_blocks`, owned by the `plan` domain — there is no `capacity`-owned table).

## What "capacity" means here

- A single constant: the same number of theoretical free hours, every day of the year. Not a
  time-of-day window, not different on weekends — just a flat daily total.
- **Not editable from the app.** It is the `DAILY_CAPACITY_HOURS` environment variable
  (default `14`), read once at startup. Changing it means changing the deployment's env, not
  calling an endpoint — there is deliberately no `PUT` for it.
- "Busy" hours per day come from `plan_blocks` with a `task_id` and/or an `event_ref` set (see
  [business_logic/plan.md](business_logic/plan.md)) — calendar events themselves are never
  read directly; only a block someone actually committed to counts.
- `free_hours = max(capacity_hours - busy_hours, 0)`, computed independently per day.

## Get Free/Busy Range

- **Method:** `GET`
- **Endpoint:** `/capacity/free-busy`
- **Description:** Returns the daily capacity/busy/free breakdown for a date range.
- **Query Parameters:**
  - `from` (required): `YYYY-MM-DD`.
  - `to` (required): `YYYY-MM-DD`, exclusive, must be after `from`. Range capped at 90 days.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "from": "2026-08-29",
      "to": "2026-09-05",
      "days": [
        {
          "date": "2026-08-29",
          "capacity_hours": "14",
          "busy_hours": "0",
          "free_hours": "14"
        },
        {
          "date": "2026-08-30",
          "capacity_hours": "14",
          "busy_hours": "3.5",
          "free_hours": "10.5"
        }
      ]
    }
    ```
  - All hour values are decimal strings (same convention as `finance` money fields), not floats.
  - `capacity_hours` is identical across every day in the response — it is the constant, repeated for convenience so a client never has to fetch it separately.
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid or missing from`, `invalid or missing to`, `to must be after from`, or `range is longer than 90 days`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get free/busy range`
