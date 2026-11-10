# Tasks

Milestones: **H0** foundations (1–5) · **H1** parity harness (6) · **H2** simple domains (7–8) · **H3** tasks, plan, capacity and calendar (9–12) · **H4** finance (13) · **H5** uptime (14) · **H6** backups (15) · **H7** failover (16) · **H8** printers (17) · **H9** cutover (18).

## 1. H0 · Project bootstrap

- [x] 1.1 Create `pyproject.toml` for Python 3.14 with uv, with the runtime deps from design D2 and the dev deps from D1, and commit `uv.lock`. Verify that `uv sync` succeeds on a clean checkout.
- [x] 1.2 Configure ruff (line length 100, rules `E,F,I,UP,B,SIM,ASYNC,RUF,S`), mypy `--strict` with the pydantic plugin, and pytest (asyncio auto mode, coverage at 85% overall and 90% on `service.py`). Verify that `uv run ruff check`, `uv run mypy src` and `uv run pytest` all pass on the empty package.
- [x] 1.3 Create the `src/gv_api/` layout from D3 and the `tests/{unit,integration,e2e,parity}` directories. Add `py-run`, `py-lint`, `py-test-unit`, `py-test-integration` and `py-test-e2e` Make targets next to the Go ones. Verify that each target runs.
- [x] 1.4 Add `Dockerfile.py` (python:3.14-slim with `postgresql15-client`-compatible `pg_dump`/`psql`, `ffmpeg`, `fonts-dejavu`, non-root uid 1000, `/data/recordings` and `/data/prusa`). Verify that the image builds and that `docker run … gv-api --help` lists `serve`, `backup` and `prusa-login`.

## 2. H0 · HTTP platform

- [x] 2.1 Implement `config.py` with pydantic-settings: every `.env.example` name and default except `LIGHTS_*`, plus the optional `TELEGRAM_*`, with the same required-variable, `FAILOVER_SIDE`, `TIMEZONE` and retention checks. Verify with unit tests ported from `internal/config`, including the exact `required env vars not set: …` message.
- [x] 2.2 Implement JSON logging with the `request_id` contextvar filter, and disable uvicorn's access log. Verify with a unit test that one log line is one JSON object with `time`, `level`, `msg` and `request_id`.
- [x] 2.3 Implement `core/encoding.py`: `ApiModel`, `Money`, `Timestamp`, `Day` and `respond()`. Verify with table tests that use Go's own outputs as fixtures (`12.50` → `"12.5"`, `100.00` → `"100"`, whole-second and sub-second timestamps, null vs `[]`).
- [x] 2.4 Implement `parse_body`, `parse_id`, `query` and the error helpers (`error`, `error_code`, `internal_error`). Verify with unit tests for unknown fields, null → zero value, wrong type → handler message, and invalid ids (`abc`, `0`, `4294967296`).
- [x] 2.5 Build the app factory with the D9 settings (no docs routes, `redirect_slashes=False`, text 404, empty-body 405 with `Allow`, no automatic HEAD, empty-body 500), plus request-id middleware, CORS and `/health`. Verify with tests for each http-platform scenario, including CORS preflight with the gv-web headers and a foreign origin.
- [x] 2.6 Add the `serve` command and lifespan skeleton: start and cancel a worker registry, and drain for 10 s on SIGTERM. Verify with a test that SIGTERM during a slow request lets it finish and the process exits 0.
- [x] 2.7 Generate `tests/parity/routes.txt` (method, path, tier) once from the Go router (a small Go program walking `chi.Walk`, minus lights), and add the route-inventory test. It starts red and turns green as domains land.

## 3. H0 · Database and migrations

