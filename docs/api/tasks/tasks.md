# Tasks

See [README](README.md) for shared `task_type` / `recurrence` / `priority` semantics.

## List Tasks (Fast)

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/list-fast`
- **Description:** Unfinished tasks (`id`, `name`, `project_id`, `project_name`) in project-tree pre-order (A → B → C, A → D, E gives A, B, C, D, E), by name within a project, orphans last.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "My Task",
        "project_id": 5,
        "project_name": "My Project",
        "task_type": "standard",
        "priority": 3
      },
      {
        "id": 2,
        "name": "Orphan Task",
        "project_id": null,
        "project_name": null,
        "task_type": "recurring",
        "recurrence": 7,
        "priority": 1
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to list tasks`

## Create Task

- **Method:** `POST`
- **Endpoint:** `/tasks/tasks`
- **Description:** Creates a new task.
- **Request Body:**
  ```json
  {
    "project_id": 1,
    "name": "My Task",
    "description": "Task description.",
    "due_at": "2025-06-01",
    "depends_on": [2, 3],
    "task_type": "recurring",
    "recurrence": 7,
    "priority": 2,
    "estimate_hours": "3.5"
  }
  ```
  - `name` (required): The name of the task.
  - `project_id` (optional): The ID of the parent project.
  - `description` (optional): A description of the task.
  - `due_at` (optional): The due date of the task in `YYYY-MM-DD` format.
  - `depends_on` (optional): List of task IDs this task depends on.
  - `task_type` (optional): One of `"standard"` (default), `"continuous"`, or `"recurring"`.
  - `recurrence` (required when `task_type` is `"recurring"`, rejected otherwise): Number of days between recurrences (positive integer).
  - `priority` (optional): Integer from 1 (highest) to 5 (lowest). Defaults to 3.
  - `estimate_hours` (optional): decimal hours string, > 0. Drives urgency for `standard` tasks.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "project_id": 1,
      "name": "My Task",
      "description": "Task description.",
      "due_at": "2025-06-01",
      "started_at": null,
      "finished_at": null,
      "task_type": "recurring",
      "recurrence": 7,
      "priority": 2,
      "estimate_hours": "3.5",
      "depends_on": [{"id": 2, "name": "Other Task", "due_at": "2025-05-15"}, {"id": 3, "name": "Another Task", "due_at": null}],
      "blocks": [],
      "blocked": true
    }
    ```
  - `estimate_hours`: echoes the request, `null` if not set.
  - `depends_on`: Tasks this task depends on (this task is blocked by them). Each entry contains `id`, `name`, and `due_at` (used for effective due date computation).
  - `blocks`: Tasks that depend on this task (they are blocked by this task). Each entry contains `id`, `name`, and `due_at`.
  - `blocked`: `true` if the task has at least one unfinished dependency, `false` otherwise.
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `name is required`, `task_type must be standard, continuous, or recurring`, `recurrence is required when task_type is recurring`, `recurrence is only valid when task_type is recurring`, or `recurrence must be a positive number of days`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create task`

## Get Task

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/{id}`
- **Description:** Returns a single task with its dependencies, todos, and `time_spent`. For the time entries themselves, use `/tasks/tasks/{id}/time-entries`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "project_id": 5,
      "project_name": "My Project",
      "name": "Implement feature X",
      "description": "...",
      "due_at": "2025-04-01",
      "started_at": "2025-03-01T09:00:00Z",
      "finished_at": null,
      "task_type": "standard",
      "priority": 3,
      "time_spent": 5400,
      "depends_on": [{"id": 2, "name": "Setup DB", "due_at": null}],
      "blocks": [{"id": 4, "name": "Write tests", "due_at": null}],
      "blocked": true,
      "todos": [
        {"id": 1, "task_id": 1, "name": "My Todo", "is_done": false}
      ]
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid task id`
  - **Code:** `404 Not Found`
    - **Content:** `task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get task`

## Update Task

