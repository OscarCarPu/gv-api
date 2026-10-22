# Uptime (Domotics)

How much of the time the service answered: from home (up), by nobody (down), or from the backup server (failover). Failover counts as uptime in the percentage. **Semiprivate auth**.

## Where the numbers come from

One table, `outages`: a timeline of ranges. A range has no start of its own, it starts where the previous row ends.

| `kind` | Meaning |
|---|---|
| `start` | The origin, not a range. Its `ends_at` is when migration 036 ran; nothing before it is counted. |
| `up` | Home answered. |
| `down` | Nobody answered. |
| `failover` | The backup server answered. |

- **Heartbeat.** Every 30 s gv-api moves forward the `ends_at` of the last `up` row. A silence of more than 90 s becomes a `down` row ending when gv-api writes again, so restarts, crashes and home outages shorter than the failover threshold are recorded. It writes only while `FAILOVER_SIDE=home`, the failover controller is in `normal` mode and the AWS state is `NORMAL`.
- **Failover record.** After a failover the Lambda sends `POST /failover/outage` with `ec2_up_at` and `home_at`. gv-api closes the `up` row at the last heartbeat home wrote before the restore replaced its database, then adds `down` until `ec2_up_at` and `failover` until `home_at`. The call is accepted only while the AWS state is `HOME_READY` and is idempotent.

On the backup server (`FAILOVER_SIDE=aws`) both endpoints answer `503` with `code: "unavailable_on_failover"` and no heartbeat runs.

## Endpoints

### `GET /domotics/uptime`

```json
{
  "up_since": "2026-08-13T16:54:25Z",
  "ranges": [
    {
      "range": "month",
      "uptime": 99.92,
      "up_seconds": 2584000,
      "down_seconds": 900,
      "failover_seconds": 6100,
      "range_start": "2026-07-20T18:07:44Z",
      "range_end": "2026-08-19T18:07:44Z"
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `up_since` | End of the last downtime or failover, or the start of the history. |
| `range` | `month`, `3 months`, `year` or `all`, in that order. |
| `uptime` | `(up + failover) / covered`, 0-100, two decimals. `null` when nothing is covered yet. |
| `range_start` | Floored at the start of the history, so a young system is not reported at ~0%. |

### `GET /domotics/uptime/windows`

| Query | Default | Meaning |
|---|---|---|
| `from` | `to` - 30 days | RFC 3339 or `YYYY-MM-DD` (UTC midnight). Floored at the start of the history. |
| `to` | now | Same formats. Clamped to now. |
| `limit` | 1000 | Windows returned, newest first, capped at 5000. The totals ignore it. |

```json
{
  "from": "2026-08-01T00:00:00Z",
  "to": "2026-08-10T00:00:00Z",
  "uptime": 99.1,
  "up_seconds": 766800,
  "down_seconds": 900,
  "failover_seconds": 6100,
  "outages": 1,
  "windows": [{ "kind": "failover", "start_time": "…", "end_time": "…", "seconds": 6100 }],
  "truncated": false
}
```

`seconds` is the part of the window inside the range. `outages` counts the `down` windows overlapping it.

### Errors

| Status | When |
|---|---|
| `400` | Unparseable `from`/`to`, non-positive `limit`, or `from` not before `to`. |
| `401` | No token, or a token of the wrong tier. |
| `503` | `unavailable_on_failover`, on the backup server. |
