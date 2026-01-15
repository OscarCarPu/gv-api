up:
	if ! docker compose ps -q | grep -q ^; then \
		docker compose up -d --wait; \
	fi

down:
	docker compose down

clean:
	docker compose down -v

logs:
	docker compose logs -f

test:
	uv run pytest -vv

generate-migration:
	uv run alembic revision --autogenerate -m "$(m)" --rev-id="$(r)" --head=default

execute-migrations:
	uv run alembic upgrade head

db-reset:
	docker compose down -v && docker compose up -d --build db --wait && uv run alembic upgrade head && docker compose up -d --wait --build

reset:
	docker compose up -d --build --wait

server-reset:
	docker compose down
	docker compose up -d --build --wait
	uv run alembic upgrade head
	docker system prune -f

deploy:
	git checkout main
	git merge develop
	git push
	git checkout develop
