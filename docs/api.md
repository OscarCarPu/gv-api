# GV-API Endpoints

This document provides details on the available endpoints for the GV-API.

---

## Authentication

The API uses a two-step JWT authentication flow:

1. **POST /login** — submit your password to receive a short-lived temporary token (`kind: tmp`, 5 min).
2. **POST /login/2fa** — submit the temporary token and your TOTP code to receive a full month token (`kind: full`, 30 days).

All protected endpoints require the full token in the `Authorization` header:

```
Authorization: Bearer <full-token>
```

Error responses are JSON: `{"error": "<message>"}`.

---

## Login

- **Method:** `POST`
- **Endpoint:** `/login`
- **Description:** Authenticates with a password and returns a temporary token to be used in the 2FA step.
- **Request Body:**
  ```json
  { "password": "your-password" }
  ```
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    { "token": "<tmp-jwt>" }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request` — `{"error": "Invalid Request"}`
  - **Code:** `401 Unauthorized` — `{"error": "invalid password"}`

---

## Login 2FA

- **Method:** `POST`
- **Endpoint:** `/login/2fa`
- **Description:** Validates the temporary token and a TOTP code, returning a full month token that grants access to protected endpoints.
- **Request Body:**
  ```json
  { "token": "<tmp-jwt>", "code": "123456" }
  ```
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    { "token": "<full-jwt>" }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request` — `{"error": "Invalid Request"}`
  - **Code:** `401 Unauthorized` — `{"error": "invalid token"}` or `{"error": "invalid 2fa code"}`

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
