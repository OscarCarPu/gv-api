# Uptime (Domotics)

How much of the time the home lab and its ESP32 watchdog have been reachable. **Semiprivate auth**.

## Where the numbers come from

Each device publishes up/down events to MQTT (`events/uptime/lab`, `events/uptime/watchdog`); [central-pipeline][cp] models them with dbt into two marts in its own PostgreSQL:

```
device → MQTT → central-pipeline (raw → staging → marts) → gv-api → client
```

gv-api reads them over a separate read-only connection (`PIPELINE_DATABASE_URL`, `internal/pipeline`). dbt owns that schema and recreates it every run, so nothing is migrated or built on top of it.

With no DSN configured both endpoints answer **503**; nothing else is affected.

[cp]: https://github.com/OscarCarPu/central-pipeline

## What the numbers are not

- **Not live.** `computed_at` is when dbt last ran. Older than `PIPELINE_STALE_AFTER_MS` (2h) means `stale: true`.
- **Not a heartbeat.** Events are edge-triggered: an old `since` means nothing changed. The open window counts as up until `computed_at`, so a device that dies without publishing `down` reads as up until its peer reports it.
- **Floored at the first event.** A young device reports its real history; read `range_start`.

## Endpoints

### `GET /domotics/uptime`

Both devices' current state plus the four precomputed percentages for each.

Both devices always appear (`lab`, `watchdog`), ranges shortest first. A device never heard from has `state: "unknown"`, `since: null` and no ranges.

```json
{
  "computed_at": "2026-08-20T18:07:44.680255+02:00",
  "stale": false,
  "stale_after_seconds": 7200,
  "devices": [
    {
      "device": "lab",
      "state": "up",
      "since": "2026-08-13T16:54:25+02:00",
      "ranges": [
        {
          "range": "month",
          "uptime": 98.92,
          "range_start": "2026-07-20T18:07:44.680255+02:00",
          "range_end": "2026-08-20T18:07:44.680255+02:00"
        },
        { "range": "3 months", "uptime": 98.28, "range_start": "…", "range_end": "…" },
        { "range": "year", "uptime": 98.05, "range_start": "…", "range_end": "…" },
        { "range": "all", "uptime": 97.95, "range_start": "2025-06-30T01:54:25+02:00", "range_end": "…" }
      ]
    },
    { "device": "watchdog", "state": "up", "since": "…", "ranges": [] }
  ]
}
```

| Field | Meaning |
|---|---|
| `computed_at` | The dbt run time — the newest `range_end` present. `null` when nothing has been computed yet. |
| `stale` | `computed_at` is older than `stale_after_seconds`, or absent. |
| `state` | `up`, `down`, or `unknown` when the pipeline has no window for the device. |
| `since` | Start of the open window: when the device entered this state, and the last thing heard about it. |
| `uptime` | Percentage, 0-100, two decimals. |
| `range_start` | Start of the interval the percentage covers, floored at the device's first event. |

Only those four lookbacks are served. Anything else goes to `/windows`.

### `GET /domotics/uptime/windows`

State changes over an arbitrary range, with the percentage computed for exactly that range. For timelines and incident lists.

| Query | Default | Meaning |
|---|---|---|
| `device` | both | `lab` or `watchdog`; anything else is a 400. |
| `from` | `to` - 30 days | RFC 3339 or `YYYY-MM-DD` (read as UTC midnight). |
| `to` | now | Same formats. Clamped to now: the open window counts up to `to`, so a future `to` would invent uptime. |
| `limit` | 1000 | Windows returned, newest first, capped at 5000. The percentages ignore it. |

```json
{
  "from": "2026-08-01T00:00:00Z",
  "to": "2026-08-10T00:00:00Z",
  "devices": [
    {
      "device": "watchdog",
      "uptime": 98.61,
      "up_seconds": 766800,
      "down_seconds": 10800,
      "outages": 3,
      "covered_from": "2026-08-01T02:00:00+02:00",
      "covered_to": "2026-08-10T02:00:00+02:00",
      "windows": [
        {
          "state": "up",
          "start_time": "2026-08-06T04:54:25+02:00",
          "end_time": "2026-08-11T08:54:25+02:00",
          "seconds": 335135
        }
      ],
      "truncated": true
    }
  ]
}
```

- `uptime` divides by `up_seconds + down_seconds`, not the range length, so time before the first event is not downtime. `null` when no window overlaps; `covered_from`/`covered_to` bound what the windows span.
- `windows` are newest first (`truncated: true` drops the oldest). `end_time: null` is the open window.
- `seconds` is the part of that window inside the queried range, with an open window counted
  up to `to`.
- `outages` counts the `down` windows overlapping the range.

### Errors

| Status | When |
|---|---|
| `400` | Unknown `device`, unparseable `from`/`to`, non-positive `limit`, or `from` not before `to`. |
| `401` | No token, or a token of the wrong tier. |
| `503` | `PIPELINE_DATABASE_URL` is not configured, or the API runs on the backup server (`FAILOVER_SIDE=aws`, body carries `code: unavailable_on_failover`, which wins). |

Reads hitting a dbt rebuild are retried twice before becoming a 500.

## Granting access

The role gv-api connects with needs `USAGE` on `marts` and `SELECT` on its tables, granted so it **survives a dbt run**:

```sql
GRANT USAGE ON SCHEMA marts TO gv_api;
-- the tables that exist now
GRANT SELECT ON ALL TABLES IN SCHEMA marts TO gv_api;
-- and the ones dbt will create in their place
ALTER DEFAULT PRIVILEGES IN SCHEMA marts GRANT SELECT ON TABLES TO gv_api;
```

Both are needed: a per-table grant dies when dbt recreates the table, and `ALTER DEFAULT PRIVILEGES` only covers future tables.

**Run the last statement as dbt's role** (or use `FOR ROLE <dbt role>`): default privileges attach to the granting role, not the schema.

A broken grant looks intermittent, because the rebuild retry absorbs the first failures.

## Events can be lost, which flatters the numbers

The producer publishes at QoS 0, non-retained, so events published while the consumer is down are **lost**. A lost `down` means the outage never becomes a window and uptime reads high; there is no gap to detect.

This API reports what the marts say; the fix belongs in the producer. After a consumer deploy, `state: "unknown"` is expected until the next transition.

## No liveness signal

`watchdog/ping` exists in the topic contract but nothing publishes to it, so "up and quiet" and "died silently" look the same. `stale` and `computed_at` are the limit for now.
