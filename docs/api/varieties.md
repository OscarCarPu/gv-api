
## Weed Varieties

CRUD for catalog entries describing strains rated on four sensory/effect axes.

**Auth:** semi-private. Endpoints accept either a `semi` or a `full` token (see [auth.md](auth.md)).

**Audit:** every mutation is recorded in `weed_varieties_history` (see [data_models/varieties.md](../data_models/varieties.md)). The server stamps each history row with `actor_ip`, `actor_user_agent`, `actor_token_kind` and the `X-Device-ID` header (a stable per-browser UUID set by the web client). DELETE is a soft delete — the row is hidden from list/get but kept on disk for recovery.

**Resource fields:**

| Field      | Type           | Notes                                                                       |
|------------|----------------|-----------------------------------------------------------------------------|
| `id`       | integer        | Server-assigned.                                                            |
| `name`     | string         | Required, max 40 chars.                                                     |
| `scent`    | float          | Required, range `[0, 10]`.                                                  |
| `flavor`   | float          | Required, range `[0, 10]`.                                                  |
| `power`    | float          | Required, range `[0, 10]`.                                                  |
| `quality`  | float          | Required, range `[0, 10]`.                                                  |
| `score`    | float          | Read-only. Auto-computed as `(scent + flavor + power + quality) / 4`.       |
| `price`    | float          | Required. No bounds.                                                        |
| `comments` | string \| null | Optional free-form notes.                                                   |
| `judge`    | string         | Required, max 40 chars. Self-reported name of the rater (defaults to `Oscar` in the UI). |

---

## List Varieties

- **Method:** `GET`
- **Endpoint:** `/varieties`
- **Description:** Returns every variety, ordered by `score DESC, price ASC` (best score first; within the same score, cheapest first).
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      {
        "id": 1,
        "name": "Northern Lights",
        "scent": 8.5,
        "flavor": 8,
        "power": 9,
        "quality": 9.5,
        "score": 8.75,
        "price": 12,
        "comments": "Classic indica, deeply relaxing",
        "judge": "Oscar"
      }
    ]
    ```
- **Error Response:**
  - **Code:** `500 Internal Server Error` — `Failed to list varieties`

---

## Get Variety

- **Method:** `GET`
- **Endpoint:** `/varieties/{id}`
- **Path Parameters:**
  - `id` (required): The variety ID.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** A single variety object (same shape as items in the list response).
- **Error Responses:**
  - **Code:** `400 Bad Request` — `invalid variety id`
  - **Code:** `404 Not Found` — `variety not found`
  - **Code:** `500 Internal Server Error` — `Failed to get variety`

---

## Create Variety

- **Method:** `POST`
- **Endpoint:** `/varieties`
- **Request Body:**
  ```json
  {
    "name": "Test Strain",
    "scent": 7.5,
    "flavor": 8,
    "power": 6,
    "quality": 7,
    "price": 9.99,
    "comments": "optional notes",
    "judge": "Oscar"
  }
  ```
- **Success Response:**
  - **Code:** `201 Created`
  - **Content:** The created variety object including the auto-computed `score`.
- **Error Responses:**
  - **Code:** `400 Bad Request` — `Invalid Body`, `name is required`, `name must be at most 40 characters`, `judge is required`, `judge must be at most 40 characters`, or `<field> must be between 0 and 10` (where `<field>` is `scent`/`flavor`/`power`/`quality`)
  - **Code:** `500 Internal Server Error` — `Failed to create variety`

---

## Update Variety

- **Method:** `PUT`
- **Endpoint:** `/varieties/{id}`
- **Description:** Replaces all editable fields. `score` is recomputed from the new sensory values.
- **Path Parameters:**
  - `id` (required): The variety ID.
- **Request Body:** Same shape as Create.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:** The updated variety object.
- **Error Responses:**
  - **Code:** `400 Bad Request` — same validation errors as Create, plus `invalid variety id`
  - **Code:** `404 Not Found` — `variety not found`
  - **Code:** `500 Internal Server Error` — `Failed to update variety`

---

## Delete Variety

- **Method:** `DELETE`
- **Endpoint:** `/varieties/{id}`
- **Description:** Soft delete — sets `deleted_at = now()`. The row disappears from `GET /varieties` and `GET /varieties/{id}` but remains on disk and can be restored manually with `UPDATE weed_varieties SET deleted_at = NULL WHERE id = $1`.
- **Path Parameters:**
  - `id` (required): The variety ID.
- **Success Response:**
  - **Code:** `204 No Content`
- **Error Responses:**
  - **Code:** `400 Bad Request` — `invalid variety id`
  - **Code:** `500 Internal Server Error` — `Failed to delete variety`
