# Design

## Context

See proposal.md for motivation and scope. The current service:

- **`cmd/api/main.go`** is the composition root. It wires roughly 15 services and three route tiers (public, semi, full), and runs four kinds of background work: the backup schedule, the uptime heartbeat, printer auto-record and the failover gate.
- **`internal/<domain>/`** follows handler → service → repository. Data access is sqlc over pgx, using `db/queries/*.sql` (about 1,240 lines of SQL).
- **Circular dependency:** tasks and plan depend on each other. The urgency providers (capacity, plan) are injected into the tasks service with a setter after construction.
- **In-process state** that must live in exactly one process: failover mode and merge, printer transfers, the warm ffmpeg camera process, active recordings, the Prusa Connect session, and caches.
- **External processes and services:** `pg_dump`/`psql` for backups and restore, `ffmpeg` for camera and recordings, PrusaLink (HTTP Digest), Prusa Connect, and AWS SSM and S3. (Go's `internal/pipeline`, a read-only connection to central-pipeline, has no consumer since uptime moved to its own heartbeat, so it is not ported.)
- **Tests:** about 12k lines of Go tests. Unit tests use mockery, repository integration tests run against a real DB, and an 824-line e2e suite runs over HTTP.

The constraints come from the specs: client-compatible HTTP (`http-platform`, `auth`, `api-surface`), a self-contained migration history (`schema-migrations`), the same runtime behavior (`runtime-lifecycle`), and Telegram alerts (`error-alerts`).

## Goals / Non-Goals

**Goals:**
- Reach parity that is proven by an automated comparison against the last Go release, not only argued.
- Keep one obvious place for each concern, with the same layering as today so domains port almost one to one.
- Keep the Python code small: no duplicated model layers, and comments only where the reason is not obvious.
- Prefer simple, idiomatic Python over byte-for-byte parity. When matching the Go service would need extra code (hand-made encoders, ports of Go library behavior, accidental orderings), the simpler behavior wins: the difference goes into `tests/parity/ACCEPTED.md`, and gv-web is changed if it depends on it. The code does not refer to the Go service.

**Non-Goals:**
- Fixing API warts. Known oddities are ported as they are and listed for a later change.
- Schema changes. The baseline is exactly the Go release's schema at version 036.
- Horizontal scaling. One process with one worker, as today.
- Porting lights.

## Decisions

### D1. Toolchain

| Concern | Choice | Alternatives rejected |
|---|---|---|
| Python | 3.14 | 3.13 (no reason to start older) |
| Packaging | uv: `pyproject.toml`, `uv.lock`, `uv run` | poetry and pip-tools (slower, two tools) |
| Lint and format | ruff, line length 100, rules `E,F,I,UP,B,SIM,ASYNC,RUF,S` | black + flake8 + isort |
| Types | mypy `--strict` with the pydantic plugin, required in CI | basedpyright (also fine, but mypy is the more common default) |
| Tests | pytest, pytest-asyncio (auto mode), httpx `AsyncClient` | — |
| Coverage | pytest-cov. CI fails under **85% overall** or **90% on `service.py` files** | no threshold |
| Test doubles | Hand-written fakes over `typing.Protocol` for repositories; `AsyncMock` only at external edges (PrusaLink, AWS, Telegram) | mocks everywhere (brittle, as mockery showed) |

Comments are kept to a minimum: only a non-obvious reason or an external constraint. There are no docstrings on private helpers.

### D2. Runtime libraries

FastAPI + uvicorn[standard], SQLAlchemy 2.0 (asyncio) + asyncpg, Alembic, Pydantic v2 + pydantic-settings, PyJWT, pyotp, httpx and boto3. Nothing else at runtime. Recurrence in `plan` is hand-written in Go today (no rrule library), so it is ported as is and needs no python-dateutil.

### D3. Layout

```
src/gv_api/
  main.py              # app factory + lifespan: the composition root, like main.go
  cli.py               # `serve` | `backup` | `prusa-login` (argparse)
  config.py            # pydantic-settings; same env names and defaults
  core/                # encoding types, errors, body/id/query parsing, request id,
                       # logging, CORS, health, failover-side guard, route helpers
  db/                  # engines, shared MetaData, startup migration runner
  alerts/              # Telegram logging handler
  auth/ habits/ tasks/ plan/ capacity/ finance/ calendar/ rutas/
  uptime/ backup/ failover/ restore/ report/ history/ printers/
      router.py  service.py  repository.py  schemas.py  tables.py  errors.py
migrations/            # alembic env.py, versions/ (one linear history), baseline.sql
alembic.ini            # for the alembic CLI (the deploy's one-time stamp)
tests/{unit,integration,e2e,parity}/
```

A domain only has the files it needs. For example, `capacity` has no `tables.py` and `report` has no router.

### D4. Composition root and DI

`main.py` builds every repository and service in the FastAPI lifespan, in the same order as `main.go`, and stores them in a `Container` dataclass on `app.state`. Routers read their service through one small dependency, `svc = Depends(service(HabitsService))`. Services take collaborators in their constructor. The tasks ↔ plan cycle keeps Go's setter (`tasks.set_urgency_providers(capacity, plan)`).

*Alternative:* nested FastAPI `Depends` graphs. Rejected because the construction order becomes implicit, and singletons with in-process state (printers, failover) are easy to build twice by accident.

### D5. Concurrency model

Everything is async, and uvicorn runs **one worker**. In-process state needs a single owner, and load is one user. Blocking work is pushed off the event loop:

- boto3 runs through `anyio.to_thread.run_sync`.
- `pg_dump`, `psql` and `ffmpeg` run through `asyncio.create_subprocess_exec`.
- File IO on large media goes through threads (Starlette `FileResponse` already does this).

Background workers are `asyncio` tasks started in the lifespan and cancelled on shutdown. uvicorn's `timeout_graceful_shutdown=10` gives the 10-second drain.

### D6. Data access

- **Tables:** each domain declares its own tables in `tables.py` on one shared `MetaData` (`db/metadata.py`). Alembic imports every domain's `tables.py`. An integration test compares the declared metadata with the migrated schema (columns, types, nullability), so the declarations cannot drift.
- **Queries:** queries are ported from `db/queries/<domain>.sql` to SQLAlchemy Core expressions. Recursive CTEs, the effective-due view, and calls to SQL functions such as the habit streak function may use `text()` with bound parameters, where that is clearer than Core. The `.sql` files are deleted once their domain is ported.
- **Connections and transactions:** the pool has no pre-ping and is LIFO. `engine.connect()` runs in autocommit, one round trip per statement, and serves reads and single-statement writes; `engine.begin()` is a transaction, for several statements that must apply together. Multi-step service operations receive a connection explicitly.
- **Row mapping:** repositories return Pydantic models built with `Model.model_construct(**row._mapping)`, which skips validation for trusted DB data (see D7). Large lists can instead come from Postgres as one `json_agg` array, validated with a `TypeAdapter` in compiled code.

### D7. One Pydantic model per shape

There is a single model per request or response shape, shared by router and service, and no parallel dataclass layer. Validation runs only on input coming from HTTP. Pydantic v2's core is compiled, so per-object overhead is microseconds. The real cost would be duplicate types, and this decision avoids it.

Shared building blocks live in `core/encoding.py`:

- `ApiModel(BaseModel)` with `extra="ignore"` and lax mode; request fields default to their zero value.
- `Money`, `Timestamp` and `Day` are plain `Decimal`, `datetime` and `date`, encoded by Pydantic's compiled serializer: an amount is a string that keeps its scale (`"12.50"`), an instant is RFC 3339 with `Z` and six fractional digits when it has any, a date is `YYYY-MM-DD`. Python-level serializers on these fields cost about 6 µs per row on large lists, so the exact Go formats (`"12.5"`, `.12Z`) were dropped; clients parse both.
- `DayTime` writes a date column as midnight UTC where the API types it as a timestamp.
- `Real` writes a SQL `real` with 7 significant digits.

Null versus `[]` is decided per field by its declared type: `list[X] | None = None` where Go emitted `null`, and `list[X] = []` where it emitted `[]`. Every field is always serialized: keys Go left out when empty are present as `null` or `[]`, so there is no `omitempty` machinery. The one exception is the printer routes: gv-web's printer schemas read their fields as optional, not nullable, so printer responses leave `None` out (`exclude_none`). The parity harness catches any other mistake.

Responses go through `core.respond(status, data)`, which serializes models, lists and plain dicts with `pydantic_core.to_json`. FastAPI's `response_model` is not used, which avoids validating twice and keeps encoding in one place.

### D8. Request parsing

These are the simplest form that keeps the existing error messages:

- `body = await parse_body(request, Model, "Invalid Body")` returns the handler's 400 on invalid JSON or a value that cannot be coerced.
- `id = parse_id(raw, "habit")` returns 400 `invalid habit id` unless the value is a positive int32.
- `q = query(request, "date")` returns the first value, or `""`.

Path parameters are declared as `str` and query parameters are not declared, so FastAPI never produces a 422. Bodies are parsed with `json.loads`, so content after the JSON value is a 400. A PATCH field where `null` means "clear" and absence means "keep" is told apart with `model_fields_set`.

### D9. HTTP settings

- `FastAPI(docs_url=None, redoc_url=None, openapi_url=None)`: no extra public routes.
- `redirect_slashes=False`: `/habits/` is a 404, not a redirect.
- Unmatched routes, wrong methods and unhandled exceptions use the framework defaults: 404 `{"detail": "Not Found"}`, 405 with an `Allow` header, and a logged traceback with Starlette's plain 500.
- HEAD is not added to GET routes.
- Static subpaths (`list-fast`, `by-due-date`, `future`, `active`, `summary`, `history`, `meta`, `progress`, `rename`, …) are registered before `{id}` routes. A route-inventory test compares the app's (method, path, tier) table with `tests/parity/routes.txt`, which is generated once from the Go router.
- Middleware is pure ASGI, outermost first: request id, then CORS (Starlette `CORSMiddleware` with the spec's lists and `max_age=300`), then the failover hold lock, then the routes. Auth and the AWS-side guard are router-level dependencies per tier.

### D10. Time

- Timestamps are used exactly as asyncpg returns them (aware, UTC), with no conversion layer.
- `TIMEZONE` becomes one `ZoneInfo` held by the services that used `loc` in Go. It is used only to decide "today" and day or month boundaries, as Go does.
- The clock is injected (`now: Callable[[], datetime]`) wherever Go injected `time.Now`, so tests and the harness can control it.

### D11. Migrations with Alembic

The Go migrations are history, not something the new app builds on. Nothing in Python, the image or `bench/` reads `db/`, so `db/` goes away at cutover.

- **Baseline:** `migrations/baseline.sql` is a `pg_dump --schema-only --no-owner --no-privileges` of an empty database migrated by the `go-final` image, without `schema_migrations`. Revision `0001_baseline` runs it on the asyncpg driver connection, inside Alembic's transaction, because asyncpg's prepared statements take one statement at a time.
- **Startup:** the runner only does `alembic upgrade head`, in one transaction, under a `pg_advisory_xact_lock`. It does not look at `schema_migrations`. A populated database without history fails on the baseline's first `CREATE`, rolls back and stops startup.
- **Cutover is a deployment step:** before starting the new container, the cutover deploy runs `alembic stamp 0001_baseline` against production, only when `alembic current` reports no revision, so later deploys skip it. Development databases are stamped once by hand (`make py-db-stamp`).
- **Rollback safety:** `schema_migrations` is never touched. The Go image carries its own migrations, so it starts on the stamped database until a newer revision exists.
- **When it runs:** `lifespan` calls the runner through `run_sync`. It is skipped while the failover holds the app, and it runs again after a restore swap, mirroring Go.
- New migrations are hand-written Alembic revisions. Autogenerate is allowed as a starting point, but it misses views, functions and triggers.

*Alternatives:* a golang-migrate-compatible runner (rejected: Alembic is the standard tool), and adoption logic at startup (rejected: it carries a one-time transition into the app forever; the deploy step does the same once).

### D12. Logging and Telegram alerts

- **Logging:** stdlib `logging` with a small JSON formatter (`time`, `level`, `msg` plus extras). A `contextvars` filter adds `request_id`. Uvicorn's access log is disabled, because Go did not log requests.
- **Telegram handler:** `alerts.TelegramHandler(logging.Handler)` is attached at ERROR level. `emit` only appends to a bounded in-memory buffer, so it never blocks.
- **Flushing:** a lifespan task flushes every 10 s. It groups records by message with a count, renders one message capped at 4,096 characters, and posts it to `sendMessage` with httpx (5 s timeout). Failures are logged at WARNING on a logger excluded from the handler.
- **Fatal exit:** `cli.py` flushes with a 5 s cap before exiting.
- **Secrets:** the token only appears in the request URL and is never logged.

### D13. Integrations

- **PrusaLink:** `httpx.AsyncClient(auth=httpx.DigestAuth(...))`, or an API key header.
- **Prusa Connect:** same token-file format, so an existing session carries over.
- **Uploads:** chunked uploads keep the `X-Upload-*` protocol and write to `PRINTER_UPLOADS_DIR`.
- **Camera:** one supervised `ffmpeg` subprocess per printer, with a 5-minute idle-out. MJPEG is served as a `StreamingResponse` with `multipart/x-mixed-replace`.
- **Recordings:** served with `FileResponse` (Range support). Range responses (206, `Content-Range`, 416) are checked against Go's `http.ServeContent` in the harness.
- **Signed URLs:** the same HMAC-SHA256 input string and expiry rounding as Go.
- **AWS:** boto3 clients for SSM and S3. moto stays in `docker-compose.dev-aws.yaml`.

### D14. Test strategy

| Level | What | Where |
|---|---|---|
| Unit | services with Protocol fakes; encoding helpers; parsing; alerts batching | `tests/unit` |
| Integration | repositories and migrations against the compose test DB, truncated per test | `tests/integration` |
| E2E | the Go e2e suite ported as pytest over HTTP against the running container | `tests/e2e` |
| Parity | Go and Python side by side (D15) | `tests/parity` |

Each domain group ports that domain's existing Go tests, and its coverage counts toward the D1 thresholds.

### D15. Parity harness

- **What it runs:** a separate compose project (`docker-compose.parity.yaml`, driven by `make parity-up`) starts both apps at the same time with their own Postgres, each on its own copy of the same snapshot, without touching the dev stack. The Go reference image is built from tag `go-final` (`d32ab42`).
- **Snapshot:** cloned with `CREATE DATABASE … TEMPLATE`. Either the demo seed (`db/demo.sql`, moved to `scripts/demo.sql` at cutover) or a local production backup from `./backups` (`PARITY_SNAPSHOT=...`), which exercises real data shapes.
- **Corpus:** one YAML file per domain in `tests/parity/corpus/`. Each entry has the method, path, tier, body, and a list of volatile JSON paths. Ids created by earlier writes are captured and substituted by name. Tables written only by background workers (the uptime heartbeat) can be ignored per corpus until that worker is ported.
- **Runner:** sends each request to both apps concurrently and compares status, selected headers and decoded bodies. At the end it compares both databases table by table, ignoring volatile columns.
- **Printers:** reads (status, files, history, chamber, recordings list) run against the real printer, with telemetry paths marked volatile. Writes are not replayed, because each write would hit the physical printer twice. They are covered by recorded-response unit tests and a manual checklist (task 14.x).
- **Ownership:** each domain group adds its own corpus file and must pass it before the group is done.

## Risks / Trade-offs

- **[Undocumented behavior lost]** → The harness runs against a production snapshot, not only demo data. Every difference is fixed or recorded as an accepted deviation in `tests/parity/ACCEPTED.md`.
- **[Encoding drift: null vs `[]`, decimals, timestamps]** → Use the typed helpers from D7 only, and the harness diffs decoded bodies.
- **[Framework defaults leak (422, 307, HEAD, `/docs`)]** → D9 settings plus explicit tests for each.
- **[Lax coercion accepts `"5"` for an int]** → Accepted and written into the http-platform spec. No client sends this.
- **[A client relies on a detail that changed]** → Each difference is in `ACCEPTED.md`; gv-web was checked for absent-vs-null fields and CORS, and is changed in its own repo if a difference matters.
- **[Event loop blocked by sync work]** → Rule: boto3, file and subprocess IO always go through threads or async subprocesses. Ruff's `ASYNC` rules flag blocking calls.
- **[Cutover stamp forgotten or run on the wrong database]** → Forgetting it fails safe: the baseline's first `CREATE` errors, the transaction rolls back and the app does not start. The stamp only runs when `alembic current` is empty, and the CI pre-deploy backup still runs first.
- **[Rollback window closes on the first new migration]** → Documented. Rolling back after that needs a down step written at that time.
- **[Python slower than Go]** → The baseline in `benchmarks.md` shows the heavy endpoints are SQL-bound (Go CPU is 0.5–12 ms per request against 25–200 ms in Postgres), and the D7 encoding already matches Go's cost on a 5,000-row payload. Gate: at scenario `5y` of `bench/run.sh`, on the same machine, every endpoint's median p95 over 3 runs ≤ max(1.5 × Go's, Go's + 5 ms). There is a checkpoint after H3 and the full gate before cutover.
- **[Telegram noise]** → 10 s batching and deduplication. ERROR-level discipline stays as in Go.
- **[Camera and ffmpeg lifecycle is subtle]** → Port the Go printer tests first, then use the manual checklist on real hardware.

## Migration Plan

1. Build Python under `src/` while Go keeps running in production. Both coexist in the repo, and Make targets for Python get a `py-` prefix until cutover.
2. Each domain group finishes with its unit, integration and parity checks green.
3. Cutover gate:
   - the full parity run is green against a fresh production backup
   - the printer checklist is done on the real printer
   - the latency gate holds
   - coverage thresholds are met
4. Tag `go-final`, then in one commit on `develop`:
   - switch the Dockerfile, compose (drop the D-Bus mount), Makefile and CI to Python
   - add `TELEGRAM_*` to `.env.example`
   - delete `cmd/`, `internal/`, `test/`, `go.*`, `sqlc.yaml`, `.mockery.yaml` and the whole `db/` directory, after moving `db/demo.sql` to `scripts/demo.sql`
   - rewrite README and `docs/architecture.md`, and delete `docs/api/lights.md`
5. Merge to `main`. The pre-deploy backup step runs against the container that is still up, which is Go on the cutover deploy. So the new `deploy.yml` calls `gv-api backup || ./main backup`, and **that one deploy treats a failed backup as fatal** instead of a warning. Then the new image is built and `alembic stamp 0001_baseline` runs against production through `docker compose run`, only if `alembic current` is empty. Then `docker compose up --build --wait` runs, and the logs must show no pending migration. **Cutover only with the failover state `NORMAL`**: a dump restored from the Go era would have no Alembic history.
6. **Rollback:** redeploy the `go-final` image. No data step is needed while no new migration exists.
7. **Follow-ups outside this change:** remove the lights screens in gv-web and Android, and later drop the `lights` table and `schema_migrations`.
