up:
	docker compose up -d

down:
	docker compose down

clean:
	docker compose down -v

rebuild:
	docker compose up -d --build

prod-up:
	STAGE=prod docker compose up -d

prod-down:
	STAGE=prod docker compose down

prod-clean:
	STAGE=prod docker compose down -v

prod-rebuild:
	STAGE=prod docker compose up -d --build
