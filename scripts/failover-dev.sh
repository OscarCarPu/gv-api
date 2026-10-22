#!/bin/bash
# Rehearses an AWS failover on this machine, against the fake SSM and S3 of docker-compose.dev-aws.yaml.
#   scenario   build a divergence from the current dev database and stage it in the fake AWS
#   merge      play the Lambda: hold the app, then ask it to restore
#   status     show what the fake AWS and gv-api say
#   outage     play the Lambda's last step: record the failover in the uptime history
#   finish     back to NORMAL, as when the failover is over
#   bucket     create the dump bucket (make up does it)
set -euo pipefail
cd "$(dirname "$0")/.."

env_value() { grep -E "^$1=" .env | head -n 1 | cut -d= -f2-; }
POSTGRES_USER=${POSTGRES_USER:-$(env_value POSTGRES_USER)}
POSTGRES_DB=${POSTGRES_DB:-$(env_value POSTGRES_DB)}

AWS_ENDPOINT=${AWS_ENDPOINT:-http://localhost:5000}
API=${API:-http://localhost:8080}
TOKEN=${FAILOVER_TOKEN:-dev-token}
BUCKET=${BUCKET:-gv-dev}
DBX=${DBX:-docker compose exec -T db}
export AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_DEFAULT_REGION=eu-south-2

aws_() { aws --endpoint-url "$AWS_ENDPOINT" "$@"; }
psql_() { $DBX psql -q -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$@"; }
dump_() { $DBX pg_dump -U "$POSTGRES_USER" --clean --if-exists --no-owner --no-privileges "$@"; }
param() { aws_ ssm put-parameter --name "/home-lab-failover/$1" --type String --overwrite --value "$2" > /dev/null; }
forget() { aws_ ssm delete-parameter --name "/home-lab-failover/$1" > /dev/null 2>&1 || true; }
state() { param state "{\"state\":\"$1\"}"; }
push() {
  curl -fsS -X POST "$API/failover" -H "X-Failover-Token: $TOKEN" -H 'Content-Type: application/json' -d "{\"mode\":\"$1\"}"
  echo
}
stamp() { date -u -d "$1" +%Y%m%dT%H%M%SZ; }

bucket() {
  curl -sS -X PUT "$AWS_ENDPOINT/$BUCKET" > /dev/null || echo "the fake AWS is not answering at $AWS_ENDPOINT" >&2
}

scenario() {
  if [ "${YES:-}" != 1 ]; then
    read -r -p "This adds rows to the dev database '$POSTGRES_DB' and swaps it on restore. Continue? [y/N] " yn
    [ "$yn" = y ] || exit 1
  fi
  bucket
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT

  psql_ -d "$POSTGRES_DB" -c "INSERT INTO tasks (name) SELECT 'Dev task' WHERE NOT EXISTS (SELECT 1 FROM tasks)"
  dump_ -d "$POSTGRES_DB" | gzip > "$work/base.sql.gz"

  psql_ -d postgres -c 'DROP DATABASE IF EXISTS gv_dev_ec2' -c 'CREATE DATABASE gv_dev_ec2'
  gunzip -c "$work/base.sql.gz" | psql_ -d gv_dev_ec2 > /dev/null
  psql_ -d gv_dev_ec2 > /dev/null <<'SQL'
INSERT INTO tasks (name) VALUES ('Created on the backup server');
INSERT INTO habits (name) VALUES ('Habit from the backup server');
UPDATE tasks SET description = 'edited on the backup server' WHERE id = (SELECT min(id) FROM tasks);
SQL
  dump_ -d gv_dev_ec2 | gzip > "$work/ec2.sql.gz"
  psql_ -d postgres -c 'DROP DATABASE gv_dev_ec2'

  psql_ -d "$POSTGRES_DB" <<'SQL'
INSERT INTO tasks (name) VALUES ('Created at home during the outage');
INSERT INTO habits (name) VALUES ('Habit from home');
UPDATE tasks SET description = 'edited at home' WHERE id = (SELECT min(id) FROM tasks);
SQL

  base_key="gv-db/hourly/gv-db-$(stamp '-2 hours').sql.gz"
  dump_key="gv-db/hourly/gv-db-$(stamp 'now').sql.gz"
  aws_ s3 cp "$work/base.sql.gz" "s3://$BUCKET/$base_key" > /dev/null
  aws_ s3 cp "$work/ec2.sql.gz" "s3://$BUCKET/$dump_key" > /dev/null

  forget dump
  param base "$base_key"
  param dump-staged "$dump_key"
  state FAILED_OVER
  echo "staged: the app at home and the backup server both changed the first task (it shows up in the report). Next: make failover-merge"
}

merge() {
  dump_key=$(aws_ ssm get-parameter --name /home-lab-failover/dump-staged --query Parameter.Value --output text 2> /dev/null) || {
    echo "run make failover-scenario first" >&2
    exit 1
  }
  push hold
  param dump "$dump_key"
  state FAILING_BACK
  push merge
  echo "merging. Open the web app: it should land on /report. Watch: make failover-status"
}

status() {
  echo "state: $(aws_ ssm get-parameter --name /home-lab-failover/state --query Parameter.Value --output text 2> /dev/null || echo '(none, counts as NORMAL)')"
  echo -n "gv-api: "
  curl -sS "$API/failover/status" -H "X-Failover-Token: $TOKEN"
  echo
}

outage() {
  state HOME_READY
  home_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  ec2_up_at=$(date -u -d '-1 minute' +%Y-%m-%dT%H:%M:%SZ)
  curl -fsS -X POST "$API/failover/outage" -H "X-Failover-Token: $TOKEN" -H 'Content-Type: application/json' \
    -d "{\"ec2_up_at\":\"$ec2_up_at\",\"home_at\":\"$home_at\"}"
  echo "recorded"
}

finish() {
  state NORMAL
  forget base
  forget dump
  forget dump-staged
  echo "state is NORMAL again. gv-api leaves the hold by itself once its restore is done; if you stopped halfway: docker compose restart gv-api"
}

case "${1:-}" in
  scenario | merge | status | outage | finish | bucket) "$1" ;;
  *)
    sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//' >&2
    exit 1
    ;;
esac
