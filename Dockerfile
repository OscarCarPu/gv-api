# gv-api image. Alpine, so the Postgres 15 client matches the server: a newer pg_dump
# writes settings (transaction_timeout) that a 15 server rejects when the failover restores a dump.
FROM python:3.14-alpine3.22 AS builder

COPY --from=ghcr.io/astral-sh/uv:0.13.7 /uv /bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=never

WORKDIR /app
COPY pyproject.toml uv.lock README.md ./
RUN --mount=type=cache,target=/root/.cache/uv uv sync --locked --no-dev --no-install-project
COPY src ./src
RUN --mount=type=cache,target=/root/.cache/uv uv sync --locked --no-dev --no-editable

FROM python:3.14-alpine3.22

RUN apk add --no-cache postgresql15-client ffmpeg font-dejavu \
    && addgroup -S -g 1000 appgroup && adduser -S -u 1000 -G appgroup appuser \
    && mkdir -p /data/recordings /data/prusa && chown 1000:1000 /data/recordings /data/prusa

WORKDIR /app
COPY --from=builder /app/.venv /app/.venv
COPY alembic.ini ./
COPY migrations ./migrations
ENV PATH=/app/.venv/bin:$PATH PYTHONUNBUFFERED=1

USER appuser
EXPOSE 8080

CMD ["gv-api", "serve"]
