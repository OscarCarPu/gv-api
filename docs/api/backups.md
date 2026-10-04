# Backups

Database dumps made by gv-api itself: listing them, taking one on demand, and downloading one. **Full auth**.

## How backups are made

A `pg_dump` of the API's database, gzipped plain SQL (`--clean --if-exists --no-owner --no-privileges`), taken:

- every `BACKUP_INTERVAL_MS` (default 1 h, at the top of the hour; `0` turns the schedule off);
- on `POST /backups`;
- from `./main backup` in the container, which the deploy workflow runs before every deploy.

Only one backup runs at a time, even across processes: a Postgres advisory lock (`hashtext('backup')`) guards each run. A run that finds the lock taken gets `409` (or exit code 1 from the command).

Files are named `gv-db-<YYYYMMDD>T<HHMMSS>Z.sql.gz`, in UTC. A run that lands in the same second as the previous one returns that backup instead of overwriting it.

## Storage and retention

```
/backups                 BACKUP_DIR, bind-mounted from ./backups
├── hourly/              every backup
└── daily/               first backup of each UTC day (a hard link, no extra space)
```

After each successful run:

| Folder | Deleted when older than | Default |
|---|---|---|
| `hourly/` | `BACKUP_KEEP_HOURLY_DAYS` | 2 days |
| `daily/` | `BACKUP_KEEP_DAILY_DAYS` | 30 days |

The newest file in each folder is always kept. Files whose names don't match the pattern, such as the old sidecar dumps, are never listed or deleted. A failed dump deletes nothing.

The folders must be owned by uid 1000, which gv-api runs as. `make up`, `make reset` and the deploy workflow take care of that.

## Endpoints

### `GET /backups`

Every backup in `hourly/` and `daily/`, newest first. A daily backup that is also still in `hourly/` appears once.

```json
[
  { "name": "gv-db-20261004T130000Z.sql.gz", "size": 38511, "created_at": "2026-10-04T13:00:00Z" },
  { "name": "gv-db-20261004T120000Z.sql.gz", "size": 38507, "created_at": "2026-10-04T12:00:00Z" }
]
```

| Field | Meaning |
|---|---|
| `name` | File name, used by the download endpoint. |
| `size` | Bytes, compressed. |
| `created_at` | When the dump started, UTC. |

The age of the first entry is the health signal: older than the interval means scheduled runs are failing. The reason is in the API logs (`backup: scheduled backup failed`).

### `POST /backups`

Takes a backup now and answers once it's done (about a second today).

| Status | When |
|---|---|
| `201` | The new backup, same shape as a list item. |
| `409` | Another backup is running. |
| `500` | The dump failed. Details are in the API logs. |

### `GET /backups/{name}`

Downloads one backup, looking in `hourly/` first, then `daily/`.

| Status | When |
|---|---|
| `200` | The file, with `Content-Type: application/gzip` and `Content-Disposition: attachment; filename="<name>"`. |
| `400` | `name` doesn't match `gv-db-<YYYYMMDD>T<HHMMSS>Z.sql.gz`. This also rejects paths like `../.env`. |
| `404` | No backup with that name. |

## Restoring

The dump drops and recreates every object, so it restores into an existing database:

```bash
gunzip -c backups/hourly/<name> | docker compose exec -T db psql -U <user> -d <database>
```

Try it on a scratch database first. On the server, a restore is a manual decision, not something the API does.

## Off-site copy

With `BACKUP_S3_BUCKET` set, each backup is also uploaded to `gv-db/hourly/`, and the day's first to `gv-db/daily/`. The bucket and its upload-only key live in `aws-failover`. A failed upload is only logged.
