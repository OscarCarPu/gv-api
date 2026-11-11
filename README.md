# gv-api

Single-user API that centralises personal data from services and devices.

## Tech Stack

Python 3.14, FastAPI on uvicorn, PostgreSQL via SQLAlchemy Core (asyncpg), Alembic, pytest. See [architecture](docs/architecture.md).

## Setup

### Requirements
- Git, Docker & Docker Compose
- [uv](https://docs.astral.sh/uv/) (installs Python 3.14 and the dependencies)

### Getting Started

1. **Clone and configure:**
   ```bash
   git clone https://github.com/OscarCarPu/gv-api.git
   cd gv-api
   make setup-project
   ```

2. **Edit `.env`** (every variable is listed in `.env.example`).

3. **Run the stack:**
   ```bash
   make up
   ```

   `make up` runs the whole stack in Docker and adds a fake AWS (SSM and S3) so the failover restore can be rehearsed locally, see [docs/dev-failover.md](docs/dev-failover.md).

## Environment Variables

All variables are in `.env.example` with their defaults. Required: `DATABASE_URL`, `PASSWORD`,
`SEMIPRIVATE_PASSWORD`, `JWT_SECRET`, `TOTP_SECRET`, `ALLOWED_ORIGINS`.
Optional integrations (printers, Prusa Connect, S3)
degrade gracefully when unset.

`TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` (optional) send every ERROR log
to a Telegram chat: batched every 10 s, repeated messages collapsed with a count, and the environment
(`ENVIRONMENT`, default `dev`), the side (`home` or `aws`) and `request_id` in each alert. With either one unset, alerts are off and the
service logs that once at startup. A failed send is dropped with a WARNING and never affects
requests or jobs.

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
| **Printers** | PrusaLink telemetry, job control, print files, camera and recordings. Semiprivate auth. | [printers](docs/api/printers.md) |
| **Calendar** | Events: a name and a time range, linkable from plan blocks. | [calendar](docs/api/calendar.md) |
| **Uptime** | Uptime, downtime and failover time, from a heartbeat and the failover record. Semiprivate auth. | [uptime](docs/api/uptime.md) |
| **Backups** | List, take and download the API's own database dumps. | [backups](docs/api/backups.md) |

### Infrastructure

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | None | 200 when the database answers, 503 otherwise. |

### Request / Response Headers

| Header | Direction | Description |
|---|---|---|
| `X-Request-ID` | Request & Response | Optional; generated if absent, echoed back and logged. |
| `X-Device-ID` | Request | Per-browser UUID from gv-web. Unused, but must stay CORS-allowlisted or preflights fail. |

## Backups

gv-api dumps its own database with `pg_dump` every `BACKUP_INTERVAL_MS` (default 5 min, on the clock; `0` turns the schedule off). The dumps are gzipped plain SQL.

**On demand:** run the same backup outside the schedule from the running container. Only one backup runs at a time, so this is safe while the schedule or another run is active:

```bash
docker compose exec -T gv-api gv-api backup
```

The deploy workflow runs this before every deploy. The HTTP endpoints, response codes and restore steps are in [docs/api/backups.md](docs/api/backups.md).

**Retention:** every backup goes to `hourly/`, and the first one of each UTC day is also hard-linked into `daily/`. After each successful run, backups older than `BACKUP_KEEP_HOURLY_DAYS` (default 2) are deleted from `hourly/`, and older than `BACKUP_KEEP_DAILY_DAYS` (default 30) from `daily/`. The newest file in each folder is always kept, and files that don't match `gv-db-<YYYYMMDD>T<HHMMSS>Z.sql.gz` are never touched.

**Storage:** compose mounts `./backups` at `/backups` (`BACKUP_DIR`). gv-api runs as uid 1000, so `backups/`, `backups/hourly/` and `backups/daily/` must be owned by uid 1000, or every backup fails with `permission denied`. `make up`, `make reset` and the deploy workflow create the folders and fix their owner with a throwaway container, so no `sudo` is needed.

## Testing

| Command | Scope |
|---|---|
| `make lint` | ruff (lint and format) and mypy `--strict`. Runs in CI before the tests. |
| `make test-unit` | Routers and services against in-memory fakes. No database. |
| `make test-integration` | Repositories and migrations against a real test database. |
| `make test-e2e` | HTTP requests against the compose API, rebuilt on a fresh test database. |
| `make test` | Unit and integration with the coverage gate (85% overall, 90% per `service.py`), then e2e. |

The test databases are created per run. Load benchmarks live in [bench/](bench/README.md).

`uv run pytest --testmon tests/unit` runs only the tests affected by the code changed since the
last run ([pytest-testmon](https://testmon.org), data in the git-ignored `.testmondata`); the
pre-commit hook uses it. CI and `make test` always run everything with the coverage gate.

## Migrations

Alembic, applied at startup in one transaction (`migrations/versions/`). The first revision,
`0001_baseline`, creates the schema from `migrations/baseline.sql`. New migrations are hand-written revisions; autogenerate misses views, functions and triggers.


## Disclaimer

This project was developed partially with the help of [Claude Code](https://claude.com/claude-code). The code is being reviewed and modified to clean up the architecture and take full ownership of the project.
