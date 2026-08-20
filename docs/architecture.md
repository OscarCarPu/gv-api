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
    response/, httputil/     # JSON responses, URL param parsing
    middleware/              # request id + slog correlation
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
`uptime`.

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

`calendar` adds the same kind of seam for a remote service: a `google.Client`
interface with an HTTP implementation and an in-memory `Fake` that reproduces
what actually matters there — sync tokens going stale (410), etags failing
(412), grants dying (`invalid_grant`), occurrences materialising on first
write. The sync rules are testable without a network because of it. It is also
the only domain with a **background worker**: one goroutine, started in
`main.go` and cancelled on shutdown, that drains push notifications, polls as a
safety net and replaces push channels before they expire.

### Database Access (sqlc)

Queries live in `db/queries/<domain>.sql`. `sqlc generate` produces a single
`gvdb` package from the whole schema (`db/migrations`), with one
`<domain>.sql.go` per query file plus shared `models.go` and `db.go`. Keeping
each repository to its own domain's queries is a convention, not a compiler
boundary.

### The Second Database (central-pipeline)

Every domain but one reads gv's own PostgreSQL through sqlc. `uptime` reads
someone else's: [central-pipeline][cp] collects what the devices around the
house publish over MQTT and models it with dbt into marts in its own instance,
on its own port. `internal/pipeline` holds that connection, and any later
domain backed by the same pipeline shares it rather than opening a second one.

What that boundary implies, and why the code looks different there:

- **Separate DSN, separate pool** (`PIPELINE_DATABASE_URL`). Two servers, not
  two schemas. The pipeline stack is allowed to be down while gv-api runs, so
  the pool is opened without a ping and with no warm connections, and an unset
  DSN is a normal state: reads report `pipeline.ErrNotConfigured` and the
  handler answers 503.
- **Read-only on the connection** (`default_transaction_read_only`), not only by
  grant. dbt owns those relations.
- **No migrations, no sqlc.** sqlc generates from `db/migrations`, which does not
  describe this schema; the queries are hand-written against the column contract
  in central-pipeline's `docs/sources/watchdog.md`. Repository integration tests
  build the marts as fixtures, so they run without the other project's stack.
- **Reads retry.** Every dbt run drops and recreates the marts, so a read can
  land in the gap where a relation does not exist. `pipeline.Collect` retries
  the codes that look like that rebuild before failing.
- **Nothing is live.** The marts carry the dbt run time rather than `now()`, so
  freshness is part of the response (`computed_at`, `stale`) rather than
  assumed.

[cp]: https://github.com/OscarCarPu/central-pipeline

### Authentication

1. `POST /login` — password check. The private password returns a 5-minute
   `tmp` token; the semiprivate one returns a 30-day `semi` token.
2. `POST /login/2fa` — tmp token + TOTP code returns a 30-day `full` token.
3. Routes are grouped by the kinds they accept: `lights` and `uptime` take
   `semi` or `full`, everything else requires `full`.

Two endpoints are public because they cannot be otherwise, each with its own
guard rather than an exemption: `GET /calendar/google/callback` (Google's
consent redirect lands on the API host, where the web app's session cookie does
not exist — guarded by an HMAC-signed `state`) and `POST
/calendar/google/webhook` (Google's notification carries no credentials —
guarded by the per-channel token it echoes back).

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

### Where it runs

The homelab host, reached as `ssh.lab-ocp.com` (ssh over `cloudflared access
ssh`). The stacks live in `/home/ocp/docker/gv/{gv-api,gv-web}`.

Both services are published through a Cloudflare tunnel, with valid
certificates and no Cloudflare Access in front:

| Hostname | Service |
|---|---|
| `https://gv.lab-ocp.com` | gv-web (`ORIGIN`) |
| `https://gv-api.lab-ocp.com` | gv-api (`VITE_API_URL`; `ALLOWED_ORIGINS` is just the web host) |

The ingress rules for those two hostnames are managed in the Cloudflare
dashboard, not in `/etc/cloudflared/config.yml` on the host — that file only
carries the ssh and minecraft routes.

That the API is publicly reachable over HTTPS is what makes third-party OAuth
redirects and inbound webhooks possible at all; the `calendar` domain depends on
both.
