#!/usr/bin/env bash
# Seeds one database per scenario, starts the API against it and measures it.
#
#   bench/run.sh <api-command> <label> [scenario...]
#
# <api-command> starts the API in the foreground and reads its config from the environment
# from the repository root (for the Go release: a built binary; for Python: `gv-api serve`). Results go to
# bench/results/<label>/. Runs against the local `gv_db` container; databases are gv_bench_*.
# ONLY=<name,...> measures just those endpoints (the names in bench/load/main.go).
set -euo pipefail
export LC_ALL=C

here=$(cd "$(dirname "$0")" && pwd)
api_cmd=$1 label=$2; shift 2
scenarios=("${@:-prod 5y 5y_x3}")
read -r -a scenarios <<<"${scenarios[*]}"

set -a; . "$here/../.env"; set +a
# shellcheck source=bench/scenarios.sh
. "$here/scenarios.sh"

out="$here/results/$label"
work=$(mktemp -d)
mkdir -p "$out"
load="$work/load"
(cd "$here/load" && go build -o "$load" .)

psql_db() { docker exec -i gv_db psql -X -q -U "$POSTGRES_USER" -d "$1" "${@:2}"; }
port=18080
secret=bench-secret

# Template: the app's migrations (alembic upgrade head) plus golang-migrate's bookkeeping row,
# so the Go release (frozen at version 36) also starts on it. Built once.
if ! psql_db postgres -Atc "select 1 from pg_database where datname='gv_bench_template'" | grep -q 1; then
  echo ">>> building gv_bench_template with alembic upgrade head" >&2
  psql_db postgres -c "CREATE DATABASE gv_bench_template"
  DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:5432/gv_bench_template?sslmode=disable" \
    uv run --project "$here/.." alembic -c "$here/../alembic.ini" upgrade head
  psql_db gv_bench_template \
    -c "CREATE TABLE schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)" \
    -c "INSERT INTO schema_migrations VALUES (36, false)"
fi

for s in "${scenarios[@]}"; do
  db="gv_bench_$s"
  echo ">>> $s: seeding $db" >&2
  psql_db postgres -c "DROP DATABASE IF EXISTS $db" -c "CREATE DATABASE $db TEMPLATE gv_bench_template"
  "scenario_$s"
  hl_prob=$(python3 -c "print(min(1, $n_hl / ($n_habits * ($days + 1))))")
  vars=()
  for v in days n_accounts n_cat_roots n_cat_children n_tx n_budget_cats budget_months n_habits \
           n_projects n_proj_roots n_tasks n_open n_dep n_todos n_te n_pb future_days n_ev \
           ev_days_back n_marks n_out hl_prob; do
    vars+=(-v "$v=${!v}")
  done
  psql_db "$db" "${vars[@]}" <"$here/seed.sql"
  psql_db "$db" -At -F'|' >"$out/$s.tables.txt" <<'SQL'
select relname, n_live_tup, pg_total_relation_size(relid) from pg_stat_user_tables order by 1;
select '__db_bytes', 0, pg_database_size(current_database());
SQL

  mkdir -p "$work/$s"
  t0=$(date +%s%N)
  (cd "$here/.." && exec env -i PATH="$PATH" HOME="$HOME" \
    DATABASE_URL="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@127.0.0.1:5432/$db?sslmode=disable" \
    PORT=$port PASSWORD=bench-private SEMIPRIVATE_PASSWORD=bench-semi JWT_SECRET=$secret \
    TOTP_SECRET=JBSWY3DPEHPK3PXP ALLOWED_ORIGINS=http://localhost TIMEZONE=Europe/Madrid \
    FAILOVER_SIDE=home BACKUP_INTERVAL_MS=0 BACKUP_DIR="$work/$s" LIGHTS_DRIVER=mock \
    PRINTER_RECORDINGS_DIR="$work/$s" PRUSA_CONNECT_TOKEN_FILE="$work/$s/connect.json" \
    $api_cmd) >"$out/$s.api.log" 2>&1 &
  pid=$!
  until curl -sf "http://127.0.0.1:$port/health" >/dev/null; do
    kill -0 $pid 2>/dev/null || { echo "API died, see $out/$s.api.log" >&2; exit 1; }
    sleep 0.05
  done
  startup_ms=$(( ($(date +%s%N) - t0) / 1000000 ))
  rss_idle=$(awk '/VmRSS/{print $2}' /proc/$pid/status)
  cpu0=$(awk '{print $14+$15}' /proc/$pid/stat)

  "$load" -url "http://127.0.0.1:$port" -secret $secret -scenario "$s" -task "$n_tasks" -project 1 \
    ${ONLY:+-only "$ONLY"} \
    >"$out/$s.jsonl" 2> >(tee "$out/$s.txt" >&2)

  cpu1=$(awk '{print $14+$15}' /proc/$pid/stat)
  rss_end=$(awk '/VmRSS/{print $2}' /proc/$pid/status)
  rss_peak=$(awk '/VmHWM/{print $2}' /proc/$pid/status)
  kill $pid; wait $pid 2>/dev/null || true
  printf '{"scenario":"%s","startup_ms":%d,"rss_idle_kb":%d,"rss_end_kb":%d,"rss_peak_kb":%d,"cpu_seconds":%.2f}\n' \
    "$s" "$startup_ms" "$rss_idle" "$rss_end" "$rss_peak" "$(awk -v a="$cpu0" -v b="$cpu1" -v t="$(getconf CLK_TCK)" 'BEGIN{print (b-a)/t}')" \
    >"$out/$s.process.json"
done
rm -rf "$work"
echo ">>> results in $out" >&2
