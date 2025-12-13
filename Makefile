up:
	docker compose up -d

down:
	docker compose down

clean:
	docker compose down -v

rebuild:
	docker compose up -d --build

logs:
	docker compose logs -f

prod-up:
	STAGE=prod docker compose up -d

prod-down:
	STAGE=prod docker compose down

prod-clean:
	STAGE=prod docker compose down -v

prod-rebuild:
	STAGE=prod docker compose up -d --build

test:
	uv run pytest

generate-migration:
	uv run alembic revision --autogenerate -m "$(m)"

execute-migrations:
	uv run alembic upgrade head

db-reset:
	docker compose down -v && docker compose up -d db && sleep 3 && uv run alembic upgrade head
