# gv-api

A comprehensive life orchestrator built in Go, designed to centralize data from multiple web services, platforms, and devices.

## Tech Stack

- **Go** — system core
- **`go-chi/chi/v5`** — lightweight, idiomatic HTTP router
- **`pgx/v5` & `sqlc`** — efficient PostgreSQL interaction with auto-generated type-safe queries
- **`testify` & `mockery`** — testing assertions and auto-generated interface mocks with type-safe expecters

## Setup

### Requirements
- Git, Docker & Docker Compose
- Go (v1.25.6+)
- sqlc
- mockery (`go install github.com/vektra/mockery/v2@latest`)

### Getting Started

1. **Clone and configure:**
   ```bash
   git clone https://github.com/OscarCarPu/gv-api.git
   cd gv-api
   make setup-project
   ```

2. **Edit `.env`** with your database credentials and secrets.

3. **Start the database and run:**
   ```bash
   docker compose up -d
   make run
   ```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | **Yes** | — | PostgreSQL connection string. |
| `PASSWORD` | **Yes** | — | Login password for full-access tokens. |
| `SEMIPRIVATE_PASSWORD` | **Yes** | — | Login password for read-only tokens. |
| `JWT_SECRET` | **Yes** | — | Secret used to sign JWTs. Generate with `openssl rand -hex 32`. |
| `TOTP_SECRET` | **Yes** | — | Base32 secret for TOTP 2FA. Generate with `openssl rand -base32 20`. |
| `ALLOWED_ORIGINS` | **Yes** | — | Comma-separated CORS origins. |
| `PORT` | No | `8080` | HTTP listen port. |
| `TIMEZONE` | No | `Europe/Madrid` | IANA timezone for date arithmetic. |
| `LIGHTS_DRIVER` | No | `mock` | `bluez` to drive real bulbs; anything else uses the in-memory mock. |
| `LIGHTS_*` | No | — | Adapter, timeouts, cache TTL and settle retries. See `.env.example`. |
| `PIPELINE_DATABASE_URL` | No | — | Connection string for **central-pipeline's** PostgreSQL, which owns the marts the Uptime domain reads. A second database, not gv's: read-only, never migrated from here. Unset means those endpoints answer 503. |
| `PIPELINE_STALE_AFTER_MS` | No | `7200000` | How old a mart may be before the API reports it as stale. Every mart carries the dbt run time, not `now()`. |
| `GOOGLE_CLIENT_ID` | No | — | OAuth client for the Calendar domain. Unset means no account can be connected; everything else still runs. |
| `GOOGLE_CLIENT_SECRET` | No | — | Its secret. |
| `GOOGLE_OAUTH_REDIRECT_URL` | No | — | Must match the client's redirect URI exactly, e.g. `https://gv-api.lab-ocp.com/calendar/google/callback`. |
| `GOOGLE_TOKEN_KEY` | If client set | — | 32 bytes of hex encrypting the stored refresh tokens. Generate with `openssl rand -hex 32`. The server refuses to start without it once a client id is set. |
| `CALENDAR_WEB_APP_URL` | No | first `ALLOWED_ORIGINS` | Where the OAuth callback sends the browser back to. |
| `CALENDAR_WEBHOOK_URL` | No | — | Public HTTPS address Google posts change notifications to. Unset means polling only. |
| `CALENDAR_*` | No | — | Webhook toggle, channel TTL and renewal window, poll interval, notification debounce. See `.env.example`. |

## API

### Domains

| Domain | Description | Docs |
|---|---|---|
| **Auth** | JWT login with optional TOTP 2FA. Two token tiers: full-access and semiprivate (read-only). | [auth](docs/api/auth.md) |
| **Habits** | Daily habit definitions with per-day logging and history. | [habits](docs/api/habits.md) |
| **Tasks** | Hierarchical project/task tree with todos, due dates, and Pomodoro time entries. | [tasks](docs/api/tasks/README.md) |
| **Plan** | Daily time-block planner that schedules tasks from the task tree. | [plan](docs/api/plan.md) |
| **Finance** | Accounts, categories, transactions, and spending stats (net worth, by-category, monthly, estimation). | [finance](docs/api/finance.md) |
| **Rutas** | Concello marks: which municipalities were visited and when. | [rutas](docs/api/rutas.md) |
| **Lights** | Bluetooth bulbs driven over BlueZ, with discovery and a registry. Semiprivate auth. | [lights](docs/api/lights.md) |
| **Calendar** | Google calendars mirrored locally and editable from here: OAuth per account, incremental sync, push notifications, recurring series expanded on read. | [calendar](docs/api/calendar.md) |
| **Uptime** | How much of the time the lab and its ESP32 watchdog have been reachable, read from central-pipeline's marts. Semiprivate auth. | [uptime](docs/api/uptime.md) |

### Infrastructure

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | None | Returns `200 OK`. Use for liveness probes. |
| `GET` | `/calendar/google/callback` | Signed state | Where Google's consent redirect lands. Cannot carry a bearer token: it is a browser redirect to the API host. |
| `POST` | `/calendar/google/webhook` | Channel token | Google's push notifications. Cannot carry a bearer token: Google sends no credentials. |

### Request / Response Headers

| Header | Direction | Description |
|---|---|---|
| `X-Request-ID` | Request & Response | Optional on request; auto-generated (random 8-byte hex) if absent. Echoed back in the response and propagated through logs for correlation. |
| `X-Device-ID` | Request | Optional stable per-browser UUID sent by gv-web. Nothing reads it yet, but it is CORS-allowlisted: an unlisted header makes the browser's preflight fail, which blocks the request outright. |

## Testing

| Command | Scope |
|---|---|
| `make lint` | gofmt + `go vet`. Runs in CI before the tests. |
| `make test-unit` | Handlers and services against mocks. No database. |
| `make test-integration` | Repositories against a real test database. |
| `make test-e2e` | HTTP requests against the running API. |
| `make test-bench` | Repository benchmarks (`BENCHTIME=10x` to override). |
| `make test` | All three test levels in order. |

The test database is created and dropped per run.

## Code Generation

### sqlc

Generates type-safe Go code from SQL queries:

```bash
make sqlc
```

### mockery

Generates mock implementations from Go interfaces for testing. Configured in `.mockery.yaml` with `with-expecter: true`, which provides type-safe `.EXPECT().MethodName()` helpers instead of raw string-based `.On("MethodName")` calls — giving compile-time safety if interface methods are renamed.

```bash
make generate-mocks
```

Mocks are generated into `internal/*/mocks/` directories and used by handler and service tests.

## Disclaimer

[Claude Code](https://claude.ai/code) was used as a code review tool during development of this project. No LLM was used to generate any of the code.

