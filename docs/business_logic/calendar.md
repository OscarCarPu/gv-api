# Calendar

### Description

Events: a name and a start and end time. gv is the only interface; nothing syncs with Google.

### Business rules

- `end_time` must be after `start_time`; `name` cannot be empty.
- Listing returns events overlapping the range, ordered by start.
- Updating times moves the plan block whose `event_ref` is the event id (best-effort: a failure
  is logged and never fails the update). Deleting the event clears that `event_ref`.

### Decisions / Why

- **No recurrence, calendars, colours, all-day or reminders**: nothing used them once Google
  stopped being the backend. Recurring things belong to plan commitments.
- **Migration 035** kept only one-off events. Series, their overrides and plan links to single
  occurrences (`12@...`) were dropped.
