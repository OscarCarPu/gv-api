BACKUP_DIR_OWNER=docker run --rm -v "$(CURDIR)/backups:/b" alpine:3.22 sh -c "mkdir -p /b/hourly /b/daily && chown 1000:1000 /b /b/hourly /b/daily"

.PHONY: setup-project run lint pgcli-db demo db-stamp check-rollback \
	up reset logs up-logs reset-logs down \
	failover-bucket failover-scenario failover-merge failover-status failover-outage failover-finish \
	test test-unit test-integration test-integration-db test-e2e \
	test-api-setup test-db-cleanup auth code prusa-login parity-up parity-down parity

# Colors
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[0;33m
CYAN=\033[0;36m
NC=\033[0m

# --- PROJECT SETUP ---
setup-project:
	@cp -n .env.example .env || true
	@command -v uv >/dev/null 2>&1 || { printf "$(RED)uv is not installed$(NC)\n" >&2; exit 1; }
	uv sync

# --- RUN & CHECK ---

run:
	uv run gv-api serve

lint:
	uv run ruff check
	uv run ruff format --check
	uv run mypy

# --- DATABASE ---

pgcli-db:
	pgcli postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)

demo:
	@printf "$(CYAN)>>> Loading demo data...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) < scripts/demo.sql
	@printf "$(GREEN)>>> Demo data loaded$(NC)\n"

# Marks a database the previous release migrated (golang-migrate, version 36) as being at the
# Alembic baseline. Only when it has no Alembic history yet. DB_URL defaults to the development
# database.
DB_URL ?= postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@127.0.0.1:5432/$(POSTGRES_DB)?sslmode=disable
db-stamp:
	@if [ -n "$$(DATABASE_URL='$(DB_URL)' uv run alembic current 2>/dev/null)" ]; then \
		echo "already stamped"; \
	else \
		DATABASE_URL='$(DB_URL)' uv run alembic stamp 0001_baseline && echo "stamped at 0001_baseline"; \
	fi

# Previous release -> stamp -> this app -> previous release again, on a throwaway database.
check-rollback:
	./scripts/check_rollback.sh

# --- DOCKER OPERATIONS ---

# Local runs also start a fake AWS (SSM + S3) for the failover, see docs/dev-failover.md.
# `make up DEV_AWS=0` leaves it out. The deploy workflow calls docker compose directly and never adds it.
DEV_AWS ?= 1
COMPOSE = docker compose $(if $(filter 1,$(DEV_AWS)),-f docker-compose.yaml -f docker-compose.dev-aws.yaml)

up:
	@$(BACKUP_DIR_OWNER)
	$(COMPOSE) --progress plain up --build --wait -d
	@$(MAKE) --no-print-directory failover-bucket

reset:
	@$(BACKUP_DIR_OWNER)
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) --progress plain up --build --wait -d
	@$(MAKE) --no-print-directory failover-bucket

logs:
	$(COMPOSE) logs -f

up-logs:
	make up
	make logs

reset-logs:
	make reset
	make logs

down:
	$(COMPOSE) down

# --- FAILOVER (local, against the fake AWS) ---

failover-bucket:
	@if [ "$(DEV_AWS)" = 1 ]; then ./scripts/failover-dev.sh bucket; fi

failover-scenario:
	./scripts/failover-dev.sh scenario

failover-merge:
	./scripts/failover-dev.sh merge

failover-status:
	./scripts/failover-dev.sh status

failover-outage:
	./scripts/failover-dev.sh outage

failover-finish:
	./scripts/failover-dev.sh finish

# --- TESTING ---

include .env

# The e2e suite runs against the compose API on $(TEST_DB); the integration suite has its own.
E2E_DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@127.0.0.1:5432/$(TEST_DB)?sslmode=disable
INTEGRATION_DB=$(TEST_DB)-integration
INTEGRATION_DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@127.0.0.1:5432/$(INTEGRATION_DB)?sslmode=disable

test-db-cleanup:
	@printf "$(YELLOW)>>> Cleaning up test database...$(NC)\n"
	@docker compose stop gv-api > /dev/null 2>&1 || true
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "DROP DATABASE IF EXISTS \"$(TEST_DB)\";" > /dev/null 2>&1
	@docker compose up -d --wait gv-api > /dev/null 2>&1 || true
	@printf "$(GREEN)>>> Cleanup complete$(NC)\n"

test-api-setup:
	@$(BACKUP_DIR_OWNER)
	@printf "$(CYAN)>>> Starting database...$(NC)\n"
	@docker compose stop gv-api > /dev/null 2>&1 || true
	@docker compose up -d --wait db > /dev/null
	@printf "$(YELLOW)>>> Dropping test database if exists...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "DROP DATABASE IF EXISTS \"$(TEST_DB)\";" > /dev/null 2>&1
	@printf "$(YELLOW)>>> Creating test database...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "CREATE DATABASE \"$(TEST_DB)\";" > /dev/null
	@printf "$(CYAN)>>> Rebuilding and restarting API with test database...$(NC)\n"
	@docker compose --progress plain -f docker-compose.yaml -f docker-compose.test.yaml up -d --wait --build gv-api > /dev/null
	@printf "$(GREEN)>>> API ready$(NC)\n"

# Unit tests: fast, no external dependencies
test-unit:
	uv run pytest tests/unit

# Fresh database for the integration tests.
test-integration-db:
	@docker compose up -d --wait db > /dev/null
	@docker compose exec -T db psql -q -U $(POSTGRES_USER) -d postgres -c 'DROP DATABASE IF EXISTS "$(INTEGRATION_DB)"' -c 'CREATE DATABASE "$(INTEGRATION_DB)"'

# Integration tests: repositories and migrations against a real database
test-integration: test-integration-db
	TEST_DB_URL='$(INTEGRATION_DB_URL)' uv run pytest tests/integration

# E2E tests: HTTP against the compose API, rebuilt on a fresh test database
test-e2e: test-api-setup
	@API_URL=http://127.0.0.1:$(PORT) TEST_DB_URL='$(E2E_DB_URL)' PASSWORD='$(PASSWORD)' TOTP_SECRET='$(TOTP_SECRET)' \
		uv run pytest tests/e2e --no-cov || { $(MAKE) test-db-cleanup --no-print-directory; exit 1; }
	@$(MAKE) test-db-cleanup --no-print-directory

# Unit + integration with the coverage gate (85% overall, 90% per service.py), then e2e.
test: test-integration-db
	TEST_DB_URL='$(INTEGRATION_DB_URL)' uv run pytest tests/unit tests/integration --cov --cov-report=term --cov-report=json:coverage.json
	uv run python scripts/check_coverage.py
	@$(MAKE) --no-print-directory test-e2e
	@printf "$(GREEN)>>> All tests passed$(NC)\n"

# Authenticate and print a JWT token for manual testing
auth:
	@python scripts/auth.py

# Log in to Prusa Connect from the running container; asks for the code Prusa emails
prusa-login:
	docker compose exec gv-api gv-api prusa-login

# Print just the current TOTP 2FA code
code:
	@python scripts/auth.py --code

# --- PARITY (design D15): the previous release (tag go-final) and this app side by side ---
# PARITY_SNAPSHOT=backups/daily/<dump>.sql.gz replays on production data instead of the demo seed.
# CORPUS="auth http" runs only those files of tests/parity/corpus.

parity-up:
	tests/parity/stack.sh up

parity-down:
	tests/parity/stack.sh down

parity: parity-up
	uv run python -m tests.parity.runner $(CORPUS)
