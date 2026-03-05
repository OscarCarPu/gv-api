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
        "parent_id": null
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
      "parent_id": 1
    }
    ```

- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body` or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create project`

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
    "due_at": "2025-06-01"
  }
  ```
  - `name` (required): The name of the task.
  - `project_id` (optional): The ID of the parent project.
  - `description` (optional): A description of the task.
  - `due_at` (optional): The due date of the task in `YYYY-MM-DD` format.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "project_id": 1,
      "name": "My Task",
      "description": "Task description.",
      "due_at": "2025-06-01"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body` or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create task`

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
      "name": "My Todo"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`, `task_id is required`, or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create todo`

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

## Finish Time Entry

- **Method:** `PATCH`
- **Endpoint:** `/tasks/time-entries/:id/finish`
- **Description:** Marks a time entry as finished. If `finished_at` is omitted, the server uses the current time.
- **Request Body (optional):**
  ```json
  {
    "finished_at": "2025-03-01T10:30:00Z"
  }
  ```
  - `finished_at` (optional): The end time in RFC 3339 format. Defaults to NOW().
- **Success Response:**
  - **Code:** `200 OK`
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
    - **Content:** `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `Time entry not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to finish time entry`

## Finish Task

- **Method:** `PATCH`
- **Endpoint:** `/tasks/tasks/:id/finish`
- **Description:** Marks a task as finished by setting its `finished_at` timestamp. If `finished_at` is omitted, the server uses the current time.
- **Request Body (optional):**
  ```json
  {
    "finished_at": "2025-03-01T17:00:00Z"
  }
  ```
  - `finished_at` (optional): The finish time in RFC 3339 format. Defaults to NOW().
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "project_id": 1,
      "name": "My Task",
      "description": "Task description.",
      "due_at": "2025-06-01",
      "started_at": "2025-02-15T08:00:00Z",
      "finished_at": "2025-03-01T17:00:00Z"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `Task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to finish task`

## Finish Project

- **Method:** `PATCH`
- **Endpoint:** `/tasks/projects/:id/finish`
- **Description:** Marks a project as finished by setting its `finished_at` timestamp. If `finished_at` is omitted, the server uses the current time.
- **Request Body (optional):**
  ```json
  {
    "finished_at": "2025-03-01T17:00:00Z"
  }
  ```
  - `finished_at` (optional): The finish time in RFC 3339 format. Defaults to NOW().
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "id": 1,
      "parent_id": null,
      "name": "My Project",
      "description": "This is my project.",
      "due_at": "2025-01-01",
      "started_at": "2024-12-01T08:00:00Z",
      "finished_at": "2025-03-01T17:00:00Z"
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body`
  - **Code:** `404 Not Found`
    - **Content:** `Project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to finish project`

## Get Active Tree

- **Method:** `GET`
- **Endpoint:** `/tasks/tree`
- **Description:** Returns a nested JSON tree of active projects and tasks. Projects are included if `started_at IS NOT NULL` and `finished_at IS NULL`. Tasks are included if `finished_at IS NULL`. Orphan tasks (no `project_id`) appear at the root level. Ordered: first projects, then tasks with `started_at IS NOT NULL` then tasks with `started_at IS NULL`.
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
            "type": "task"
            "name": "task_1",
          }
        ]
      },
      {
        "id": 5,
        "type": "task"
        "name": "orphan_task",
      }
    ]
    ```
- **Error Responses:**
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get active tree`

## Get Project Children

- **Method:** `GET`
- **Endpoint:** `/tasks/projects/:id/children`
- **Description:** Returns sub-projects and tasks belonging to the given project. Results are ordered: sub-projects first, then unfinished tasks, then finished tasks. Tasks include their `todos` array and `time_spent` (total seconds from time entries). Sub-projects include `time_spent` (recursive sum across all nested tasks).
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
          "todos": [
            {
              "id": 1,
              "task_id": 1,
              "name": "My Todo"
            }
          ]
        }
      ]
    }
    ```
- **Error Responses:**
  - **Code:** `404 Not Found`
    - **Content:** `Project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get project children`

## Get Task Time Entries

- **Method:** `GET`
- **Endpoint:** `/tasks/tasks/:id/time-entries`
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
        "time_spent": 5400
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
    - **Content:** `Task not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get time entries`
