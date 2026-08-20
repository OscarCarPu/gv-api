# Uptime (Domotics)

How much of the time the home lab and its ESP32 watchdog have been reachable. **Semiprivate
auth** — either token gets in, same as lights: house state, not personal data.

## Where the numbers come from

Not from here. Each device publishes an up/down event to MQTT
(`events/uptime/lab`, `events/uptime/watchdog`), [central-pipeline][cp] consumes them, and
dbt models them into two marts in **its own PostgreSQL instance**:

```
device → MQTT → central-pipeline (raw → staging → marts) → gv-api → client
```

gv-api only reads them, over a second connection with its own DSN
(`PIPELINE_DATABASE_URL`), read-only on the connection itself. That schema belongs to dbt,
which drops and recreates it on every run, so nothing here is migrated from gv-api and no
gv-side view or foreign key is built on top of it. See `internal/pipeline` — the connection
is shared by every domain that reads a mart, not owned by this one.

With no DSN configured both endpoints answer **503**. That is a deployment state, not a
fault: the rest of the API is unaffected.

[cp]: https://github.com/OscarCarPu/central-pipeline

## What the numbers are not

- **Not live.** `computed_at` is when dbt last ran, not now, and dbt is a batch job. Anything
  older than `PIPELINE_STALE_AFTER_MS` (2h by default) comes back with `stale: true`. Show
  the timestamp; never present these percentages as the present.
- **Not a heartbeat.** Events are edge-triggered, so `since` being days old means "nothing
  has changed", not "nothing is alive". The open window counts as up right up to
  `computed_at`, which means a device that dies without publishing `down` keeps reading as up
  until its peer reports it. If both die, uptime stays high and nothing here flags it.
- **Not anchored to today - 1 year.** Every range is floored at the device's first event, so
  a young device reports its real history instead of ~0%. Read `range_start` rather than
  recomputing it.

## Endpoints

### `GET /domotics/uptime`

The dashboard read: where both devices stand now, plus the four percentages the pipeline
precomputed for each. One indexed read per table, no date maths.

Both devices always appear, in a fixed order (`lab`, `watchdog`), and their ranges are
ordered shortest lookback first. A device the pipeline has never heard from comes back with
`state: "unknown"`, `since: null` and no ranges rather than being dropped.

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

State changes over an arbitrary range, with the percentage computed for exactly that range
instead of read off a precomputed row. This is the endpoint for timelines, incident lists and
"when did it last go down".

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

- `uptime` divides by `up_seconds + down_seconds`, **not** by the length of the range: before
  a device's first event there is nothing to call up or down, and charging that gap as
  downtime would report a young device as mostly dead. `null` when no window overlaps at all.
  `covered_from`/`covered_to` bound the part of the range the windows actually span.
- `windows` are newest first, so a `truncated: true` list keeps the recent history and drops
  the distant past. `end_time: null` is the open window — the state the device is in now.
- `seconds` is the part of that window inside the queried range, with an open window counted
  up to `to`.
- `outages` counts the `down` windows overlapping the range.

### Errors

| Status | When |
|---|---|
| `400` | Unknown `device`, unparseable `from`/`to`, non-positive `limit`, or `from` not before `to`. |
| `401` | No token, or a token of the wrong tier. |
| `503` | `PIPELINE_DATABASE_URL` is not configured. |

A read that lands in the middle of a dbt rebuild (the relation momentarily does not exist) is
retried twice before it becomes a 500 — see `internal/pipeline`.

## Granting access

The role gv-api connects with needs `USAGE` on `marts` and `SELECT` on its tables, granted so
that it **survives a dbt run**:

```sql
GRANT USAGE ON SCHEMA marts TO gv_api;
ALTER DEFAULT PRIVILEGES IN SCHEMA marts GRANT SELECT ON TABLES TO gv_api;
```

A bare `GRANT SELECT ON marts.uptime_windows` dies with the table the next time dbt drops and
recreates it. The rebuild retry above then masks the first few failures, so a permanently
broken grant reads as an intermittent one — worth getting right the first time.

## Events can be lost, which flatters the numbers

The producer publishes at QoS 0, non-retained. Delivery is `min(publish QoS, subscribe QoS)`,
so anything published while the pipeline's consumer is down is **lost, not delayed**. The loss
is invisible from here and biased one way: a missing `down` means the outage never becomes a
window, so uptime reads higher than it was, and the windows stay contiguous either way — there
is no gap for gv-api to detect.

Two things follow. Nothing here interpolates or reconciles: this API reports what the marts
say, and the fix belongs in the producer. And a freshly deployed consumer learns nothing about
current state until the next transition — on a stable device that can be days, so
`state: "unknown"` with no ranges is the expected first answer after a deploy rather than a
wiring fault.

## No liveness signal

There is deliberately none. `watchdog/ping` exists in the pipeline's topic contract but nothing
publishes to it yet, so nothing here can distinguish "up and quiet" from "died without saying
so". `stale` and `computed_at` are the honest ceiling; when a real heartbeat lands it will
arrive as an additive column, not a change to these responses.
