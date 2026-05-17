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
- mockery

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
| `DATABASE_URL` | **Yes** | — | PostgreSQL connection string. The server will not start without this. |
| `PORT` | No | `8080` | HTTP listen port. |
| `TIMEZONE` | No | `Europe/Madrid` | IANA timezone for date arithmetic. |
| `PASSWORD` | No | `Abc123..` | Login password for full-access tokens. |
| `SEMIPRIVATE_PASSWORD` | No | `Abc123..` | Login password for read-only tokens. |
| `JWT_SECRET` | No | `secret` | Secret used to sign JWTs. **Change in production.** |
| `TOTP_SECRET` | No | `secret` | Base32 secret for TOTP 2FA. **Change in production.** |

## API

### Infrastructure

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/health` | None | Returns `200 OK`. Use for liveness probes. |

### Request / Response Headers

| Header | Direction | Description |
|---|---|---|
| `X-Request-ID` | Request & Response | Optional on request; auto-generated (random 8-byte hex) if absent. Echoed back in the response and propagated through logs for correlation. |

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