- [x] 3.1 Set up the async engine factory and the shared `MetaData`, with an integration fixture that truncates all tables between tests. Verify that the fixture runs against the compose test DB.
- [x] 3.2 Generate `migrations/baseline.sql` from an empty database migrated by the `go-final` binary (`pg_dump --schema-only --no-owner --no-privileges`, without `schema_migrations`), and set up Alembic: `alembic.ini`, `migrations/env.py` importing every domain's `tables.py`, and the `0001_baseline` revision running the snapshot. Verify that `upgrade head` on an empty DB gives the same `pg_dump --schema-only` as the Go release, apart from `alembic_version` and `schema_migrations`.
- [x] 3.3 Implement the startup runner: `upgrade head` in one transaction under an advisory lock, with no golang-migrate logic, called from the lifespan, and a non-zero exit when it fails. Verify with integration tests for a fresh DB, an up-to-date DB, a failing migration that leaves the schema untouched, and a populated DB without history that fails without changing data; and check that `serve` exits non-zero when migrations fail.
- [x] 3.4 Add the cutover stamp and check the rollback window: `make py-db-stamp` runs `alembic stamp 0001_baseline` only when `alembic current` is empty, and `make py-check-rollback` runs the Go binary on a fresh DB, stamps it, starts the Python app (no pending migration), then starts the Go binary again. Verify that the script passes and that a second stamp does nothing.
- [x] 3.5 Make Python, the image and `bench/` independent of `db/`: `Dockerfile.py` copies `migrations/` and `alembic.ini` instead of `db/migrations`, and `bench/run.sh` builds its template with `alembic upgrade head` plus golang-migrate's row for the Go runs. Verify by building the image and running the migrations from a copy of the repo without `db/`, and that `git grep` finds `db/` only in Go code and docs.

## 4. H0 · Telegram alerts

- [x] 4.1 Implement `alerts.TelegramHandler` and the 10 s flusher: bounded buffer, grouping with counts, the 4,096-character cap with a truncation note, and the side and `request_id` in each message. Verify with unit tests for batching, deduplication, truncation and the exact message format.
- [x] 4.2 Make delivery safe: httpx with a 5 s timeout, WARNING on failure from a logger excluded from the handler, and no token in any output. Verify with tests for an unreachable API, request latency left unchanged, and a token-absence check on captured logs.
- [x] 4.3 Add the disabled mode and the fatal-exit flush (at most 5 s) in `cli.py`. Verify with tests that unset variables produce one info line, and that a failing startup sends before exiting.
- [x] 4.4 Document `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` in `.env.example` and in the README section on environment variables.

## 5. H0 · Auth

- [x] 5.1 Port the auth service (PyJWT HS256 with `exp`, `iat` and `kind`; pyotp with SHA-1, 30 s steps and ±1 skew) and the `/login` and `/login/2fa` routes. Verify with tests ported from `internal/auth`, plus a test that a token signed by the Go service is accepted and vice versa.
- [x] 5.2 Add the tier dependencies (`full`; `semi` or `full`) with the exact 401 bodies. Verify with auth spec scenarios covering a semi token on a full route, a missing `Bearer `, an `alg: none` token and an expired token.

## 6. H1 · Parity harness and e2e

- [x] 6.1 Tag `go-final` at `d32ab42`, and add a compose `parity` profile running the Go image, the Python image and a Postgres with template cloning from `db/demo.sql` or `PARITY_SNAPSHOT`. Verify that `make parity-up` brings both apps up healthy on separate DB copies.
- [x] 6.2 Implement the corpus format (YAML per domain, captured ids, volatile paths) and the runner. The runner sends each request to both apps concurrently, diffs status, headers and decoded bodies, records latency, and diffs the final databases table by table. Verify that a deliberately broken Python response fails the run with a readable diff.
- [x] 6.3 Write corpora for health, auth, the 404/405/CORS behavior and `/domotics/lights/*` returning 404 (an expected difference listed in `tests/parity/ACCEPTED.md`). Verify that `make parity` is green for them.
- [x] 6.4 Port the Go e2e harness (`test/e2e/setup.go`, `client.go`) and `auth_test.go` and `health_test.go` to `tests/e2e`. Verify that `make py-test-e2e` passes against the Python container.

