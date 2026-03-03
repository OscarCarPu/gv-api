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
