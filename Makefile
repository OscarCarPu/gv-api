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
	$(eval LAST_ID := $(shell ls alembic/versions/[0-8]*.py | sort | tail -n 1 | xargs basename | cut -c 1-3))
	$(eval NEXT_ID := $(shell printf "%03d" $$(($(LAST_ID) + 1))))
	$(if $(m),,$(error Please provide a message, like this make generate-migration m="Add a new column"))
	uv run alembic revision --autogenerate -m "$(m)" --rev-id=$(NEXT_ID) --head=default

execute-migrations:
	uv run alembic upgrade head

local-reset:
	docker compose down -v
	docker compose up -d --build db --wait
	uv run alembic upgrade default@head
	uv run alembic upgrade data_seed@head
	docker compose up -d --wait --build

reset:
	docker compose up -d --build --wait

server-reset:
	docker compose down
	docker compose up -d --build --wait
	uv run alembic upgrade default@head
	docker system prune -f

deploy:
	git checkout main
	git merge develop
	git push
	git checkout develop
