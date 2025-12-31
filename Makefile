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
	uv run alembic revision --autogenerate -m "$(m)"

execute-migrations:
	uv run alembic upgrade head

db-reset:
	docker compose down -v && docker compose up -d db --wait && uv run alembic upgrade head && docker compose up -d --wait

ollama-start:
	OLLAMA_HOST=0.0.0.0 ollama serve & sleep 2 && ollama run qwen3-fast

ollama-stop:
	pkill ollama || true

ollama-create-model:
	ollama pull qwen3:0.6b && ollama create qwen3-fast -f ollama/qwen3-fast.modelfile
