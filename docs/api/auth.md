
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
