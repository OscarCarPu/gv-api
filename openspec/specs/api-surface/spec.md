# api-surface Specification

## Purpose

Lists which routes gv-api serves, under which access tier, and holds each one to the behavior documented in `docs/api/` and `docs/business_logic/`. This lets clients rely on a fixed surface across implementations.

## Requirements

### Requirement: Documented endpoint behavior
Each route SHALL behave as `docs/api/` and `docs/business_logic/` describe, with the same query parameters, request bodies, status codes and response bodies. Where the docs are silent, the behavior of the last Go release (`d32ab42`) SHALL be the reference.

#### Scenario: Recorded request replayed
- **WHEN** a request from the parity corpus is sent at the same moment to the new service and to the Go release, each running against its own copy of the same database snapshot
- **THEN** both return the same status and equal decoded bodies, ignoring fields marked as volatile in the corpus (signatures, `computed_at`, live printer telemetry) and the differences listed in `tests/parity/ACCEPTED.md`

#### Scenario: Database state after replay
- **WHEN** the whole corpus, writes included, has been replayed against both services
- **THEN** both databases hold the same rows in every table, ignoring columns the corpus marks as volatile (such as `created_at`)

### Requirement: Public routes
The service SHALL serve without a bearer token: `GET /health`, `POST /login`, `POST /login/2fa`, and the signed printer routes `GET /domotics/printers/{id}/camera/stream`, `GET /domotics/printers/{id}/recordings/{name}` and `GET /domotics/printers/{id}/files/download`.

#### Scenario: Valid signature
- **WHEN** a client follows a recording URL returned by `GET …/recordings` before its `exp`
- **THEN** the file is served without an `Authorization` header

#### Scenario: Tampered or expired signature
- **WHEN** a signed printer URL is used after `exp`, or with a modified `sig`, `name` or `exp`
- **THEN** the request is refused, with the same status the Go service returned

### Requirement: Signed URL compatibility
Signed printer URLs SHALL use the HMAC-SHA256 scheme of the Go service, keyed with `JWT_SECRET` over `printer-media:<printer>/<name>/<exp>`, and the same expiry rounding. URLs issued before cutover SHALL stay valid until their `exp`.

#### Scenario: URL issued by the Go service
- **WHEN** a camera stream URL issued by the Go service is opened on the new service before its `exp`
- **THEN** the stream is served

### Requirement: Semiprivate routes
The service SHALL serve these routes to `semi` or `full` tokens: `/rutas/marks` (GET, POST), `/rutas/marks/{id}` (GET, PUT, DELETE), `/domotics/uptime` and `/domotics/uptime/windows` (GET), and every non-public route under `/domotics/printers`.

#### Scenario: Printer list
- **WHEN** a client calls `GET /domotics/printers` with a `semi` token
- **THEN** the response is 200 with `[{id, name, model}]`

### Requirement: Full-private routes
The service SHALL serve these route families to `full` tokens only: `/habits`, `/tasks/projects`, `/tasks/tasks`, `/tasks/todos`, `/tasks/time-entries`, `/tasks/tree`, `/plan`, `/capacity/free-busy`, `/finance`, `/calendar/events`, `/backups` and, when the failover is enabled, `/report`. Within each family the methods and subpaths SHALL be exactly those the Go release served.

#### Scenario: Static subpath beside an id
- **WHEN** a client calls `GET /tasks/projects/list-fast`
- **THEN** the list-fast handler answers, not the get-by-id handler with an invalid id

#### Scenario: Route inventory
- **WHEN** the route table of the new service is listed, excluding lights
- **THEN** it equals the Go release's route table: every (method, path) pair is present in both, with the same tier

### Requirement: Failover control routes
When `FAILOVER_SIDE=home` and `FAILOVER_TOKEN` is set, the service SHALL serve `POST /failover`, `GET /failover/status` and `POST /failover/outage`, authenticated by the `X-Failover-Token` header compared in constant time, with 401 `{"error": "invalid failover token"}` on mismatch. Otherwise these routes SHALL not exist.

#### Scenario: Failover disabled
- **WHEN** `FAILOVER_TOKEN` is empty and a client calls `GET /failover/status`
- **THEN** the response is 404

### Requirement: Hold lock
While the failover holds the app or a merge runs, every request except those under `/health`, `/login`, `/failover` and `/report` SHALL return 503 `{"error": "Restoring data after the failover. Back in a moment.", "code": "merge_pending"}`, before authentication is checked.

#### Scenario: Held app
- **WHEN** the app is held and a client calls `GET /habits` with a valid `full` token
- **THEN** the response is 503 with code `merge_pending`

#### Scenario: Exempt route while held
- **WHEN** the app is held and a client calls `GET /report` with a `full` token
- **THEN** the report is served

### Requirement: Lights routes retired
The service SHALL NOT serve any route under `/domotics/lights`, and SHALL NOT read or write the `lights` table. The table and its rows SHALL be left intact, so the Go release can be redeployed without data loss.

#### Scenario: Former lights endpoint
- **WHEN** a client calls `GET /domotics/lights/state` with a `full` token
- **THEN** the response is 404 with body `{"detail": "Not Found"}`

#### Scenario: Data kept for rollback
- **WHEN** the new service has run its startup migrations on a database holding lights rows
- **THEN** the `lights` table still exists with the same rows