## 7. H2 · Habits

- [x] 7.1 Port the habits tables, repository (including the streak function and history queries) and its integration tests from `internal/habits/repository_test.go`. Verify with `py-test-integration -k habits`.
- [x] 7.2 Port the habits service, the shared `history` period maths and the router, with unit tests from `service_test.go`, `handler_test.go` and `internal/history`. Verify that coverage on the habits service is ≥ 90%.
- [x] 7.3 Port `test/e2e/habits_test.go` and add `tests/parity/corpus/habits.yaml` (daily view with and without a date, CRUD, logging, history per frequency, validation errors). Verify that e2e and parity are green.

## 8. H2 · Rutas

- [x] 8.1 Port the rutas tables, repository, service and router (semi tier) with their Go tests. Verify that unit and integration tests pass.
- [x] 8.2 Add `tests/parity/corpus/rutas.yaml` covering CRUD with semi and full tokens and the date parsing errors. Verify that parity is green.

## 9. H3 · Tasks

- [x] 9.1 Port the tasks tables (projects, tasks, todos, time entries, dependencies, effective-due view) and the repositories for projects, tasks, todos and time entries, with integration tests from `repository_test.go`. Verify with `py-test-integration -k tasks`.
- [x] 9.2 Port validation and the service: tree, list-fast, children, parent candidates, the single active time entry, summary and history, and budget. Unit tests come from `service_test.go`, `budget_test.go` and `validation_test`. Verify coverage ≥ 90%.
- [x] 9.3 Port the tasks router with static subpaths ordered before `{id}`. Verify with handler tests ported from `handler_test.go` and the route-inventory rows for `/tasks/*`.
- [x] 9.4 Port `test/e2e/tasks_test.go` and add `tests/parity/corpus/tasks.yaml` (excluding `by-due-date`, which needs plan and capacity, see 11.3). Verify that e2e and parity are green.

## 10. H3 · Plan

- [ ] 10.1 Port the plan tables (blocks, commitments, event refs) and the repository, with integration tests. Verify with `py-test-integration -k plan`.
- [ ] 10.2 Port the plan service, including block generation from commitments (`generate.go`, hand-written recurrence) and clipping of in-progress blocks, with unit tests from `service_test.go`. Verify coverage ≥ 90%.
- [ ] 10.3 Port the plan router (`/plan/today`, `/plan/range`, blocks with `future` before `{id}`, commitments). Add `tests/parity/corpus/plan.yaml` and verify that it is green.

## 11. H3 · Capacity and urgency

- [ ] 11.1 Port the capacity service and router (`DAILY_CAPACITY_HOURS`, busy hours from plan, injected today/clock) with unit tests. Verify that tests pass.
- [ ] 11.2 Wire the tasks ↔ plan ↔ capacity cycle in the composition root with `set_urgency_providers`. Verify with a unit test that urgency uses both providers.
- [ ] 11.3 Add `by-due-date` and `by-due-date/slack` to the parity corpus, plus `capacity.yaml`. Verify that parity is green.

## 12. H3 · Calendar

- [ ] 12.1 Port the calendar tables, repository, service (linked to the plan service) and router, with the Go tests and `test/e2e/calendar_test.go`. Verify that unit, integration and e2e tests pass.
- [ ] 12.2 Add `tests/parity/corpus/calendar.yaml` (RFC 3339 and date-only filters, links from plan blocks). Verify that it is green.
- [ ] 12.3 Latency checkpoint: run `bench/run.sh` with the Python app at scenario `5y`. Verify that the habits, tasks, plan, capacity and calendar endpoints meet the gate in design (median p95 over 3 runs ≤ max(1.5 × Go, Go + 5 ms)); investigate any miss before starting H4.

## 13. H4 · Finance

