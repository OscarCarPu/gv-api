# GV-API Endpoints

This document provides details on the available endpoints for the GV-API.

---

## Get Daily Habits

- **Method:** `GET`
- **Endpoint:** `/habits`
- **Description:** Retrieves a list of all habits and their logged status for a specific date. If no date is provided, it defaults to the current day.
- **Query Parameters:**
  - `date` (optional): The date for which to retrieve habits, in `YYYY-MM-DD` format.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** An array of `HabitWithLog` objects.
    ```json
    [
      {
        "id": "123",
        "name": "Exercise",
        "description": "Go for a 30-minute run.",
        "log_value": "done"
      },
      {
        "id": "456",
        "name": "Read",
        "description": "Read for 15 minutes.",
        "log_value": null
      }
    ]
    ```
- **Error Response:**
  - **Code:** `500 Internal Server Error`
  - **Content:** `Internal Server Error`

---

## Upsert Habit Log

- **Method:** `POST`
- **Endpoint:** `/habits/log`
- **Description:** Creates or updates a log entry for a specific habit on a given date.
- **Request Body:**
  ```json
  {
    "habit_id": "123",
    "date": "2023-10-27",
    "value": "done"
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
- **Description:** Creates a new habit.
- **Request Body:**
  ```json
  {
    "name": "Exercise",
    "description": "Go for a 30-minute run."
  }
  ```
  - `name` (required): The name of the habit.
  - `description` (optional): A description of the habit.
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:**
    ```json
    {
      "id": 1,
      "name": "Exercise",
      "description": "Go for a 30-minute run."
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request`
    - **Content:** `Invalid Body` or `name is required`
  - **Code:** `500 Internal Server Error`
    - **Content:** `Failed to create habit`
