# Projects

See [README](README.md) for shared `task_type` / `recurrence` / `priority` semantics referenced by tree and children responses.

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

## Get Project

- **Method:** `GET`
- **Endpoint:** `/tasks/projects/{id}`
- **Description:** One project with its recursive `time_spent`. Children are at `/tasks/projects/{id}/children`.
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
      "finished_at": null,
      "time_spent": 7200
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid project id`
  - **Code:** `404 Not Found`
    - **Content:** `project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to get project`

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
  - `due_at` (optional): New due date. Pass `null` to clear the due date. Omitting the field leaves it unchanged.
  - `parent_id` (optional): new parent; `null` moves it to the root, omitted leaves it. The subtree moves too. Cannot be the project itself or a descendant.
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
    - **Content:** `invalid project id`, `Invalid Body` or `parent project not found`
  - **Code:** `404 Not Found`
    - **Content:** `project not found`
  - **Code:** `409 Conflict`
    - **Content:** `a project cannot be moved under itself or one of its sub-projects`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to update project`

## Get Project Parent Candidates

- **Method:** `GET`
- **Endpoint:** `/tasks/projects/{id}/parent-candidates`
- **Description:** Projects `{id}` may be moved under: all except itself, its descendants and finished ones. The current parent is always included. Ordered by `path`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      { "id": 1, "name": "Home", "path": "Home" },
      { "id": 4, "name": "Kitchen", "path": "Home / Renovation / Kitchen" }
    ]
    ```
  - `path` is the full ancestor chain joined with ` / `, for display and to tell apart projects that share a name.
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid project id`
  - **Code:** `404 Not Found`
    - **Content:** `project not found`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to list parent candidates`

## Delete Project

- **Method:** `DELETE`
- **Endpoint:** `/tasks/projects/{id}`
- **Description:** Deletes a project. Its tasks are not deleted with it; finish the project first to close its descendants.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid project id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete project`

## Get Active Tree

- **Method:** `GET`
- **Endpoint:** `/tasks/tree`
- **Description:** Nested tree of active projects (started, unfinished) and unfinished tasks, orphans at the root. Projects first, then started tasks, then unstarted. `due_at` is the effective due date.
- **Query Parameters:**
  - `min_priority` (optional): 1–5. Keeps tasks with `priority <= min_priority`; projects are always kept.
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
            "task_type": "standard",
            "priority": 2,
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
        "task_type": "continuous",
        "priority": 4,
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
- **Description:** Sub-projects and tasks of the project: sub-projects, started tasks, unstarted tasks, finished tasks; ties by `due_at` (nulls last), then name. Tasks include `todos` and `time_spent`; sub-projects a recursive `time_spent`.
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
          "task_type": "standard",
          "priority": 3,
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
