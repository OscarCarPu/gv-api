# Proposal

## Why

gv-api is roughly 20k lines of handwritten Go (chi + pgx + sqlc + mockery) across 11 live domains, plus backups, failover/restore and printer integrations. We want the service on Python/FastAPI so it shares one language and toolchain with the rest of the stack we maintain, and so new domains need less boilerplate per endpoint (handler/service/repository/dto/mocks). We are doing it now, while the API contract is stable and fully documented in `docs/api/`, so the rewrite can be checked against a fixed target.

## What Changes

- Replace the Go service (`cmd/`, `internal/`, `go.mod`, `sqlc.yaml`, mockery config) with a FastAPI application in the same repository. It runs on uvicorn, uses SQLAlchemy 2.0 Core (async, asyncpg driver) for data access and Pydantic for request and response models.
- Keep the HTTP contract **exactly**: same paths, methods, auth tiers, status codes, JSON field names and value encodings, error bodies, headers and CORS policy. gv-web and the Android app keep working without changes, and JWTs issued by the Go service stay valid after the switch.
- Keep the database as it is. Migrations move to Alembic: one baseline revision runs the untouched `001`–`036` SQL on fresh databases, and existing databases (golang-migrate at version 36) are adopted without re-running anything. `schema_migrations` is left in place, so a rollback to the Go image works until the first post-rewrite migration.
- Add Telegram alerts for every ERROR log, batched and deduplicated, and off when the bot is not configured.
- Port every live domain: auth, habits, tasks (projects/tasks/todos/time entries), plan, capacity, finance, calendar, rutas, printers (PrusaLink, Prusa Connect, camera, recordings, uploads), uptime (including the read-only central-pipeline database), backups (pg_dump, retention, S3) and the AWS failover (SSM/S3 gate, restore, report, lock).
- Port the background work and CLI entry points: the backup schedule, the uptime heartbeat, printer polling/auto-record, and the `backup` and `prusa-login` subcommands.
- **BREAKING**: Retire the lights domain. `/domotics/lights/*` is no longer served, and the BlueZ/D-Bus driver, its config (`LIGHTS_*`) and the D-Bus socket mount are removed. The `lights` table is left in place (no destructive migration).
- Replace the test stack (testify/mockery) with pytest. Port the existing e2e suite to run as black-box HTTP tests, and add a differential harness that replays a request corpus against both the Go and the Python builds and diffs the responses before cutover.
- Big-bang cutover: Python is built next to the Go code, parity is proven, and one deploy switches the image. The Go code is deleted in the same change, after parity is shown.

## Capabilities

### New Capabilities

There are no existing specs. These capture the cross-cutting behavior the rewrite must hold. Per-endpoint semantics stay in `docs/api/` and `docs/business_logic/`, which the specs adopt as the reference contract.

- `http-platform`: wire conventions shared by every endpoint. These cover JSON encoding (decimals, timestamps, null and empty collections), the error envelope, lenient body decoding, path id parsing, unmatched routes, CORS, `X-Request-ID`, structured logging, `/health`, and the 503 guard used on the failover side.
- `auth`: password login, TOTP second factor, the three token kinds (`tmp`, `semi`, `full`), token format compatibility, and which kinds each route group accepts.
- `api-surface`: the set of routes the API exposes and their auth tier, parity with the documented contract, and the retirement of the lights routes.
- `schema-migrations`: applying the migration history at startup, adopting databases migrated by the Go release, the rollback window, and the failover cases where migrations are skipped or re-run.
- `error-alerts`: sending ERROR logs to Telegram, with batching, deduplication and a guarantee that alerting never affects the service.
- `runtime-lifecycle`: process startup and shutdown, background workers (backups, heartbeat, printers), CLI subcommands, and how they behave on each failover side.

### Modified Capabilities

None. No specs exist yet.

## Impact

- **Code**: `cmd/`, `internal/`, `test/e2e/` (Go) are replaced by a Python package (`app/`) and `tests/`. `go.mod`, `go.sum`, `sqlc.yaml`, `.mockery.yaml` and `internal/database/gvdb` go away. `db/queries/*.sql` become reference material for the SQLAlchemy Core repositories and are removed once ported.
- **Build/deploy**: `Dockerfile` becomes a Python image that keeps `postgresql15-client`, `ffmpeg` and `font-dejavu`. The `Makefile` targets (`run`, `lint`, `test-*`, `backup`, `prusa-login`) are reimplemented with uv, ruff and pytest. The `.gitea/workflows/deploy.yml` test step changes from `go test` to the pytest unit suite, and the pre-deploy `./main backup` call changes to the new CLI. `docker-compose.yaml` drops the D-Bus socket mount.
- **Config**: every env var keeps its name and default except `LIGHTS_*`, which are removed. `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are new and optional.
- **Dependencies**: FastAPI, uvicorn, SQLAlchemy 2.0, asyncpg, Alembic, Pydantic v2, pydantic-settings, PyJWT, pyotp, httpx, boto3. Dev: uv, ruff, mypy, pytest, pytest-asyncio, pytest-cov.
- **Clients**: gv-web and Android need no changes, except that their lights screens stop working. Removing those screens is follow-up work in those repos.
- **Docs**: `README.md`, `docs/architecture.md` and the setup and testing sections are rewritten for Python. `docs/api/lights.md` is removed.
- **Rollback**: redeploy the last Go image. The schema and migration state are unchanged, so no data step is needed.
