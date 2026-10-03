# gv-api

Single-user Go API that centralises personal data from services and devices.

## Tech Stack

Go, chi, PostgreSQL via pgx + sqlc, testify + mockery. See [architecture](docs/architecture.md).

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

2. **Edit `.env`** (every variable is listed in `.env.example`).

3. **Start the database and run:**
   ```bash
   docker compose up -d
   make run
   ```

## Environment Variables

All variables are in `.env.example` with their defaults. Required: `DATABASE_URL`, `PASSWORD`,
`SEMIPRIVATE_PASSWORD`, `JWT_SECRET`, `TOTP_SECRET`, `ALLOWED_ORIGINS`, plus `GOOGLE_TOKEN_KEY`
once `GOOGLE_CLIENT_ID` is set. Optional integrations (lights, central-pipeline, Google Calendar)
degrade gracefully when unset.

## API

### Domains

| Domain | Description | Docs |
|---|---|---|
| **Auth** | JWT login with TOTP 2FA. Two token tiers: full and semiprivate. | [auth](docs/api/auth.md) |
| **Habits** | Habits with daily/weekly/monthly targets, logs, streaks and history. | [habits](docs/api/habits.md) |
| **Tasks** | Hierarchical project/task tree with todos, due dates, and Pomodoro time entries. | [tasks](docs/api/tasks/README.md) |
| **Plan** | Daily time blocks, linked to tasks, events or recurring commitments. | [plan](docs/api/plan.md) |
| **Capacity** | Free and busy hours per day. | [capacity](docs/api/capacity.md) |
| **Finance** | Accounts, categories, transactions, monthly/yearly budgets and stats. | [finance](docs/api/finance.md) |
| **Rutas** | Concello marks: which municipalities were visited and when. Semiprivate auth. | [rutas](docs/api/rutas.md) |
| **Lights** | Bluetooth bulbs driven over BlueZ, with discovery and a registry. Semiprivate auth. | [lights](docs/api/lights.md) |
| **Calendar** | Google calendars mirrored locally and editable: OAuth, incremental sync, push notifications. | [calendar](docs/api/calendar.md) |
| **Uptime** | Lab and ESP32 watchdog uptime from central-pipeline's marts. Semiprivate auth. | [uptime](docs/api/uptime.md) |

### Infrastructure

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | None | 200 when the database answers, 503 otherwise. |
| `GET` | `/calendar/google/callback` | Signed state | Google's OAuth redirect. |
| `POST` | `/calendar/google/webhook` | Channel token | Google's push notifications. |

### Request / Response Headers

| Header | Direction | Description |
|---|---|---|
| `X-Request-ID` | Request & Response | Optional; generated if absent, echoed back and logged. |
| `X-Device-ID` | Request | Per-browser UUID from gv-web. Unused, but must stay CORS-allowlisted or preflights fail. |

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

```bash
make sqlc            # db/queries -> internal/database/gvdb
make generate-mocks  # interfaces -> internal/*/mocks (typed .EXPECT() helpers)
```

