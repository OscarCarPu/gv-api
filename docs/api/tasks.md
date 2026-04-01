## List Projects (Fast)

- **Method:** `GET`
- **Endpoint:** `/tasks/projects/list-fast`
- **Description:** Returns all active projects (where `finished_at` is null) as a flat list with only `id` and `name`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "My Project"
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to list projects`

## List Tasks (Fast)

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/list-fast`
- **Description:** Returns all unfinished tasks (where `finished_at` is null) as a flat list with only `id` and `name`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "My Task"
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to list tasks`

## Get Root Projects

- **Method:** `GET`
- **Endpoint:** `/tasks/projects`
- **Description:** Returns all root projects (no parent, not finished).
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "My Project",
        "description": "This is my project.",
        "due_at": "2025-01-01",
        "parent_id": null,
        "started_at": null,
        "finished_at": null
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get projects`

## Create Project

- **Method:** `POST`
- **Endpoint:** `/tasks/projects`
- **Description:** Creates a new project.
- **Request Body:**
  ```json
  {
    "name": "My Project",
    "description": "This is my project.",
    "due_at": "2025-01-01",
    "parent_id": 1
  }
  ```
  - `name` (required): The name of the project.
  - `description` (optional): A description of the project.
  - `due_at` (optional): The due date of the project in `YYYY-MM-DD` format.
  - `parent_id` (optional): The ID of the parent project.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "name": "My Project",
      "description": "This is my project.",
      "due_at": "2025-01-01",
      "parent_id": 1,
      "started_at": null,
      "finished_at": null
    }
    ```

- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body` or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create project`

## Update Project

- **Method:** `PATCH`
- **Endpoint:** `/tasks/projects/{id}`
- **Description:** Partially updates a project. Only provided fields are modified.
- **Request Body:**
  ```json
  {
    "name": "Renamed Project",
    "description": "Updated description.",
    "due_at": "2025-06-01",
    "parent_id": 2,
    "started_at": "2025-01-01T08:00:00Z",
    "finished_at": "2025-03-01T17:00:00Z"
  }
  ```
  - `name` (optional): New name.
  - `description` (optional): New description.
  - `due_at` (optional): New due date.
  - `parent_id` (optional): New parent project ID.
  - `started_at` (optional): Start timestamp.
  - `finished_at` (optional): Finish timestamp.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "name": "Renamed Project",
      "description": "Updated description.",
      "due_at": "2025-06-01",
      "parent_id": 2,
      "started_at": "2025-01-01T08:00:00Z",
      "finished_at": "2025-03-01T17:00:00Z"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid project id` or `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update project`

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
    "depends_on": [2, 3]
  }
  ```
  - `name` (required): The name of the task.
  - `project_id` (optional): The ID of the parent project.
  - `description` (optional): A description of the task.
  - `due_at` (optional): The due date of the task in `YYYY-MM-DD` format.
  - `depends_on` (optional): List of task IDs this task depends on.
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
    - **Content:** `Invalid Body` or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create task`

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
    "depends_on": [3, 4]
  }
  ```
  - `name` (optional): New name.
  - `description` (optional): New description.
  - `due_at` (optional): New due date.
  - `project_id` (optional): New parent project ID.
  - `started_at` (optional): Start timestamp.
  - `finished_at` (optional): Finish timestamp.
  - `depends_on` (optional): List of task IDs this task depends on. Replaces all existing dependencies. Omitting the field leaves dependencies unchanged. Pass `[]` to clear all dependencies.
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
      "depends_on": [{"id": 3, "name": "Dep A", "due_at": null}, {"id": 4, "name": "Dep B", "due_at": "2025-07-01"}],
      "blocks": [{"id": 7, "name": "Blocked Task", "due_at": null}],
      "blocked": true
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid task id` or `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update task`

## Create Todo

