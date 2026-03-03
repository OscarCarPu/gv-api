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
