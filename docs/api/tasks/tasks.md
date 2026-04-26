# Tasks

See [README](README.md) for shared `task_type` / `recurrence` / `priority` semantics.

## List Tasks (Fast)

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/list-fast`
- **Description:** Returns all unfinished tasks (where `finished_at` is null) with `id`, `name`, `project_id`, and `project_name`. Tasks are ordered by project tree (DFS pre-order): for a tree A → B → C, A → D, E the order is A, B, C, D, E. Tasks without project appear at the end. Within each project group, tasks are sorted by name.
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
    "priority": 2
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
      "depends_on": [{"id": 2, "name": "Other Task", "due_at": "2025-05-15"}, {"id": 3, "name": "Another Task", "due_at": null}],
      "blocks": [],
      "blocked": true
    }
    ```
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
    "priority": 1
  }
  ```
  - `name` (optional): New name.
  - `description` (optional): New description.
  - `due_at` (optional): New due date. Pass `null` to clear the due date. Omitting the field leaves it unchanged.
  - `project_id` (optional): New parent project ID.
  - `started_at` (optional): Start timestamp.
  - `finished_at` (optional): Finish timestamp.
  - `depends_on` (optional): List of task IDs this task depends on. Replaces all existing dependencies. Omitting the field leaves dependencies unchanged. Pass `[]` to clear all dependencies.
  - `task_type` (optional): One of `"standard"`, `"continuous"`, or `"recurring"`. When changing to `"recurring"`, `recurrence` must be provided. When changing to a non-recurring type, `recurrence` is automatically cleared.
  - `recurrence` (optional): Number of days between recurrences (positive integer). Required when `task_type` is set to `"recurring"`. Rejected when `task_type` is set to a non-recurring type. Can be sent alone to change the interval of an already-recurring task.
  - `priority` (optional): Integer from 1 (highest) to 5 (lowest).
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
- **Description:** Returns unfinished tasks that have a due date (own, project, or inherited from dependencies), ordered by effective `due_at` first, then by project `due_at`, then by name. Tasks hidden by blocked dependencies are excluded. A task's `due_at` in the response reflects its effective due date (minimum of own and dependencies'). Includes time spent from completed time entries.
- **Query Parameters:**
  - `min_priority` (optional): Integer from 1 to 5. When provided, only tasks with `priority <= min_priority` are returned (1 = highest importance).
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
        "priority": 2,
        "time_spent": 5400,
        "project_id": 1,
        "project_name": "My Project",
        "project_due_at": "2025-12-31",
        "depends_on": [{"id": 2, "name": "Blocking Task", "due_at": null}],
        "blocks": [{"id": 4, "name": "Dep A", "due_at": null}, {"id": 5, "name": "Dep B", "due_at": null}],
        "blocked": true
      }
    ]
    ```
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
