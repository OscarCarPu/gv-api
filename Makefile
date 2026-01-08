up:
	docker compose up -d --wait

down:
	docker compose down

clean:
	docker compose down -v

logs:
	docker compose logs -f

test:
	uv run pytest

generate-migration:
	uv run alembic revision --autogenerate -m "$(m)" --rev-id="$(r)"

execute-migrations:
	uv run alembic upgrade head

db-reset:
	docker compose down -v && docker compose up -d --build db --wait && uv run alembic upgrade head && docker compose up -d --wait --build

reset:
	docker compose up -d --build --wait
