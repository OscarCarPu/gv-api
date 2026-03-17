# gv-api

A comprehensive life orchestrator built in Go, designed to centralize data from multiple web services, platforms, and devices.

## Tech Stack

- **Go** — system core
- **`go-chi/chi/v5`** — lightweight, idiomatic HTTP router
- **`pgx/v5` & `sqlc`** — efficient PostgreSQL interaction with auto-generated type-safe queries
- **`testify` & `mockery`** — testing assertions and auto-generated interface mocks with type-safe expecters

## Coverage

| File | Coverage |
| :--- | :---: |
| `gv-api/internal/auth/handler.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |
| `gv-api/internal/auth/middleware.go` | ![92.8%](https://img.shields.io/badge/92.8%25-brightgreen) |
| `gv-api/internal/auth/service.go` | ![96.0%](https://img.shields.io/badge/96.0%25-brightgreen) |
| `gv-api/internal/habits/handler.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |
| `gv-api/internal/habits/period.go` | ![100.0%](https://img.shields.io/badge/100.0%25-brightgreen) |
| `gv-api/internal/habits/repository.go` | ![0.0%](https://img.shields.io/badge/0.0%25-red) |
| `gv-api/internal/habits/service.go` | ![76.6%](https://img.shields.io/badge/76.6%25-brightgreen) |
| `gv-api/internal/response/response.go` | ![87.5%](https://img.shields.io/badge/87.5%25-brightgreen) |
| `gv-api/internal/tasks/handler.go` | ![94.1%](https://img.shields.io/badge/94.1%25-brightgreen) |
| `gv-api/internal/tasks/repository.go` | ![0.0%](https://img.shields.io/badge/0.0%25-red) |
| `gv-api/internal/tasks/service.go` | ![13.1%](https://img.shields.io/badge/13.1%25-red) |
| `gv-api/test/e2e/client.go` | ![75.0%](https://img.shields.io/badge/75.0%25-brightgreen) |
| `gv-api/test/e2e/setup.go` | ![76.7%](https://img.shields.io/badge/76.7%25-brightgreen) |
| **Total** | ![62.5%](https://img.shields.io/badge/62.5%25-yellow) |

> Untested code not shown above is either auto-generated, boilerplate delegation, or covered by E2E.

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

## Testing

```bash
make test-coverage
```
