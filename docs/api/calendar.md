# Calendar

A local mirror of the user's Google calendars, editable from here. **Full auth**, except the
two endpoints Google itself has to reach.

Google stays the source of truth. Reads are served from the mirror; writes go to Google first
and are stored only once it accepts them, so a row here always corresponds to an event over
there.

## Setup

The domain mounts and answers with no Google credentials configured: accounts and calendars
come back empty, reads work, and anything that needs Google answers `503`. `GET
/calendar/sync/status` reports `configured: false`, which is the one place that says why
nothing is syncing.

To configure it, see the environment variables in the [README](../../README.md) and the
one-time Google Cloud setup in [business_logic/calendar.md](../business_logic/calendar.md).

## References

An event reference is either an event id or one occurrence of a recurring series:

| Reference | Means |
|---|---|
| `12` | the event with local id 12 (a one-off, or the series as a whole) |
| `12@2026-08-20T07:00:00Z` | the occurrence of series 12 whose **original** start is that instant |

The suffix is the original start, not the current one: an override can move an occurrence to
another day, and the slot it came from is the only stable name it has. It is what `instance_id`
returns and what `PATCH`/`DELETE` accept.

## Accounts

### `GET /calendar/accounts`

```json
[
  {
    "id": 1,
    "email": "me@example.com",
    "label": "personal",
    "color": "#3366cc",
    "status": "connected",
    "calendars": 4,
    "last_sync_at": "2026-08-20T10:14:26Z",
    "last_sync_error": null,
    "created_at": "2026-08-19T18:02:11Z"
  }
]
```

`status` is `connected`, `needs_reauth` (Google rejected the stored grant — only a person can
fix it, by connecting the account again) or `revoked`.

### `POST /calendar/accounts/auth-url`

Returns the Google consent URL for adding an account. One account per grant: four Google
logins mean running this four times.

```json
{ "url": "https://accounts.google.com/o/oauth2/auth?access_type=offline&prompt=consent&..." }
```

`503` when Google is not configured on the server.

### `PATCH /calendar/accounts/{id}`

`label` and `color`, both optional. Local only — nothing is sent to Google.

### `DELETE /calendar/accounts/{id}`

Stops the account's push channels, revokes the grant at Google, and deletes the account with
its calendars and events. `204`.

### `POST /calendar/accounts/{id}/resync`

Throws the local copy of that account away and rebuilds it from scratch. The escape hatch for
when the mirror is visibly wrong.

```json
{ "calendars": 4, "upserted": 812, "deleted": 0, "errors": [] }
```

## Calendars

### `GET /calendar/calendars`

Every calendar of every account, with its sync state.

```json
[
  {
    "id": 1,
    "account_id": 1,
    "account_email": "me@example.com",
    "account_status": "connected",
    "google_calendar_id": "me@example.com",
    "summary": "Personal",
    "description": "",
    "time_zone": "Europe/Madrid",
    "color": "#3366cc",
    "foreground_color": "#ffffff",
    "access_role": "owner",
    "writable": true,
    "is_primary": true,
    "sync_enabled": true,
    "visible": true,
    "deleted": false,
    "sync": {
      "has_sync_token": true,
      "last_sync_at": "2026-08-20T10:14:26Z",
      "last_full_sync_at": "2026-08-19T18:02:14Z",
      "last_sync_error": null,
      "watch_active": true,
      "watch_expires_at": "2026-08-26T18:02:14Z"
    }
  }
]
```

- `writable` is `false` for the `reader` and `freeBusyReader` roles. Writes to those are
  refused here rather than forwarded to be rejected.
- `color` is the local override when there is one, and Google's colour otherwise.
- `deleted: true` means the calendar is gone from Google. The row and its events stay, so a
  view does not empty out without explanation.
- `watch_active: false` means changes to that calendar arrive on the next poll instead of in
  seconds.

### `PATCH /calendar/calendars/{id}`

Local preferences only:

| Field | Effect |
|---|---|
| `sync_enabled` | whether the calendar is synced at all. Turning it off deletes its local events and stops its push channel — the mirror must not keep stale rows that look current |
| `visible` | whether it is included in `visible_only` queries |
| `color_override` | overrides Google's colour |

