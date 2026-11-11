#!/usr/bin/env bash
# The parity stack (docker-compose.parity.yaml).
#
#   tests/parity/stack.sh up     build both images, seed a fresh template, clone it once per app
#                                and start both apps (healthy). Run again to reset every database.
#   tests/parity/stack.sh down
#
# The template is the demo seed on the migrated schema, or PARITY_SNAPSHOT=<backup .sql[.gz]>
# (a production dump from ./backups) for real data shapes.
set -euo pipefail

repo=$(cd "$(dirname "$0")/../.." && pwd)
compose=(docker compose -f "$repo/docker-compose.parity.yaml")
go_image=gv-api-go:go-final
template_url=postgresql://parity:parity@127.0.0.1:15432/parity_template?sslmode=disable

psql() { "${compose[@]}" exec -T db psql -X -q -v ON_ERROR_STOP=1 -U parity "$@"; }
alembic() { DATABASE_URL=$template_url uv run --project "$repo" alembic -c "$repo/alembic.ini" "$@"; }

seed_template() {
  psql -d postgres -c "DROP DATABASE IF EXISTS parity_template WITH (FORCE)" \
    -c "CREATE DATABASE parity_template"
  if [ -n "${PARITY_SNAPSHOT:-}" ]; then
    echo ">>> template: snapshot $PARITY_SNAPSHOT" >&2
    case $PARITY_SNAPSHOT in
      *.gz) gunzip -c "$PARITY_SNAPSHOT" ;;
      *) cat "$PARITY_SNAPSHOT" ;;
    esac | psql -d parity_template >/dev/null
    # A Go-era dump has no Alembic history: the same one-time stamp as the cutover.
    if [ -z "$(alembic current 2>/dev/null)" ]; then alembic stamp 0001_baseline >/dev/null 2>&1; fi
  else
    local demo=$repo/scripts/demo.sql
    [ -f "$demo" ] || demo=$repo/db/demo.sql
    echo ">>> template: migrations + $(basename "$(dirname "$demo")")/demo.sql" >&2
    alembic upgrade head >/dev/null 2>&1
    # golang-migrate's bookkeeping, so the Go release (frozen at 36) starts with nothing pending.
    psql -d parity_template \
      -c "CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)" \
      -c "INSERT INTO schema_migrations VALUES (36, false)"
    psql -d parity_template >/dev/null <"$demo"
  fi
  # The corpora open their own timer; a running one (the demo leaves one depending on the hour)
  # would make that a conflict.
  psql -d parity_template -c "UPDATE time_entries SET finished_at = now() WHERE finished_at IS NULL"
}

up() {
  if ! docker image inspect "$go_image" >/dev/null 2>&1; then
    echo ">>> building $go_image from git tag go-final" >&2
    git -C "$repo" archive go-final | docker build -q -t "$go_image" - >/dev/null
  fi
  "${compose[@]}" build -q py
  "${compose[@]}" up -d --wait db
  "${compose[@]}" stop go py >/dev/null 2>&1 || true
  seed_template
  for db in parity_go parity_py; do
    psql -d postgres -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" \
      -c "CREATE DATABASE $db TEMPLATE parity_template"
  done
  "${compose[@]}" up -d --wait go py
  echo ">>> parity stack up: go http://127.0.0.1:18180  py http://127.0.0.1:18181" >&2
}

case ${1:-} in
  up) up ;;
  down) "${compose[@]}" down -v ;;
  *) echo "usage: $0 up|down" >&2; exit 2 ;;
esac
