# Architecture

## Overview

**gv-api** is a single-user REST API in Python that centralises personal data:
habits, tasks (projects/tasks/todos/time entries), day planning, finance,
route marks, calendar events, 3D printers, backups and the uptime of the home lab.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Python 3.14, managed with [uv](https://docs.astral.sh/uv/) |
| HTTP | [FastAPI](https://fastapi.tiangolo.com/) on [uvicorn](https://www.uvicorn.org/), one worker |
| Database | PostgreSQL 15 |
| Data access | [SQLAlchemy 2.0 Core](https://docs.sqlalchemy.org/) (async) over [asyncpg](https://github.com/MagicStack/asyncpg) |
| Models | [Pydantic v2](https://docs.pydantic.dev/), config with pydantic-settings |
| Migrations | [Alembic](https://alembic.sqlalchemy.org/), run at startup |
| Auth | JWT ([PyJWT](https://pyjwt.readthedocs.io/)) + TOTP 2FA ([pyotp](https://pyauth.github.io/pyotp/)) |
| Outbound HTTP | [httpx](https://www.python-httpx.org/) (PrusaLink, Prusa Connect, Telegram) |
| AWS | boto3 (SSM and S3, for backups and the failover) |
| External tools | `pg_dump`/`psql` (backups, restore), `ffmpeg` (camera, recordings) |
| Quality | ruff, mypy `--strict`, pytest + pytest-asyncio + pytest-cov |
| Containerization | Docker multi-stage build (uv builder -> python:alpine) + Docker Compose |

## Project Structure

```
gv-api/
  src/gv_api/
    main.py                  # composition root: builds services, workers, routers, lifespan
    cli.py                   # `gv-api serve | backup | prusa-login`
    config.py                # environment configuration (pydantic-settings)
    core/                    # shared HTTP plumbing: encoding types, body/id/query parsing,
                             # errors, request id, JSON logging, CORS, workers, failover guard
    db/                      # engine factory, shared MetaData, startup migration runner
    alerts/                  # Telegram handler for ERROR logs
    history/                 # shared history types and period maths
    <domain>/                # router.py, service.py, repository.py, schemas.py, tables.py,
                             # errors.py (only the files the domain needs)
  migrations/                # Alembic env, versions/, baseline.sql
  tests/{unit,integration,e2e}/
  scripts/                   # demo data, dev failover, operator helpers
  bench/                     # load benchmarks
  docs/                      # api/, business_logic/, data_models/
```

Domains: `auth`, `habits`, `tasks`, `plan`, `capacity`, `finance`, `calendar`, `rutas`,
`printers`, `uptime`, `backup`, `failover`, `restore`, `report`.

`capacity` has no table: daily free hours come from `DAILY_CAPACITY_HOURS`, and busy hours
from the plan service (see [plan](business_logic/plan.md)). `report` has no router of its own.

## Architecture Pattern

### Router -> Service -> Repository

Every domain follows the same three layers:

- **Router**: HTTP only. Parses the body, path ids and query, calls the service and writes the
  response with `core.respond`.
- **Service**: business rules. Takes its repository and collaborators in its constructor.
- **Repository**: data access with SQLAlchemy Core, returning Pydantic models.

There is one Pydantic model per request or response shape, shared by router and service.
Validation only runs on HTTP input; rows from the database are trusted.

### Composition root

`main.py` builds every repository and service once, in the FastAPI lifespan, and keeps them in
a container on `app.state`. Routers get their service through a small dependency. Tasks and plan
depend on each other, so the tasks service receives the capacity and plan providers with a
setter after construction.

### Database access

Each domain declares its tables in `tables.py` on one shared `MetaData`. Queries are Core
expressions, with `text()` and bound parameters where SQL is clearer (recursive CTEs, the
streak function). `engine.connect()` runs in autocommit for reads and single-statement
writes; `engine.begin()` is a transaction for several statements that must apply together.

### Concurrency

Everything is async on one uvicorn worker, because some state lives in the process (failover
mode, printer transfers, the camera, recordings). boto3 runs in threads; `pg_dump`, `psql` and
`ffmpeg` run as async subprocesses. Background workers (backup schedule, uptime heartbeat,
printer polling and auto-record, Telegram flush) are asyncio tasks started and cancelled by the
lifespan.

### Authentication

1. `POST /login` — password check. The private password returns a 5-minute
   `tmp` token; the semiprivate one returns a 30-day `semi` token.
2. `POST /login/2fa` — tmp token + TOTP code returns a 30-day `full` token.
3. Routes are grouped by the kinds they accept: `printers`, `uptime` and `rutas`
   take `semi` or `full`, everything else requires `full`.

Single-user system: no user table, passwords come from the environment.

## Testing

See the [README](../README.md#testing).

## Deployment

- Multi-stage Docker build (uv builder -> python:alpine runtime with the Postgres 15 client,
  ffmpeg and DejaVu fonts), non-root uid 1000.
- Docker Compose with `db` (postgres:15-alpine) + `gv-api` on the external `gv`
  network.
- Migrations run from the API at startup, not from the database image.
- Gitea Actions deploys on push to `main`: ruff, mypy and the unit tests with the coverage
  gate; a backup from the running container; then `docker compose up --build --wait`; and the
  image push to ECR for the AWS side.

### Where it runs

The homelab host, reached as `ssh.lab-ocp.com` (ssh over `cloudflared access
ssh`). The stacks live in `/home/ocp/docker/gv/{gv-api,gv-web}`.

Both services are published through a Cloudflare tunnel, with valid
certificates and no Cloudflare Access in front:

| Hostname | Service |
|---|---|
| `https://gv.lab-ocp.com` | gv-web (`ORIGIN`) |
| `https://gv-api.lab-ocp.com` | gv-api (`VITE_API_URL`; `ALLOWED_ORIGINS` is just the web host) |

Their ingress rules live in the Cloudflare dashboard, not in
`/etc/cloudflared/config.yml` on the host.
