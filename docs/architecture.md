# Architecture

## Overview

**gv-api** is a single-user REST API in Go that centralises personal data:
habits, tasks (projects/tasks/todos/time entries), day planning, finance,
route marks, Bluetooth light bulbs, a mirror of the user's Google calendars
and the uptime of the home lab. No framework beyond a router.

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
| Google OAuth | [x/oauth2](https://pkg.go.dev/golang.org/x/oauth2) for tokens; the Calendar REST calls are hand-rolled |
| Recurrence | [rrule-go](https://github.com/teambition/rrule-go) (RFC 5545 expansion) |
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
    core/                    # shared HTTP plumbing: request id + slog correlation,
                             # CORS, JSON responses, URL params
    history/                 # shared history types and period maths
    testutil/                # test DB pool and truncation
    pipeline/                # read-only connection to central-pipeline's database
    auth/                    # login, 2FA, bearer middleware
    <domain>/                # handler.go, service.go, repository.go, dto.go,
                             # errors.go, doc.go, mocks/
  db/
    migrations/              # schema, applied in order at startup
    queries/                 # SQL consumed by sqlc, one file per domain
  test/e2e/                  # End-to-end tests (full stack via HTTP)
  docs/                      # api/, business_logic/, data_models/
```

Domains: `habits`, `tasks`, `plan`, `finance`, `rutas`, `lights`, `calendar`,
`uptime`, `capacity`.

`capacity` has no table: daily free hours come from `DAILY_CAPACITY_HOURS`, and busy hours
from `plan.Service` (see [plan](business_logic/plan.md)).

## Architecture Pattern

### Handler -> Service -> Repository

Every domain follows the same three layers:

- **Handler**: HTTP only — decode, validate request shape, call service, encode.
  Declares the `ServiceInterface` it depends on.
- **Service**: business rules. Depends on the `Repository` interface.
- **Repository**: data access, mapping sqlc rows to DTOs. Holds the pool so it can
  open transactions.

Handler and service interfaces are mocked with mockery; repositories have
integration tests against a real database.

Two domains add a seam for external systems: `lights` has a `Driver` (BlueZ or
in-memory mock) and `calendar` a `google.Client` (HTTP or an in-memory `Fake`
that reproduces 410s, 412s and revoked grants). `calendar` also runs the only
background worker: it drains push notifications, polls and renews channels.

### Database Access (sqlc)

Queries live in `db/queries/<domain>.sql`; `sqlc generate` builds one `gvdb`
package with a `<domain>.sql.go` per query file. Repositories use only their own
domain's queries by convention.

### The Second Database (central-pipeline)

`uptime` reads [central-pipeline][cp]'s dbt marts through `internal/pipeline`,
shared by any future domain on that pipeline:

- **Separate DSN and pool** (`PIPELINE_DATABASE_URL`), opened without a ping. Unset
  means `pipeline.ErrNotConfigured` and a 503.
- **Read-only on the connection** (`default_transaction_read_only`).
- **No migrations, no sqlc.** Queries are hand-written against central-pipeline's
  `docs/sources/watchdog.md`; integration tests build the marts as fixtures.
- **Reads retry** while dbt drops and recreates a mart.
- **Nothing is live.** Responses carry `computed_at` and `stale`.

[cp]: https://github.com/OscarCarPu/central-pipeline

### Authentication

1. `POST /login` — password check. The private password returns a 5-minute
   `tmp` token; the semiprivate one returns a 30-day `semi` token.
2. `POST /login/2fa` — tmp token + TOTP code returns a 30-day `full` token.
3. Routes are grouped by the kinds they accept: `lights`, `uptime` and `rutas`
   take `semi` or `full`, everything else requires `full`.

Two public endpoints have their own guards: `GET /calendar/google/callback`
(HMAC-signed `state`) and `POST /calendar/google/webhook` (per-channel token).

Single-user system: no user table, passwords come from the environment.

## Testing

See the [README](../README.md#testing).

## Deployment

- Multi-stage Docker build (golang:alpine builder -> alpine runtime), non-root.
- Docker Compose with `db` (postgres:15-alpine) + `gv-api` on the external `gv`
  network.
- Migrations run from the API at startup, not from the database image.
- Gitea Actions deploys on push to `main`: lint, unit tests, then
  `docker compose up --build --wait`.

### Where it runs

The homelab host, reached as `ssh.lab-ocp.com` (ssh over `cloudflared access
ssh`). The stacks live in `/home/ocp/docker/gv/{gv-api,gv-web}`.

Both services are published through a Cloudflare tunnel, with valid
certificates and no Cloudflare Access in front:

| Hostname | Service |
|---|---|
| `https://gv.lab-ocp.com` | gv-web (`ORIGIN`) |
| `https://gv-api.lab-ocp.com` | gv-api (`VITE_API_URL`; `ALLOWED_ORIGINS` is just the web host) |

Their ingress rules live in the Cloudflare dashboard, not in
`/etc/cloudflared/config.yml` on the host. Public HTTPS is what makes the
calendar's OAuth redirect and webhooks possible.
