# Rutas

CRUD for concello marks: the record of which councils have been visited and when.

**Auth:** full-private. All endpoints require a `full` token (see [auth.md](auth.md)).

### Mark fields

| Field         | Type    | Notes                                                     |
|---------------|---------|-----------------------------------------------------------|
| `id`          | integer | Server-assigned.                                          |
| `name`        | string  | Council name, 1–200 chars. Set on create, never updated.  |
| `visited_on`  | string  | Date, `YYYY-MM-DD`.                                       |
| `description` | string  | Free-form, defaults to `""`.                              |

---

## List Marks

- **Method:** `GET`
- **Endpoint:** `/rutas/marks`
- **Description:** All marks, most recently visited first, ties broken by name.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "Ourense",
        "visited_on": "2026-04-12T00:00:00Z",
        "description": "termas"
      }
    ]
    ```

---

## Get Mark

- **Method:** `GET`
- **Endpoint:** `/rutas/marks/{id}`
- **Success Response:** `200 OK` with the mark.
- **Error Responses:**
  - `400 Bad Request` — invalid id.
  - `404 Not Found` — no mark with that id.

---

## Create Mark

- **Method:** `POST`
- **Endpoint:** `/rutas/marks`
- **Body:**
  ```json
  {
    "name": "Ourense",
    "visited_on": "2026-04-12",
    "description": "termas"
  }
  ```
- **Success Response:** `201 Created` with the created mark.
- **Error Responses:**
  - `400 Bad Request` — invalid body, missing or over-long `name` (max 200),
    missing `visited_on`, or a `visited_on` that is not `YYYY-MM-DD`.

---

## Update Mark

- **Method:** `PUT`
- **Endpoint:** `/rutas/marks/{id}`
- **Description:** Updates `visited_on` and `description`. The name is fixed at creation.
- **Body:**
  ```json
  {
    "visited_on": "2026-04-13",
    "description": "termas y casco vello"
  }
  ```
- **Success Response:** `200 OK` with the updated mark.
- **Error Responses:**
  - `400 Bad Request` — invalid id, invalid body, or missing/malformed `visited_on`.
  - `404 Not Found` — no mark with that id.

---

## Delete Mark

- **Method:** `DELETE`
- **Endpoint:** `/rutas/marks/{id}`
- **Success Response:** `204 No Content`.
- **Error Responses:**
  - `400 Bad Request` — invalid id.
