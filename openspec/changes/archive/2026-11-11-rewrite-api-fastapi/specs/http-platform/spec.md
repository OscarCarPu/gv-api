# Spec Delta

## Purpose

Defines the wire conventions every gv-api endpoint shares, so that clients (gv-web, Android, the failover Lambda) keep working whatever implementation serves them. Exact bytes may differ from the Go service where the simpler implementation was chosen; each such difference is listed in `tests/parity/ACCEPTED.md`.

## ADDED Requirements

### Requirement: JSON responses
Every JSON response SHALL carry `Content-Type: application/json` and a body that decodes to the same value the Go service returned for the same request and data, apart from the differences listed in `tests/parity/ACCEPTED.md`. Field names, value types, and the choice between `null` and an empty array for each field SHALL match. Every field of a response shape SHALL always be present, as `null` or `[]` when it has no value.

#### Scenario: Empty collection keeps its encoding
- **WHEN** a client requests a resource whose list field is empty, and the Go service encoded that field as `null` (or as `[]`)
- **THEN** the response encodes that field as `null` (or as `[]`) as well

#### Scenario: Optional field absent from data
- **WHEN** a nullable column is NULL for the requested row
- **THEN** the corresponding key is present with value `null`

#### Scenario: Field the Go service omitted when empty
- **WHEN** a task without recurrence is returned, a field the Go service left out when empty
- **THEN** the key is present with value `null`

### Requirement: Decimal encoding
Monetary and hour amounts held as SQL `numeric` SHALL encode as JSON strings holding the decimal value with its scale, never as JSON numbers.

#### Scenario: Amount with trailing zero
- **WHEN** an account total stored as `12.50` is returned
- **THEN** the JSON value is the string `"12.50"`

#### Scenario: Whole amount
- **WHEN** a transaction amount stored as `100.00` is returned
- **THEN** the JSON value is the string `"100.00"`, not a number

### Requirement: Timestamp and date encoding
Timestamps SHALL encode as RFC 3339 strings, with `Z` for UTC and six fractional digits when the second has a fraction. Calendar dates SHALL encode as `YYYY-MM-DD`.

#### Scenario: Whole-second timestamp
- **WHEN** a time entry started at exactly 09:00:00 UTC is returned in a field the Go service rendered in UTC
- **THEN** the value is `"…T09:00:00Z"` with no fractional part

#### Scenario: Sub-second timestamp
- **WHEN** a timestamp has 120 ms past the second
- **THEN** the value ends in `.120000Z`

### Requirement: Error envelope
Error responses SHALL be JSON objects `{"error": "<message>"}`, with an added `"code"` key where the Go service sent one. Status codes and message texts SHALL match the Go service for the same failure.

#### Scenario: Validation failure
- **WHEN** a client creates a habit with an empty name
- **THEN** the response is 400 with body `{"error": "name is required"}`

#### Scenario: Unexpected server failure
- **WHEN** a repository call fails while listing daily habits
- **THEN** the response is 500 with body `{"error": "Failed to get daily habits"}` and the cause is logged, not returned

### Requirement: Lenient request body decoding
Unknown body fields SHALL be ignored, and missing fields SHALL take their zero value (`""`, `0`, `false`, or null for optional fields). A malformed body (including content after the JSON value), or a value that cannot be coerced to the field's type, SHALL return the handler's own 400 message, never a 422 or a framework error shape. Coercible values such as `"5"` for an integer MAY be accepted.

#### Scenario: Unknown field
- **WHEN** a client posts a habit log with an extra key `"foo": 1`
- **THEN** the request succeeds as if the key were absent

#### Scenario: Wrong type
- **WHEN** a client posts `{"habit_id": "abc"}` to `/habits/log`
- **THEN** the response is 400 with body `{"error": "Invalid Body"}`

### Requirement: Path id parsing
Numeric path ids SHALL be accepted only as positive integers that fit in 32 bits. Any other value SHALL return 400 with `{"error": "invalid <entity> id"}`.

#### Scenario: Non-numeric id
- **WHEN** a client calls `DELETE /habits/abc`
- **THEN** the response is 400 with body `{"error": "invalid habit id"}`

#### Scenario: Zero or overflow
- **WHEN** a client calls `DELETE /habits/0` or `DELETE /habits/4294967296`
- **THEN** the response is 400 with body `{"error": "invalid habit id"}`

### Requirement: Unmatched routes and methods
A path no route matches SHALL return 404 with body `{"detail": "Not Found"}`. A known path called with an unsupported method SHALL return 405 with an `Allow` header listing the supported methods. Neither SHALL require authentication.

#### Scenario: Unknown path
- **WHEN** a client calls `GET /does-not-exist` without a token
- **THEN** the response is 404 with body `{"detail": "Not Found"}`

#### Scenario: Wrong method
- **WHEN** a client calls `PATCH /habits`
- **THEN** the response is 405 and its `Allow` header includes `GET` and `POST`

### Requirement: CORS policy
The API SHALL answer CORS for the origins in `ALLOWED_ORIGINS` only, allow methods GET, POST, PUT, PATCH, DELETE, OPTIONS and HEAD, allow request headers `Content-Type`, `Authorization`, `X-Request-ID`, `X-Device-ID`, `X-File-Name`, `X-Upload-Id`, `X-Upload-Offset` and `X-Upload-Total`, expose `Content-Length` and `X-Request-ID`, send no credentials flag, and cache preflights for 300 seconds. Preflights SHALL succeed without authentication.

#### Scenario: Preflight from gv-web
- **WHEN** the allowed origin sends a preflight asking for `content-type,authorization,x-device-id,x-request-id`
- **THEN** the response carries `Access-Control-Allow-Origin` equal to that origin

#### Scenario: Foreign origin
- **WHEN** an origin not in `ALLOWED_ORIGINS` sends a preflight
- **THEN** the response carries no `Access-Control-Allow-Origin` header

### Requirement: Request correlation
Every response SHALL carry `X-Request-ID`. If the request sent one, it SHALL be echoed. Otherwise a new 16-hex-character id SHALL be generated. Every log line written while handling the request SHALL include it as `request_id`.

#### Scenario: Client-supplied id
- **WHEN** a request carries `X-Request-ID: abc123`
- **THEN** the response carries `X-Request-ID: abc123`

#### Scenario: Generated id
- **WHEN** a request carries no `X-Request-ID`
- **THEN** the response carries an `X-Request-ID` of 16 lowercase hex characters

### Requirement: Structured logs
The process SHALL write logs to stdout as one JSON object per line, at info level and above, with keys for level, message and time plus the event's attributes.

#### Scenario: Log line shape
- **WHEN** the server starts
- **THEN** stdout receives a JSON line whose message reports the server starting and the port

### Requirement: Health check
`GET /health` SHALL need no authentication. It SHALL return 200 with an empty body when the main database answers a ping within `HEALTH_TIMEOUT_MS`, and 503 with an empty body otherwise.

#### Scenario: Database down
- **WHEN** the database is unreachable
- **THEN** `GET /health` returns 503

### Requirement: Failover-side guard
When `FAILOVER_SIDE=aws`, the printers and uptime routes, including the public signed printer routes, SHALL return 503 with `{"error": "Not available while running on the backup server. Back when home is restored.", "code": "unavailable_on_failover"}` for every method.

#### Scenario: Printer status on the backup server
- **WHEN** the service runs with `FAILOVER_SIDE=aws` and a client calls `GET /domotics/printers/core-one/status` with a valid token
- **THEN** the response is 503 with code `unavailable_on_failover`