Holiday, birthday and week-number calendars start with `sync_enabled: false`: the initial
import cannot be bounded by date (Google forbids `timeMin` next to a sync token), so a calendar
with a decade of public holidays would be imported whole.

## Events

### `GET /calendar/events?from&to`

Everything happening in `[from, to)`, with recurring series already expanded into occurrences.

| Parameter | Notes |
|---|---|
| `from`, `to` | required, RFC3339 or `YYYY-MM-DD`. `to` is exclusive. Ranges longer than two years are refused |
| `calendar_ids` | comma-separated, optional filter |
| `account_ids` | comma-separated, optional filter |
| `visible_only` | `true` drops the calendars the user has hidden |

```json
[
  {
    "instance_id": "12@2026-08-20T07:00:00Z",
    "event_id": 12,
    "calendar_id": 1,
    "account_id": 1,
    "account_email": "me@example.com",
    "calendar_name": "Personal",
    "color": "#3366cc",
    "google_event_id": "6h1r...",
    "summary": "Standup",
    "description": "",
    "location": "",
    "status": "confirmed",
    "event_type": "default",
    "all_day": false,
    "starts_at": "2026-08-20T07:00:00Z",
    "ends_at": "2026-08-20T07:30:00Z",
    "time_zone": "Europe/Madrid",
    "recurring": true,
    "recurrence": ["RRULE:FREQ=DAILY;COUNT=5"],
    "is_exception": false,
    "original_starts_at": "2026-08-20T07:00:00Z",
    "editable": true,
    "organizer_email": "me@example.com",
    "attendees": [],
    "reminders": { "use_default": true },
    "html_link": "https://calendar.google.com/event?eid=...",
    "created_by_gv": false
  }
]
```

- Sorted by `starts_at`.
- `account_email` is the "source account" — with four accounts connected, it is what says
  which one an event belongs to.
- `editable: false` for read-only calendars, parked accounts, and the event kinds Google
  generates itself (`birthday`, `fromGmail`, `workingLocation`).
- An all-day event is midnight-to-midnight in `time_zone` with an **exclusive** end, the same
  convention Google uses.

### `GET /calendar/events/{ref}`

One event or one occurrence. `404` when the reference does not resolve, including an occurrence
the recurrence rule does not produce and one that has been cancelled.

### `POST /calendar/events`

Creates the event in Google and mirrors the answer. `201`.

```json
{
  "calendar_id": 1,
  "summary": "Dentist",
  "description": "second floor",
  "location": "Clínica",
  "all_day": false,
  "starts_at": "2026-08-20T17:00:00Z",
  "ends_at": "2026-08-20T18:00:00Z",
  "time_zone": "Europe/Madrid",
  "recurrence": ["RRULE:FREQ=WEEKLY;BYDAY=TH"],
  "attendees": [{ "email": "someone@example.com", "optional": true }],
  "reminders": { "use_default": false, "overrides": [{ "method": "popup", "minutes": 30 }] },
  "send_updates": "none"
}
```

- `calendar_id` and `summary` are required. `starts_at` is required: RFC3339, or `YYYY-MM-DD`
  when `all_day` is set.
- `ends_at` may be omitted: an all-day event gets one day, a timed event gets an hour.
- `send_updates` is `none` (default), `externalOnly` or `all`. It defaults to `none` so editing
  your own calendar does not mail people by accident.

### `PATCH /calendar/events/{ref}`

Every field is optional; a field that is absent is left alone. Times behave like a calendar UI
expects: moving `starts_at` alone drags `ends_at` with it, keeping the length.

`scope` decides what a change to a recurring series touches:

| `scope` | Effect |
|---|---|
| `instance` | that occurrence only. The default when the reference names one |
| `following` | that occurrence and everything after it: the original series is ended just before it and a new one carries the change |
| `all` | the whole series (or the plain event). The default when the reference has no occurrence |

`instance` and `following` require an occurrence reference and a recurring event; asking for
them without one is a `400` rather than a guess. `recurrence` can only be changed with
`scope=all`.

After a `following` split the response describes the occurrence you edited, which now belongs
to the **new** series — its `event_id` has changed.

