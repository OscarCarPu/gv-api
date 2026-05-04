
## Authentication

The API issues JWTs with a `kind` claim that determines which endpoints the token can access:

| `kind` | How to obtain                                                                | Lifetime | Accepted by                          |
|--------|------------------------------------------------------------------------------|----------|--------------------------------------|
| `tmp`  | `POST /login` with the **private** password.                                 | 5 min    | Only `POST /login/2fa`.              |
| `full` | `POST /login/2fa` with a `tmp` token + TOTP code.                            | 30 days  | All protected endpoints.             |
| `semi` | `POST /login` with the **semi-private** password (no 2FA required).          | 30 days  | Semi-private endpoints only.         |

**Endpoint tiers:**

- **Public:** `POST /login`, `POST /login/2fa`.
- **Semi-private** (accept `semi` *or* `full`): `/varieties` CRUD.
- **Full-private** (require `full`): everything else (`/habits`, `/tasks/*`).

All protected endpoints expect the token in the `Authorization` header:

```
Authorization: Bearer <jwt>
```

Error responses are JSON: `{"error": "<message>"}`.

---

## Login

- **Method:** `POST`
- **Endpoint:** `/login`
- **Description:** Authenticates with a password. Returns a `tmp` token for the private password (use it with `/login/2fa`) or a `semi` token for the semi-private password (use it directly on semi-private endpoints).
- **Request Body:**
  ```json
  { "password": "your-password" }
  ```
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    { "token": "<jwt>", "kind": "tmp" }
    ```
    `kind` will be `"tmp"` for the private password or `"semi"` for the semi-private password.
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
