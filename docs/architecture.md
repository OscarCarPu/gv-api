# Architecture

## Overview

**gv-api** is a single-user REST API in Go that centralises personal data:
habits, tasks (projects/tasks/todos/time entries), day planning, finance,
route marks and Bluetooth light bulbs. No framework beyond a router.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.25 |
| HTTP Router | [chi/v5](https://github.com/go-chi/chi) |
| Database | PostgreSQL 15 |
| DB Driver | [pgx/v5](https://github.com/jackc/pgx) (connection pool) |
| SQL Code Gen | [sqlc](https://sqlc.dev/) |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate), run at startup |
| Auth | JWT ([golang-jwt/v5](https://github.com/golang-jwt/jwt)) + TOTP 2FA ([pquerna/otp](https://github.com/pquerna/otp)) |
| Bluetooth | BlueZ over D-Bus ([godbus](https://github.com/godbus/dbus)) |
| Testing | stdlib `testing` + [testify](https://github.com/stretchr/testify) + [mockery](https://vektra.github.io/mockery/) |
| Containerization | Docker multi-stage build + Docker Compose |

## Project Structure

```
gv-api/
  cmd/api/main.go            # Wiring, router setup, server start
  internal/
    config/                  # Environment-based configuration
    database/
      db.go, migrate.go      # pgxpool factory, startup migrations
      pgconv/                # nullable pgx columns -> Go pointers
      gvdb/                  # sqlc-generated code, one file per query file
    response/, httputil/     # JSON responses, URL param parsing
    middleware/              # request id + slog correlation
    history/                 # shared history types and period maths
    testutil/                # test DB pool and truncation
    auth/                    # login, 2FA, bearer middleware
    <domain>/                # handler.go, service.go, repository.go, dto.go,
                             # errors.go, doc.go, mocks/
  db/
    migrations/              # schema, applied in order at startup
    queries/                 # SQL consumed by sqlc, one file per domain
  test/e2e/                  # End-to-end tests (full stack via HTTP)
  docs/                      # api/, business_logic/, data_models/
```

Domains: `habits`, `tasks`, `plan`, `finance`, `rutas`, `lights`.

## Architecture Pattern

### Handler -> Service -> Repository

Every domain follows the same three layers:

- **Handler**: HTTP only — decode, validate request shape, call service, encode.
  Declares the `ServiceInterface` it depends on.
- **Service**: business rules. Depends on the `Repository` interface.
- **Repository**: data access, mapping sqlc rows to domain DTOs. Takes the
  `*pgxpool.Pool` and builds its own sqlc `Queries`; the pool is also what lets
  the ones that need transactions open them.

Both interfaces are mocked by mockery (`.mockery.yaml`), so handler and service
are unit-testable without a database. Repositories are covered by integration
tests against a real one.

`lights` adds a fourth seam: a `Driver` interface over the bulbs, with a BlueZ
implementation and an in-memory mock, so the app runs without a radio.

### Database Access (sqlc)

Queries live in `db/queries/<domain>.sql`. `sqlc generate` produces a single
`gvdb` package from the whole schema (`db/migrations`), with one
`<domain>.sql.go` per query file plus shared `models.go` and `db.go`. Keeping
each repository to its own domain's queries is a convention, not a compiler
boundary.

### Authentication

1. `POST /login` — password check. The private password returns a 5-minute
   `tmp` token; the semiprivate one returns a 30-day `semi` token.
2. `POST /login/2fa` — tmp token + TOTP code returns a 30-day `full` token.
3. Routes are grouped by the kinds they accept: `lights` takes `semi` or
   `full`, everything else requires `full`.

Single-user system: no user table, passwords come from the environment.

## Testing Strategy

| Level | Command | Scope |
|---|---|---|
| Unit | `make test-unit` | Handler/service with mocks, no DB |
| Integration | `make test-integration` | Repositories against a real test DB |
| E2E | `make test-e2e` | HTTP against the running API + DB |
| Bench | `make test-bench` | Repository benchmarks |

`make lint` (gofmt + `go vet`) runs in CI before the tests. The test DB is
created and dropped per run.

## Deployment

- Multi-stage Docker build (golang:alpine builder -> alpine runtime), non-root.
- Docker Compose with `db` (postgres:15-alpine) + `gv-api` on the external `gv`
  network.
- Migrations run from the API at startup, not from the database image.
- Gitea Actions deploys on push to `main`: lint, unit tests, then
  `docker compose up --build --wait`.