- [ ] 13.1 Port the finance tables (accounts, categories, transactions with the `transaction_type` enum, budgets) and the repository, with integration tests from `repository_test.go` and `repository_budgets_test.go`. Verify with `py-test-integration -k finance`.
- [ ] 13.2 Port the finance services (accounts, categories, transactions, budgets, overview, stats by category, estimation, monthly and net worth, month boundaries in `TIMEZONE`) with unit tests. Verify coverage ≥ 90% and the `Money` encoding on every amount.
- [ ] 13.3 Port the finance routers, with handler tests from `handler_test.go` and `handler_budgets_test.go`. Verify the route-inventory rows for `/finance/*`.
- [ ] 13.4 Add `tests/parity/corpus/finance.yaml` covering every route, month and year edges, and empty months. Verify that it is green against both demo data and a production snapshot.

## 14. H5 · Uptime

- [ ] 14.1 Port the uptime tables and repository (beat, windows, outages, `LastAlive`), with integration tests. Verify with `py-test-integration -k uptime`.
- [ ] 14.2 Port `pipeline`: a lazy read-only second engine, the retry while marts rebuild, and 503 when unset. Integration tests build the marts as fixtures, as in Go. Verify that tests pass.
- [ ] 14.3 Port the uptime service, router (semi tier, AWS-side guard) and heartbeat worker (30 s, 90 s gap, failover-gated, off on the AWS side). Verify with unit tests from `service_test.go` and `heartbeat_test.go`.
- [ ] 14.4 Add `tests/parity/corpus/uptime.yaml` with `computed_at` and `stale` marked volatile. Verify that it is green.

## 15. H6 · Backups

- [ ] 15.1 Port the backup service: async `pg_dump` with gzip, hourly and daily filing and pruning, the `pg_try_advisory_lock(hashtext('backup'))` lock, and the optional S3 upload through threads. Verify with unit tests from `service_test.go` and integration tests from `integration_test.go`.
- [ ] 15.2 Port the backup worker (wall-clock aligned, interval 0 disables it, failover-gated) and the `/backups` routes, including download. Verify with unit tests and handler tests.
- [ ] 15.3 Add the `backup` CLI command (10-minute cap, exit codes). Verify that `docker compose exec gv-api gv-api backup` writes a dump and exits 0, and that a concurrent run respects the lock.
- [ ] 15.4 Add `tests/parity/corpus/backups.yaml` (list and download shape; file names volatile). Verify that it is green.

## 16. H7 · Failover, restore and report

- [ ] 16.1 Port `report` (schema introspection and the three-way diff between base, home and EC2 rows) with unit tests from `internal/report`. Verify that tests pass.
- [ ] 16.2 Port `restore`: load the dumps through `psql`, `ALLOW_CONNECTIONS` and rename swap with undo, `_pre_restore` kept, ignore and skip lists, and `LastAlive`. Verify with integration tests from `restore_test.go`.
- [ ] 16.3 Port the failover controller: the startup gate with 30 s retry, the `hold` and `merge` modes, the gate the workers call, the hold lock middleware with exempt prefixes, `/failover*` with `X-Failover-Token`, `/report` GET and DELETE, the report file, and engine dispose plus migrations after the swap. Verify with unit tests from `failover_test.go`, `flow_test.go` and `outage_test.go`.
- [ ] 16.4 Rehearse with moto using `make up` and `scripts/failover-dev.sh` (scenario → merge → status → outage → finish) against the Python image. Verify that every step behaves as `docs/dev-failover.md` describes, and that a failed restore produces a Telegram alert.

## 17. H8 · Printers