- **Method:** `POST`
- **Endpoint:** `/tasks/todos`
- **Description:** Creates a new todo item within a task.
- **Request Body:**
  ```json
  {
    "task_id": 1,
    "name": "My Todo"
  }
  ```
  - `task_id` (required): The ID of the parent task.
  - `name` (required): The name of the todo.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "task_id": 1,
      "name": "My Todo",
      "is_done": false
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `task_id is required`, or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create todo`

## Update Todo

- **Method:** `PATCH`
- **Endpoint:** `/tasks/todos/{id}`
- **Description:** Partially updates a todo. Can change name and/or toggle done status.
- **Request Body:**
  ```json
  {
    "name": "Renamed Todo",
    "is_done": true
  }
  ```
  - `name` (optional): New name.
  - `is_done` (optional): New done status.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "task_id": 1,
      "name": "Renamed Todo",
      "is_done": true
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid todo id` or `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `todo not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update todo`

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
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create time entry`

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

## Get Tasks by Due Date

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/by-due-date`
- **Description:** Returns unfinished tasks that have a due date (own, project, or inherited from dependencies), ordered by effective `due_at` first, then by project `due_at`, then by name. Tasks hidden by blocked dependencies are excluded. A task's `due_at` in the response reflects its effective due date (minimum of own and dependencies'). Includes time spent from completed time entries.
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

## Get Active Tree

- **Method:** `GET`
- **Endpoint:** `/tasks/tree`
- **Description:** Returns a nested JSON tree of active projects and tasks. Projects are included if `started_at IS NOT NULL` and `finished_at IS NULL`. Tasks are included if `finished_at IS NULL`. Orphan tasks (no `project_id`) appear at the root level. Ordered: first projects, then tasks with `started_at IS NOT NULL` then tasks with `started_at IS NULL`. Tasks hidden by blocked dependencies are excluded (a task is hidden if all of its dependencies are themselves blocked). Task `due_at` reflects the effective due date (minimum of own and dependencies').
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "type": "project",
        "name": "project_1",
        "children": [
          {
            "id": 2,
            "type": "project",
            "name": "project_2",
            "children": []
          },
          {
            "id": 1,
            "type": "task",
            "name": "task_1",
            "depends_on": [{"id": 3, "name": "Blocking Task", "due_at": "2025-06-01"}],
            "blocks": [],
            "blocked": true
          }
        ]
      },
      {
        "id": 5,
        "type": "task",
        "name": "orphan_task",
        "depends_on": [],
        "blocks": [{"id": 1, "name": "task_1", "due_at": null}],
        "blocked": false
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get active tree`

## Get Project Children

- **Method:** `GET`
- **Endpoint:** `/tasks/projects/{id}/children`
- **Description:** Returns sub-projects and tasks belonging to the given project. Results are ordered: sub-projects first, then started tasks, then unstarted tasks, then finished tasks. Ties within each group are broken by `due_at` ascending (nulls last), then name. Tasks include their `todos` array and `time_spent` (total seconds from time entries). Sub-projects include `time_spent` (recursive sum across all nested tasks).
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "project": {
        "id": 1,
        "parent_id": null,
        "name": "My Project",
        "description": "This is my project.",
        "due_at": "2025-01-01",
        "started_at": "2024-12-01T08:00:00Z",
        "finished_at": null,
        "time_spent": 7200
      },
      "children": [
        {
          "id": 2,
          "type": "project",
          "parent_id": 1,
          "name": "Sub-Project",
          "description": "A sub-project.",
          "due_at": "2025-06-01",
          "started_at": "2025-01-15T08:00:00Z",
          "finished_at": null,
          "time_spent": 7200
        },
        {
          "id": 1,
          "type": "task",
          "project_id": 1,
          "name": "My Task",
          "description": "Task description.",
          "due_at": "2025-06-01",
          "started_at": "2025-02-15T08:00:00Z",
          "finished_at": null,
          "time_spent": 5400,
          "depends_on": [{"id": 3, "name": "Setup DB", "due_at": null}],
          "blocks": [],
          "blocked": true,
          "todos": [
            {
              "id": 1,
              "task_id": 1,
              "name": "My Todo",
              "is_done": false
            }
          ]
        }
      ]
    }
    ```
- **Error Responses:**
  - **Code:** `404 Not Found`
    - **Content:** `project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get project children`

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