- **Method:** `PATCH`
- **Endpoint:** `/tasks/tasks/{id}`
- **Description:** Partially updates a task. Only provided fields are modified. Setting `depends_on` replaces all existing dependencies.
- **Request Body:**
  ```json
  {
    "name": "Renamed Task",
    "description": "Updated description.",
    "due_at": "2025-06-01",
    "project_id": 2,
    "started_at": "2025-02-15T08:00:00Z",
    "finished_at": "2025-03-01T17:00:00Z",
    "depends_on": [3, 4],
    "task_type": "recurring",
    "recurrence": 7,
    "priority": 1,
    "estimate_hours": "2"
  }
  ```
  - `name` (optional): New name.
  - `description` (optional): New description.
  - `due_at` (optional): New due date. Pass `null` to clear the due date. Omitting the field leaves it unchanged.
  - `project_id` (optional): New parent project ID.
  - `started_at` (optional): Start timestamp.
  - `finished_at` (optional): Finish timestamp.
  - `depends_on` (optional): task IDs this task depends on. Replaces all; omitted leaves them; `[]` clears.
  - `blocks` (optional): task IDs that depend on this task. Replaces all; omitted leaves them; `[]` clears.
  - `task_type` (optional): `standard`, `continuous` or `recurring`. Switching to `recurring` needs `recurrence`; switching away clears it.
  - `recurrence` (optional): days between occurrences. Required for `recurring`, rejected otherwise; can be sent alone to change the interval.
  - `priority` (optional): Integer from 1 (highest) to 5 (lowest).
  - `estimate_hours` (optional): Decimal hours string, > 0. Pass `null` to clear it. Omitting the field leaves it unchanged.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "project_id": 2,
      "name": "Renamed Task",
      "description": "Updated description.",
      "due_at": "2025-06-01",
      "started_at": "2025-02-15T08:00:00Z",
      "finished_at": "2025-03-01T17:00:00Z",
      "task_type": "recurring",
      "recurrence": 7,
      "priority": 1,
      "estimate_hours": "2",
      "depends_on": [{"id": 3, "name": "Dep A", "due_at": null}, {"id": 4, "name": "Dep B", "due_at": "2025-07-01"}],
      "blocks": [{"id": 7, "name": "Blocked Task", "due_at": null}],
      "blocked": true
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid task id`, `Invalid Body`, `task_type must be standard, continuous, or recurring`, `recurrence is required when task_type is recurring`, `recurrence is only valid when task_type is recurring`, or `recurrence must be a positive number of days`
  - **Code:** `404 Not Found`
    - **Content:** `task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update task`

## Delete Task

- **Method:** `DELETE`
- **Endpoint:** `/tasks/tasks/{id}`
- **Description:** Deletes a task. Cascades to the task's todos, time entries, and dependency edges.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid task id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete task`

## Get Tasks by Due Date

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/by-due-date`
- **Description:** Unfinished tasks with a due date (own, project's, or inherited from tasks they block), ordered by effective `due_at`, then project `due_at`, then name. `due_at` is the effective due date. Includes `time_spent` and urgency fields; see [business_logic/tasks.md](../../business_logic/tasks.md).
- **Query Parameters:**
  - `min_priority` (optional): 1–5. Only tasks with `effective_priority <= min_priority` are returned (1 = highest). Urgency is computed before filtering.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "My Task",
        "description": "Task description.",
        "due_at": "2025-06-01",
        "started_at": "2025-02-15T08:00:00Z",
        "task_type": "standard",
        "priority": 3,
        "effective_priority": 2,
        "time_spent": 5400,
        "estimate_hours": "6",
        "remaining_hours": "4.5",
        "start_by": "2025-05-30",
        "finish_by": "2025-06-01",
        "work_order": 4,
        "urgent": false,
        "project_id": 1,
        "project_name": "My Project",
        "project_due_at": "2025-12-31",
        "depends_on": [{"id": 2, "name": "Blocking Task", "due_at": null}],
        "blocks": [{"id": 4, "name": "Dep A", "due_at": null}, {"id": 5, "name": "Dep B", "due_at": null}],
        "blocked": true
      }
    ]
    ```
  - `effective_priority`: `priority` raised to the highest priority of anything the task transitively blocks.
  - `remaining_hours`, `start_by`: `null` unless the task is `standard` with an estimate.
  - `finish_by`: the due date, or earlier when a task it blocks must start first. `work_order`: position in the order work should be done (1 = first).
  - `urgent`: `start_by` is today or earlier.
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get tasks by due date`

## Get Task Time Entries

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/{id}/time-entries`
- **Description:** Returns the task details and all its time entries, along with the total time spent in seconds.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "task": {
        "id": 1,
        "project_id": 5,
        "name": "Implement feature X",
        "description": "...",
        "due_at": "2025-04-01T00:00:00Z",
        "started_at": "2025-03-01T09:00:00Z",
        "finished_at": null,
        "task_type": "standard",
        "priority": 3,
        "time_spent": 5400,
        "depends_on": [{"id": 2, "name": "Setup DB"}],
        "blocks": [{"id": 4, "name": "Write tests", "due_at": null}],
        "blocked": true
      },
      "time_entries": [
        {
          "id": 1,
          "task_id": 1,
          "started_at": "2025-03-01T09:00:00Z",
          "finished_at": "2025-03-01T10:30:00Z",
          "comment": "Worked on feature X"
        }
      ]
    }
    ```
- **Error Responses:**
  - **Code:** `404 Not Found`
    - **Content:** `task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get task time entries`