- [ ] 17.1 Port the PrusaLink client (Digest or API key) and status, job stop, and history, chamber and file metadata from Prusa Connect, keeping the token-file format and the 10 s chamber cache. Verify with unit tests on recorded responses ported from `printers_test.go`.
- [ ] 17.2 Port files: list with signed `downloadUrl`, PUT upload, PATCH chunks (idempotent, 409 with `received`), start print, delete, the rename transfer queue, progress, and the signed download stream. Verify with unit tests for the chunk protocol and the rename state machine.
- [ ] 17.3 Port the camera: a supervised ffmpeg subprocess with a 5-minute idle-out, a single JPEG frame with a 20 s cold-start cap, `camera/url`, and the MJPEG `multipart/x-mixed-replace` stream. Verify with unit tests using a fake ffmpeg script.
- [ ] 17.4 Port recordings and auto-record: start and stop, list with signed URLs, bulk delete with `{deleted, failed}`, Range-capable media, size and duration caps, overlay and font options, and a poll-driven auto start and stop. Verify with unit tests and a Range test (206, `Content-Range`, 416).
- [ ] 17.5 Port the signed URL scheme byte for byte. Verify that a URL signed by the Go service validates in Python and vice versa.
- [ ] 17.6 Add the `prusa-login` CLI command. Verify by running it interactively against Prusa Connect and checking that the running API picks the session up.
- [ ] 17.7 Add `tests/parity/corpus/printers.yaml` with read-only routes only (status, files, files/meta, history, chamber, recordings, camera/url), telemetry and signatures marked volatile, run against the real printer. Verify that it is green.
- [ ] 17.8 Run the manual printer checklist on real hardware (`tests/parity/PRINTERS_CHECKLIST.md`): upload a small and a chunked file, rename, start a print and stop it, take a camera frame, open the MJPEG stream, confirm auto-record starts and stops, download a recording with seeking, and run a bulk delete. Verify that each item is ticked with its date.

## 18. H9 · Cutover

- [ ] 18.1 Run the full parity suite against a fresh production backup (`PARITY_SNAPSHOT`). Verify that it is green, with every deviation listed and justified in `ACCEPTED.md`, and that the route inventory is fully green.
- [ ] 18.2 Run `bench/run.sh "gv-api serve" py-<sha> prod 5y 5y_x3` and compare with `bench/results/go-d32ab42`. Run 3 times per app and use the median of each p95. Verify that every endpoint at `5y` meets p95 ≤ max(1.5 × Go, Go + 5 ms) and that peak RSS stays under 512 MB, and add the comparison table to `benchmarks.md`.
- [ ] 18.3 Switch the build: `Dockerfile.py` replaces `Dockerfile`, compose drops the D-Bus mount and the `LIGHTS_*` variables, the Makefile drops the `py-` prefixes and the Go targets, and `.env.example` is updated. Verify that `make up` and `make test` pass from a clean clone.
- [ ] 18.4 Update `.gitea/workflows/deploy.yml`: ruff, mypy and pytest with the coverage gate; a pre-deploy backup with `gv-api backup || ./main backup` that is fatal on this deploy; then build the image, run `alembic stamp 0001_baseline` through `docker compose run` only when `alembic current` is empty, and start the stack; the ECR push stays unchanged. Verify with a dry run on the runner against a copy of the production database.
- [ ] 18.5 Move `db/demo.sql` to `scripts/demo.sql` (and `make demo` with it), then delete `cmd/`, `internal/`, `test/`, `go.mod`, `go.sum`, `sqlc.yaml`, `.mockery.yaml`, the whole `db/` directory and `docs/api/lights.md` (keep `bench/`, whose load tool is its own Go module). Rewrite README and `docs/architecture.md` for Python. Verify that `rg -l '\.go\b|chi|sqlc|pgx|db/migrations'` finds no stale references outside history and `bench/load`.
- [ ] 18.6 Deploy to `main`. Verify that the container is healthy, the logs show the migration adoption line, gv-web works end to end (login with 2FA, habits, tasks, finance, printers), and a test ERROR reaches Telegram.
- [ ] 18.7 Rehearse rollback on the dev stack: redeploy `go-final` on the adopted DB, check that it is healthy, then return to Python. Verify by recording the steps in `docs/architecture.md` under Deployment.

## Workflow follow-up

- Remove the lights screens from gv-web and Android (separate repos).
- Later change: drop the `lights` table and `schema_migrations` once the rollback window is no longer needed.
- Archive this change after the deploy is verified.
