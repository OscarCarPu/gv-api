.PHONY: sqlc run build docker-build up setup-project

# --- PROJECT SETUP ---
setup-project:
	@cp -n .env.example .env || true
	@command -v sqlc >/dev/null 2>&1 || { echo >&2 "sqlc is not installed."; exit 1; }

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
