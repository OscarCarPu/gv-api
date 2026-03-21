# Architecture

## Overview

**gv-api** is a personal productivity REST API built in Go, managing **habits** (daily tracking) and **tasks** (projects, tasks, todos, time entries). It follows a clean layered architecture with no external framework beyond a lightweight router.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.25 |
| HTTP Router | [chi/v5](https://github.com/go-chi/chi) |
| Database | PostgreSQL 15 (via Docker) |
| DB Driver | [pgx/v5](https://github.com/jackc/pgx) (connection pool) |
| SQL Code Gen | [sqlc](https://sqlc.dev/) |
| Auth | JWT ([golang-jwt/v5](https://github.com/golang-jwt/jwt)) + TOTP 2FA ([pquerna/otp](https://github.com/pquerna/otp)) |
| CORS | [go-chi/cors](https://github.com/go-chi/cors) |
| Testing | stdlib `testing` + [testify](https://github.com/stretchr/testify) |
| Containerization | Docker multi-stage build + Docker Compose |

## Project Structure

```
gv-api/
  cmd/api/main.go           # Entry point - wiring, router setup, server start
  internal/
    config/config.go         # Environment-based configuration
    database/
      db.go                  # pgxpool connection factory
      habitsdb/              # sqlc-generated code for habits queries
      tasksdb/               # sqlc-generated code for tasks queries
    response/response.go     # JSON/Error response helpers
    history/
      history.go             # Shared history types and period utilities
    auth/
      handler.go             # Login + 2FA HTTP handlers
      service.go             # JWT generation/validation, password check, TOTP
      middleware.go           # Bearer token auth middleware
    habits/
      handler.go             # HTTP handlers (GetDaily, UpsertLog, CreateHabit)
      service.go             # Business logic (date parsing, delegation)
      repository.go          # Data access (maps sqlc types to domain DTOs)
      dto.go                 # Request/response types
    tasks/
      handler.go             # HTTP handlers (CRUD projects/tasks/todos/time-entries)
      service.go             # Business logic (finish timestamps, delegation)
      repository.go          # Data access + tree building logic
      dto.go                 # Request/response types
  db/
    migrations/              # SQL schema files (used by Docker init + sqlc)
    queries/                 # SQL queries consumed by sqlc
  test/e2e/                  # End-to-end tests (full stack via HTTP)
  docs/api/                  # API endpoint documentation
```

## Architecture Pattern

### Handler -> Service -> Repository

Each domain (habits, tasks) follows a strict 3-layer pattern:

- **Handler**: HTTP concerns only - decode request, call service, encode response. Defines a `ServiceInterface` for testability.
- **Service**: Business logic - date handling, default timestamps. Depends on a `Repository` interface.
- **Repository**: Data access - maps between sqlc-generated types and domain DTOs. Depends on sqlc's `Querier` interface.

All dependencies flow inward via interfaces, enabling unit testing with mocks at every layer.

### Database Access (sqlc)

SQL queries live in `db/queries/*.sql` with `sqlc` annotations. Running `sqlc generate` produces type-safe Go code in `internal/database/{habitsdb,tasksdb}/`. The generated `Querier` interface is injected into repositories.

Configuration in `sqlc.yaml` splits queries into two packages (habitsdb, tasksdb) sharing the same schema migrations.

### Authentication Flow

1. `POST /login` - password check -> returns temporary JWT (5 min, kind="tmp")
2. `POST /login/2fa` - validates tmp token + TOTP code -> returns full JWT (30 days, kind="full")
3. Protected routes use `auth.Middleware` which validates "full" tokens via `Bearer` header

Single-user system - no user table, password stored in config.

### Configuration

Environment variables loaded via `os.Getenv` with sensible defaults. No `.env` file loader - relies on Docker Compose `env_file` or shell environment.

## API Endpoints

### Public
| Method | Path | Description |
|---|---|---|
| POST | `/login` | Password authentication |
| POST | `/login/2fa` | TOTP second factor |

### Protected (require Bearer token)
| Method | Path | Description |
|---|---|---|
| GET | `/habits?date=YYYY-MM-DD` | Get habits with logs for a date |
| POST | `/habits` | Create a habit |
| POST | `/habits/log` | Upsert a habit log entry |
| GET | `/habits/{id}/history` | Aggregated habit history |
| DELETE | `/habits/{id}` | Delete a habit |
| GET | `/tasks/tree` | Active project/task tree |
| GET | `/tasks/projects` | Root (unfinished, parentless) projects |
| GET | `/tasks/projects/{id}/children` | Project with descendants, tasks, todos, time stats |
| POST | `/tasks/projects` | Create project |
| PATCH | `/tasks/projects/{id}` | Update a project |
| POST | `/tasks/tasks` | Create task |
| PATCH | `/tasks/tasks/{id}` | Update a task |
| GET | `/tasks/tasks/{id}/time-entries` | Task detail with time entries |
| POST | `/tasks/todos` | Create todo |
| PATCH | `/tasks/todos/{id}` | Update a todo |
| POST | `/tasks/time-entries` | Create time entry |
| PATCH | `/tasks/time-entries/{id}` | Update a time entry |
| DELETE | `/tasks/time-entries/{id}` | Delete a time entry |
| GET | `/tasks/time-entries/history` | Aggregated time entry history |
| GET | `/tasks/time-entries/summary` | Today + week totals |
| GET | `/tasks/time-entries/active` | Currently running time entry |

## Database Schema

### Habits Domain
- `habits` - id, name, description
- `habit_logs` - habit_id, log_date, value (unique per habit+date, upsert pattern)

### Tasks Domain
- `projects` - hierarchical (self-referencing parent_id), with started_at/finished_at
- `tasks` - belong to optional project, with started_at/finished_at
- `todos` - checklist items under a task
- `time_entries` - time tracking per task (started_at/finished_at)

Recursive CTE used for project tree traversal (`GetProjectWithDescendants`).

## Testing Strategy

Three test levels orchestrated via `Makefile`:

| Level | Command | Scope |
|---|---|---|
| Unit | `make test-unit` | Handler/service/repository with mocks, no DB |
| Integration | `make test-integration` | Repository against real test DB |
| E2E | `make test-e2e` | Full HTTP requests against running API + DB |

Test DB is created/destroyed per run. E2E tests use a custom `APIClient` helper.

## Deployment

- Multi-stage Docker build (golang:alpine builder -> alpine runtime)
- Non-root user in container
- Docker Compose with `db` (postgres:15-alpine) + `gv-api` on external `gv` network
- Migrations applied via Docker entrypoint (volume mount to `/docker-entrypoint-initdb.d`)
