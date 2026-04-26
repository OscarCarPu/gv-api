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

