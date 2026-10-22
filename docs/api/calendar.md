# Calendar API

Events with a name and a time range. Full auth. A plan block links to an event through
`event_ref` (the event id as a string).

| Method | Path | Description |
|---|---|---|
| `GET` | `/calendar/events?from&to` | Events overlapping the range (RFC3339 or `YYYY-MM-DD`, at most two years). |
| `POST` | `/calendar/events` | Create. |
| `GET` | `/calendar/events/{id}` | One event. |
| `PATCH` | `/calendar/events/{id}` | Change `name`, `start_time` and/or `end_time`. |
| `DELETE` | `/calendar/events/{id}` | Delete. |

```json
{ "id": 12, "name": "Dentist", "start_time": "2026-10-23T09:00:00Z", "end_time": "2026-10-23T10:00:00Z" }
```

Create needs `name`, `start_time` and `end_time` (after the start). Validation errors are `400`,
a missing event `404`.

Moving an event (`start_time`/`end_time` in a PATCH) moves the plan block linked to it. Deleting
an event keeps the block and clears its `event_ref`.
