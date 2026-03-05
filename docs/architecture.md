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
| GET | `/tasks/tree` | Active project/task tree |
| GET | `/tasks/projects` | Root (unfinished, parentless) projects |
| GET | `/tasks/projects/{id}/children` | Project with descendants, tasks, todos, time stats |
| POST | `/tasks/projects` | Create project |
| POST | `/tasks/tasks` | Create task |
| POST | `/tasks/todos` | Create todo |
| POST | `/tasks/time-entries` | Create time entry |
| PATCH | `/tasks/time-entries/{id}/finish` | Finish a time entry |
| PATCH | `/tasks/tasks/{id}/finish` | Finish a task |
| PATCH | `/tasks/projects/{id}/finish` | Finish a project |
| GET | `/tasks/tasks/{id}/time-entries` | Task detail with time entries |

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

---

## Improvement Opportunities

### Code Quality

1. **`GetActiveTree` is in the repository layer but contains significant business logic** (tree building, sorting started vs unstarted tasks). This should live in the service layer, with the repository only returning flat data.

2. **`GetProjectChildren` same issue** - complex tree assembly, time accumulation, and sorting logic in the repository. Move to service.

3. **`GetTaskTimeEntries` computes `timeSpent` in Go** instead of in SQL. The `GetTasksByProjectIDs` query already does this in SQL - be consistent. Either compute in SQL for both or in Go for both.

4. **Inconsistent ID generation**: `habits` uses `GENERATED ALWAYS AS IDENTITY`, `tasks` schema uses `SERIAL`. Pick one convention (prefer `GENERATED ALWAYS AS IDENTITY` - it's the modern approach).

5. **Migration file naming**: `001_habits.sql`, `002_tasks.sql` use sequential numbering but the Makefile's `migration` target generates timestamp-based names. These conventions conflict.

6. **Missing `is_done` column usage in tasks**: The schema has `idx_tasks_is_done` index on `tasks(is_done)` but there's no `is_done` column in the tasks table - this migration would fail. The index references a non-existent column.

7. **`GetRootProjects` query selects `started_at`** but the handler maps to `ProjectResponse` which has `StartedAt` and `FinishedAt` fields - the query doesn't return `finished_at` since it filters `WHERE finished_at IS NULL`, but also doesn't include `finished_at` in the SELECT, causing potential sqlc type mismatches.

### Security

8. **CORS allows all origins** (`AllowedOrigins: []string{"*"}`). For a personal API this may be intentional, but worth restricting to known frontends.

9. **Default secrets in config** (`Password: "Abc123.."`, `JwtSecret: "secret"`, `TotpSecret: "secret"`). These defaults are dangerous if the env vars aren't set in production.

### Architecture

10. **No graceful shutdown**: `http.ListenAndServe` blocks without signal handling. The server won't drain connections on SIGTERM.

11. **No structured logging**: Uses `log.Printf` throughout. Consider `slog` (stdlib since Go 1.21) for structured, leveled logging.

12. **No request logging middleware**: No visibility into incoming requests, response times, or status codes.

13. **Error messages leak to client**: `auth/handler.go` returns `err.Error()` directly in some cases, which could expose internal details.

14. **No input validation library**: Validation is manual and inconsistent - some handlers check required fields, others don't. The habits `GetDaily` handler doesn't validate date format before passing to service.

### Testing

15. **E2E `truncateTables` only truncates habits tables**, not tasks tables. Task E2E tests may have stale data issues.

16. **No linter config visible** (`.golangci.yml` or similar), though the `//nolint` comment in main.go suggests one is used.

### Minor

17. **`docker-compose.yaml` container names** still reference "habits" (`habits_db`, `habits_api`) even though the project now includes tasks.

18. **Dockerfile copies migrations** to runtime image but they're only needed at DB init time (handled by docker-compose volume mount). The COPY is redundant.

19. **`response.Error` uses `map[string]string`** for error responses while handlers sometimes use `http.Error` directly (in middleware). Inconsistent error response format.
