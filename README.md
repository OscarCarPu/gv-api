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

| Backup variable | Default | Meaning |
|---|---|---|
| `BACKUP_DIR` | `/backups` | Where `hourly/` and `daily/` live inside the container. |
| `BACKUP_INTERVAL_MS` | `3600000` | Milliseconds between scheduled backups, aligned to the clock. `0` turns the schedule off. |
| `BACKUP_KEEP_HOURLY_DAYS` | `2` | Days a backup stays in `hourly/`. Must be at least 1. |
| `BACKUP_KEEP_DAILY_DAYS` | `30` | Days a backup stays in `daily/`. Must be at least `BACKUP_KEEP_HOURLY_DAYS`. |

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
| **Backups** | List, take and download the API's own database dumps. | [backups](docs/api/backups.md) |

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

## Backups

gv-api dumps its own database with `pg_dump` every `BACKUP_INTERVAL_MS` (default 1 h, at the top of the hour; `0` turns the schedule off). The dumps are gzipped plain SQL.

**On demand:** run the same backup outside the schedule from the running container. Only one backup runs at a time, so this is safe while the schedule or another run is active:

```bash
docker compose exec -T gv-api ./main backup
```

The deploy workflow runs this before every deploy. The HTTP endpoints, response codes and restore steps are in [docs/api/backups.md](docs/api/backups.md).

**Retention:** every backup goes to `hourly/`, and the first one of each UTC day is also hard-linked into `daily/`. After each successful run, backups older than `BACKUP_KEEP_HOURLY_DAYS` (default 2) are deleted from `hourly/`, and older than `BACKUP_KEEP_DAILY_DAYS` (default 30) from `daily/`. The newest file in each folder is always kept, and files that don't match `gv-db-<YYYYMMDD>T<HHMMSS>Z.sql.gz` are never touched.

**Storage:** compose mounts `./backups` at `/backups` (`BACKUP_DIR`). gv-api runs as uid 1000, so `backups/`, `backups/hourly/` and `backups/daily/` must be owned by uid 1000, or every backup fails with `permission denied`. `make up`, `make reset` and the deploy workflow create the folders and fix their owner with a throwaway container, so no `sudo` is needed.

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

