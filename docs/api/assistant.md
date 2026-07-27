
## Assistant ("Voz")

Turns a natural-language request into either a read-only SQL query or one structured write action, for the user to approve before it runs. Nothing the assistant proposes touches the database until it is approved.

**Auth:** full only. All endpoints require a `full` token (see [auth.md](auth.md)).

**Two-step flow:**

1. `POST /assistant/suggest` — the model turns the text into a *proposal* and returns it with an opaque signed `token`.
2. `POST /assistant/execute` — echo that `token` back to run the proposal.

Sending `token` *back to `/suggest`* (instead of `/execute`) together with new text is a feedback round: the model refines its prior proposal instead of starting over.

**Self-directed exploration.** While building a proposal the model may run its own read-only queries first — to find the exact stored name of an account/habit/task (a request for "leer" has to resolve to a habit actually called `Reading`), to disambiguate a vague request, or to check that its final query returns anything. These run without approval, because they cannot modify data: every one goes through the same read-only, always-rolled-back transaction as an approved read, with the same statement timeout and row/column caps. They are reported back in `steps` (see below) so a proposal's provenance is visible. Each one costs an extra model round-trip, so they are capped by `ASSISTANT_MAX_QUERIES` (default `5`; `0` disables exploration entirely). The cap is enforced server-side, not merely stated in the prompt.

**Read-query safety.** A query — proposed or exploratory — must be a single `SELECT`/`WITH` statement, and runs in a read-only transaction that is always rolled back. Postgres rejects any write, side-effecting function, or sequence advance at the engine level. Limits are configurable: `ASSISTANT_READ_TIMEOUT_MS` (default `3000`), `ASSISTANT_MAX_ROWS` (default `200`), and a fixed 40-column cap.

**Cost metering.** Every model call is recorded with its token counts and computed USD cost. One request costs one `decide` call plus one `explore` call per exploratory round (plus one `summarize` call at execute time, for reads that need one), so `interaction_count` in the usage endpoint counts `decide` rows only.

---

## Suggest

- **Method:** `POST`
- **Endpoint:** `/assistant/suggest`
- **Description:** Turns `text` into a proposal. Send `token` alongside new `text` to refine the previous proposal instead of starting fresh.
- **Request Body:**
  ```json
  {
    "text": "registra que hoy he hecho el hábito de leer",
    "token": "optional — the token of a prior suggestion, to refine it"
  }
  ```
- **Response fields:**

| Field         | Type            | Notes                                                                                              |
|---------------|-----------------|----------------------------------------------------------------------------------------------------|
| `kind`        | string          | `read` \| `write` \| `reject`.                                                                     |
| `explanation` | string          | Plain-language Spanish description of what the proposal will do, or why it was rejected.            |
| `query`       | string          | `read`: the `SELECT`. `write`: a human-readable `domain.operation {args}`. `reject`: empty.         |
| `warning`     | string          | Present on `write`: `"Esta acción modifica datos."`                                                 |
| `token`       | string          | Opaque signed token to echo back. **Absent when `kind` is `reject`.** Expires after 10 minutes.     |
| `steps`       | array \| absent | The read-only queries the model ran by itself while deciding. Absent when it ran none.               |

  Each entry in `steps`:

| Field       | Type            | Notes                                                                     |
|-------------|-----------------|---------------------------------------------------------------------------|
| `sql`       | string          | The query the model ran.                                                  |
| `row_count` | integer         | Rows returned. `0` is a legitimate answer (the thing it looked for is absent). |
| `error`     | string \| absent | Present if the query failed — e.g. it was not a read. The model is shown the error and can correct itself, so a failed step does not fail the request. |

- **Success Response:**
  - **Code:** `200 OK`
  - **Content (write, after looking the habit up):**
    ```json
    {
      "kind": "write",
      "explanation": "He registrado el hábito 'Reading' (id 2) para hoy, 2026-07-27.",
      "query": "habits.log_habit {\"habit_id\": 2, \"day\": \"2026-07-27\", \"value\": 1}",
      "warning": "Esta acción modifica datos.",
      "token": "eyJrIjoid3JpdGUi....Z_yZG9nMo2oP1SgQ",
      "steps": [
        { "sql": "SELECT id, name FROM habits", "row_count": 10 }
      ]
    }
    ```
  - **Content (reject — the lookup found nothing usable):**
    ```json
    {
      "kind": "reject",
      "explanation": "No he encontrado ningún hábito llamado 'estiramientos'. Los que existen son: Exercise, Reading, Meditation, …",
      "query": "",
      "steps": [
        { "sql": "SELECT id, name FROM habits", "row_count": 10 }
      ]
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request` — `invalid body`, `text is required`, or `invalid or expired token` (on a feedback round)
  - **Code:** `502 Bad Gateway` — `the assistant is unavailable right now` (the LLM provider failed)
  - **Code:** `500 Internal Server Error` — `Failed to build suggestion`

---

## Execute

- **Method:** `POST`
- **Endpoint:** `/assistant/execute`
- **Description:** Runs an approved proposal. For a `read`, executes the query and returns a plain-language answer (summarised by the model when the proposal asked for it, else a row count). For a `write`, dispatches the action to the owning domain service, which re-validates it and resolves any names to ids.
- **Request Body:**
  ```json
  { "token": "eyJrIjoicmVhZCI....Z_yZG9nMo2oP1SgQ" }
  ```
- **Success Response:**
  - **Code:** `200 OK`
  - **Content (read):**
    ```json
    {
      "kind": "read",
      "summary": "Has gastado 342,50 € este mes, la mayor parte en Groceries.",
      "row_count": 4
    }
    ```
  - **Content (write):**
    ```json
    {
      "kind": "write",
      "summary": "Hábito 2 registrado el 2026-07-27 (valor 1)."
    }
    ```
- **Error Responses:**
  - **Code:** `400 Bad Request` — `token is required`, `invalid or expired token`, `the proposed query is not a safe read`, `the result is too large`, `unsupported action`, or a specific validation message for an invalid action (e.g. `invalid action: no encontré "leer"`)
  - **Code:** `422 Unprocessable Entity` — `No se pudo completar la acción (puede haber dependencias o datos que lo impiden).` — the action was valid but the domain refused it (e.g. deleting an account that still has transactions)
  - **Code:** `502 Bad Gateway` — `the assistant is unavailable right now`
  - **Code:** `500 Internal Server Error` — `Failed to execute suggestion`

---

## Monthly Usage

- **Method:** `GET`
- **Endpoint:** `/assistant/usage`
- **Description:** LLM spend for a calendar month, bucketed by local day in the configured timezone.
- **Query Parameters:**
  - `month` (optional): `YYYY-MM`. Defaults to the current month.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "month": "2026-07",
      "currency": "USD",
      "total_cost_usd": "0.014230",
      "total_input_tokens": 48210,
      "total_output_tokens": 1980,
      "interaction_count": 12,
      "by_day": [
        { "date": "2026-07-27", "cost_usd": "0.002180", "count": 3 }
      ]
    }
    ```
  - Costs are decimal strings — parse them as decimals, never as floats.
- **Error Responses:**
  - **Code:** `400 Bad Request` — `month must be YYYY-MM`
  - **Code:** `500 Internal Server Error` — `Failed to load usage`
