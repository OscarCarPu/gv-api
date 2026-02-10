.PHONY: sqlc run build docker-build up setup-project

# Colors
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[0;33m
CYAN=\033[0;36m
NC=\033[0m

# --- PROJECT SETUP ---
setup-project:
	@cp -n .env.example .env || true
	@command -v go >/dev/null 2>&1 || { printf "$(RED)go is not installed$(NC)\n" >&2; exit 1; }
	@command -v sqlc >/dev/null 2>&1 || { printf "$(RED)sqlc is not installed$(NC)\n" >&2; exit 1; }

# --- CODE GENERATION ---

# Run sqlc
sqlc:
	sqlc generate

# Create a new migration file
# Usage: make migration name=add_users
migration:
	@read -p "Enter migration name: " name; \
	touch db/migrations/$(shell date +%Y%m%d%H%M%S)_$$name.sql

# --- DOCKER OPERATIONS ---

# start the project, building it
up:
	docker compose up --build --wait -d

# reset the project
reset:
	docker compose down -v --remove-orphans
	docker compose up --build --wait -d

# follow logs
logs:
	docker compose logs -f

# up and logs
up-logs:
	make up
	make logs

# reset and logs
reset-logs:
	make reset
	make logs

# --- TESTING ---

include .env

INNER_TEST_DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@db:5432/$(TEST_DB)?sslmode=disable
OUTSIDE_TEST_DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(TEST_DB)?sslmode=disable

test-db-setup:
	@printf "$(CYAN)>>> Starting database...$(NC)\n"
	@docker compose up -d --wait db > /dev/null
	@printf "$(YELLOW)>>> Dropping test database if exists...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "DROP DATABASE IF EXISTS \"$(TEST_DB)\";" > /dev/null 2>&1
	@printf "$(YELLOW)>>> Creating test database...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "CREATE DATABASE \"$(TEST_DB)\";" > /dev/null
	@printf "$(YELLOW)>>> Running migrations...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d $(TEST_DB) -f /docker-entrypoint-initdb.d/001_habits.sql > /dev/null
	@printf "$(GREEN)>>> Test database ready$(NC)\n"

test-db-cleanup:
	@printf "$(YELLOW)>>> Cleaning up test database...$(NC)\n"
	@docker compose exec -T db psql -U $(POSTGRES_USER) -d postgres -c "DROP DATABASE IF EXISTS \"$(TEST_DB)\";" > /dev/null 2>&1
	@printf "$(GREEN)>>> Cleanup complete$(NC)\n"

test-api-setup: test-db-setup
	@printf "$(CYAN)>>> Rebuilding and restarting API with test database...$(NC)\n"
	@docker compose stop api > /dev/null 2>&1 || true
	@DATABASE_URL=$(INNER_TEST_DB_URL) docker compose up -d --wait api > /dev/null
	@printf "$(GREEN)>>> API ready$(NC)\n"

# All tests: silent, only prints pass/fail
test-silent: test-api-setup
	@printf "$(CYAN)>>> Running all tests...$(NC)\n"
	@go test -short ./internal/... > /dev/null 2>&1 || { printf "$(RED)>>> Unit tests failed$(NC)\n"; $(MAKE) test-db-cleanup --no-print-directory; exit 1; }
	@TEST_DB_URL=$(OUTSIDE_TEST_DB_URL) go test -run Integration ./internal/... > /dev/null 2>&1 || { printf "$(RED)>>> Integration tests failed$(NC)\n"; $(MAKE) test-db-cleanup --no-print-directory; exit 1; }
	@TEST_DB_URL=$(OUTSIDE_TEST_DB_URL) PORT=$(PORT) go test ./test/e2e/... > /dev/null 2>&1 || { printf "$(RED)>>> E2E tests failed$(NC)\n"; $(MAKE) test-db-cleanup --no-print-directory; exit 1; }
	@$(MAKE) test-db-cleanup --no-print-directory
	@printf "$(GREEN)>>> All tests passed$(NC)\n"

# Unit tests: fast, no external dependencies
test-unit:
	@printf "$(CYAN)>>> Running unit tests...$(NC)\n"
	@go test -v -short ./internal/...

# Integration tests: require a running database
test-integration: test-db-setup
	@printf "$(CYAN)>>> Running integration tests...$(NC)\n"
	@TEST_DB_URL=$(OUTSIDE_TEST_DB_URL) go test -v -run Integration ./internal/...
	@$(MAKE) test-db-cleanup --no-print-directory

# E2E tests: require full stack (API + database)
test-e2e: test-api-setup
	@printf "$(CYAN)>>> Running e2e tests...$(NC)\n"
	@TEST_DB_URL=$(OUTSIDE_TEST_DB_URL) PORT=$(PORT) go test ./test/e2e/... -v
	@$(MAKE) test-db-cleanup --no-print-directory

# Coverage: run all tests and generate coverage report
test-coverage: test-api-setup
	@printf "$(CYAN)>>> Running all tests with coverage...$(NC)\n"
	@TEST_DB_URL=$(OUTSIDE_TEST_DB_URL) PORT=$(PORT) go test -v $$(go list ./... | grep -v /internal/database/sqlc) -coverprofile=coverage.out
	@$(MAKE) test-db-cleanup --no-print-directory
	@printf "$(YELLOW)>>> Updating README with coverage table...$(NC)\n"
	@uv run scripts/coverage.py
	@printf "$(GREEN)>>> Coverage report updated$(NC)\n"

# Run all tests
test: test-unit test-integration test-e2e
	@printf "$(GREEN)>>> All tests passed$(NC)\n"