### `DELETE /calendar/events/{ref}?scope&send_updates`

`204`. Same scopes. Deleting a single occurrence leaves a hole in the series rather than
removing a row; deleting with `scope=all` removes the event.

### `POST /calendar/events/{ref}/move`

```json
{ "calendar_id": 3, "send_updates": "none" }
```

```json
{ "event": { "...": "..." }, "recreated": false }
```

Google can only move an event between calendars of the **same account**. Across accounts there
is no move: the event is recreated on the destination and removed from the source, so its id
changes and any attendee responses are lost. That case answers `recreated: true` — a client
must not keep using the old reference. Occurrences cannot be moved between calendars (`400`).

## Sync

### `POST /calendar/sync[?calendar_id=]`

Syncs now — everything, or one calendar. Returns the same shape as a resync. Push notifications
and the background poll do this on their own; this is for when someone wants it now.

### `GET /calendar/sync/status`

The operational view. A push channel can die quietly and a sync token can expire overnight;
this is where that is visible before someone notices their calendar stopped changing.

```json
{
  "configured": true,
  "webhooks_active": true,
  "poll_interval": "15m0s",
  "accounts": [{ "id": 1, "email": "me@example.com", "status": "connected" }],
  "calendars": [
    {
      "calendar_id": 1,
      "account_email": "me@example.com",
      "summary": "Personal",
      "sync_enabled": true,
      "events": 812,
      "sync": { "has_sync_token": true, "watch_active": true, "watch_expires_at": "2026-08-26T18:02:14Z" }
    }
  ],
  "recent_runs": [
    {
      "id": 41, "calendar_id": 1, "trigger": "webhook", "kind": "incremental",
      "started_at": "2026-08-20T10:14:26Z", "finished_at": "2026-08-20T10:14:27Z",
      "pages": 1, "upserted": 1, "deleted": 0, "error": null
    }
  ]
}
```

`trigger` is `poll`, `webhook`, `manual` or `connect`.

### `GET /calendar/stream`

Server-sent events. Says *that* something changed, so the client refetches the range it is
showing — one code path for "opened the page" and "something moved", and no incremental patch
stream to drift out of step with the database.

```
: connected

event: calendar.changed
data: {"type":"calendar.changed","calendar_id":1,"at":"2026-08-20T10:14:27Z"}

: ping
```

Message types: `calendar.changed`, `account.connected`, `account.disconnected`,
`account.needs_reauth`. A `: ping` comment every 25 seconds keeps the Cloudflare tunnel from
dropping an idle connection.

**Consuming it from a browser**: this endpoint takes the bearer token in the `Authorization`
header like every other one, and the `EventSource` API cannot set headers. So a browser client
either streams it with `fetch` and reads the `ReadableStream`, or gv-web proxies it from its own
server (where the token already lives, in `event.locals`). The token deliberately does *not*
travel in the query string — it would end up in logs and history.

## Public endpoints

Neither can carry a bearer token, so each has its own guard.

### `GET /calendar/google/callback`

Where Google's redirect lands after consent. Guarded by the signed `state` parameter (HMAC over
a nonce and a 10-minute expiry). Always answers `302`, back to the web app with `?connected=`
or `?error=` — the person is looking at a browser tab, where a JSON error is a dead end.

### `POST /calendar/google/webhook`

Google's push notification: no body, no credentials, everything in `X-Goog-*` headers. Guarded
by the per-channel token in `X-Goog-Channel-Token`, compared in constant time.

Answers `200` for anything it can place, **including an unknown channel** — Google retries a
non-200 and eventually drops the channel, which is not worth losing over a notification for a
channel we already replaced. A wrong token answers `401`. It never syncs inline: it queues the
calendar and returns.

## Status codes

| Code | When |
|---|---|
| `400` | bad range, bad scope, unparseable times, invalid ids |
| `403` | read-only calendar, or an event kind Google generates |
| `404` | unknown event, calendar or account; an occurrence that does not exist |
| `409` | either the event changed in Google since we last read it (`refetch and retry`), or the account needs reconnecting (`account needs to be reconnected`). The message distinguishes them |
| `502` | Google refused or failed the write. Nothing was stored locally |
| `503` | Google is not configured on this server |
