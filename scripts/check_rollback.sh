#!/usr/bin/env bash
# Rehearses the cutover and its rollback on a throwaway database in the local gv_db container:
#   Go release migrates it -> one-time stamp -> Python app starts (nothing pending)
#   -> stamp again (no-op) -> Go release starts again on the same database.
# The Go release is built from git history, so this keeps working after cmd/ is deleted.
#
#   scripts/check_rollback.sh [git-ref]      (default: go-final, or d32ab42 before the tag)
set -euo pipefail

repo=$(cd "$(dirname "$0")/.." && pwd)
ref=${1:-$(git -C "$repo" rev-parse -q --verify go-final >/dev/null && echo go-final || echo d32ab42)}
set -a; . "$repo/.env"; set +a
db=gv_rollback_check
url="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:5432/$db?sslmode=disable"
work=$(mktemp -d)
psql_admin() { docker exec -i gv_db psql -X -q -U "$POSTGRES_USER" -d postgres "$@"; }
cleanup() { jobs -p | xargs -r kill 2>/dev/null || true; psql_admin -c "DROP DATABASE IF EXISTS $db WITH (FORCE)"; rm -rf "$work"; }
trap cleanup EXIT

app_env=(env -i PATH="$PATH" HOME="$HOME" DATABASE_URL="$url" PASSWORD=a SEMIPRIVATE_PASSWORD=b
  JWT_SECRET=c TOTP_SECRET=JBSWY3DPEHPK3PXP ALLOWED_ORIGINS=http://localhost BACKUP_INTERVAL_MS=0
  BACKUP_DIR="$work" LIGHTS_DRIVER=mock PRINTER_RECORDINGS_DIR="$work")

# start_healthy <label> <port> <dir> <cmd...>: runs until /health answers 200, then stops it.
start_healthy() {
  local label=$1 port=$2 dir=$3; shift 3
  (cd "$dir" && exec "${app_env[@]}" PORT="$port" "$@") >"$work/$label.log" 2>&1 &
  local pid=$!
  for _ in $(seq 150); do
    if curl -sf -o /dev/null -m 1 "http://127.0.0.1:$port/health"; then
      kill "$pid"; wait "$pid" 2>/dev/null || true
      echo "ok: $label healthy"; return 0
    fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  echo "FAIL: $label did not become healthy:"; tail -20 "$work/$label.log"; return 1
}

revision() { DATABASE_URL="$url" uv run --project "$repo" alembic -c "$repo/alembic.ini" current 2>/dev/null; }

echo ">>> building the Go release at $ref"
git -C "$repo" archive "$ref" | tar -x -C "$work"
(cd "$work" && go build -o gv-api-go ./cmd/api)

psql_admin -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" -c "CREATE DATABASE $db"
start_healthy go-first 18097 "$work" ./gv-api-go

DB_URL="$url" make -s -C "$repo" db-stamp
[ "$(revision)" = "0001_baseline (head)" ] || { echo "FAIL: not stamped: '$(revision)'"; exit 1; }
DB_URL="$url" make -s -C "$repo" db-stamp | grep -q "already stamped" || { echo "FAIL: second stamp was not a no-op"; exit 1; }
echo "ok: stamped once, second stamp skipped"

start_healthy python 18098 "$repo" "$repo/.venv/bin/gv-api" serve
[ "$(revision)" = "0001_baseline (head)" ] || { echo "FAIL: python changed the revision"; exit 1; }

start_healthy go-again 18097 "$work" ./gv-api-go
echo ">>> rollback window holds"
