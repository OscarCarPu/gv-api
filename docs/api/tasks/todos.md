# Todos

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

## Delete Todo

- **Method:** `DELETE`
- **Endpoint:** `/tasks/todos/{id}`
- **Description:** Deletes a todo.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `invalid todo id`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to delete todo`
