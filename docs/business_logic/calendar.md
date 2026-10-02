# Calendar

### Description

The user's Google calendars, mirrored locally and editable from gv. Google stays the backend, so
the phone, invitations and everything else that uses those calendars keep working. Four accounts
are connected, each with its own OAuth grant; every event carries its account.

### States / Lifecycle

**Account**: `connected` → `needs_reauth` on `invalid_grant` → `connected` once reconnected.
Disconnecting revokes the grant and deletes the account's data.

**Calendar**: discovered from the account's calendar list, synced or not (`sync_enabled`), each
with its own sync cursor and push channel. A calendar that disappears from Google is marked
deleted, not removed.

**Event**: whatever Google says. A recurring series is a master plus one row per modified or
cancelled occurrence.

### Business rules

**Google is the source of truth**
- Reads come from the mirror. Writes go to Google first and are stored only once accepted.
- No outbox: if Google refuses or is unreachable the request fails (`502`) and nothing is stored.
- Writes send `If-Match` with the stored etag; a `412` becomes a `409` (refetch and retry).

**Sync**
- One persisted `syncToken` per calendar: a full sync earns one, incrementals spend it.
- `410 Gone` wipes that calendar's local events and rebuilds them (common for holiday calendars).
- Listing uses `singleEvents=false`, `showDeleted=true`, `maxResults=2500` and no time bounds:
  Google forbids them next to a sync token. The initial import is therefore unbounded, so huge
  calendars start disabled.
- A cancelled one-off is deleted; a cancelled override is kept as the hole in its series.
- Overrides can arrive before their master, so master links are resolved after each pass.
- `invalid_grant` parks the account instead of retrying.
- Each pass re-reads every calendar list, since Google does not notify calendar list changes.

**Freshness: push, with polling as the safety net**
- A push channel per synced calendar delivers changes in seconds.
- Channels expire (about a week) and cannot be renewed: a replacement is created a day before
  expiry, then the old one is stopped.
- Notifications are debounced for two seconds per calendar.
- Polling (15 minutes) repeats everything push does, since push is not fully reliable.
- The webhook never syncs inline; Google drops channels that answer slowly.

**Recurring events**
- Stored as Google stores them (master `RRULE` plus overrides) and expanded on read.
- Expansion uses the event's IANA zone, so 09:00 stays 09:00 across DST; all-day events advance
  in whole local days.
- An occurrence is identified by its **original** start, even after an override moves it.
- Occurrences moved out of the queried window disappear from it; ones moved in appear.

**Editing a series**
- `scope=instance` edits one occurrence. If it has no override yet, the instance id is fetched
  from Google rather than constructed.
- `scope=following` ends the original series one second before the occurrence (`UNTIL` is
  inclusive; `COUNT` is dropped) and starts a new series there, with any `COUNT` reduced by what
  the old series keeps. The original is truncated first, so a failure leaves a series that stops
  early rather than two overlapping ones.
- `scope=instance`/`following` without an occurrence reference is refused.

**All-day events are dates**
- Stored as midnight-to-midnight instants in the calendar's zone (`start_tz`) for indexing, and
  returned as `start_date`/`end_date`.
- Clients must place them by those dates: calendars disagree on zones (some report `UTC`), so
  converting the instants shows a one-day event on two days.

**Colours**
- Google's colours are ignored: every primary calendar gets the same cyan and every holiday
  calendar the same green.
- gv assigns colours by creation order from a 12-colour palette, so new calendars never repaint
  existing ones. `color_override` always wins. Text colour is the client's call.

**What cannot be written**
- Calendars with the `reader` or `freeBusyReader` role, and Google-generated events (`birthday`,
  `fromGmail`, `workingLocation`). Writes are refused before reaching Google, and reads mark them
  `editable: false`.

**Moving between calendars**
- Same account: Google's `move`, keeping the event id.
- Across accounts: the event is recreated on the destination and deleted from the source; the id
  changes, attendee responses are lost, and the response says `recreated: true`. If the delete
  fails, the copy is kept and the error reported.

**Tokens**
- Refresh tokens are encrypted at rest (AES-256-GCM, key from the environment).
- Access tokens are cached and refreshed a minute before expiry.
- Consent URLs use `access_type=offline` and `prompt=consent`, since Google only issues a refresh
  token on first authorisation otherwise. A consent without a refresh token is rejected.

**Isolation and linked plan blocks**
- The domain never touches `tasks`, `habits` or `plan_blocks` directly. After a successful write
  it calls `planBlockSyncer`, which `plan` implements (see [plan](plan.md)):
  - `UpdateEvent` changing times: re-resolves the ref and pushes the new times, or detaches the
    link if a `scope=following` split moved the occurrence to a new series.
  - `DeleteEvent`: detaches the ref.
  - `MoveEvent`: detaches the old ref whenever the local id changes.
- These calls are best-effort: errors are logged and never fail the already-accepted write.

### Validations

**Create (`POST /calendar/events`)** — `calendar_id` and a non-empty `summary`; `starts_at`
(RFC3339, or `YYYY-MM-DD` when `all_day`); `ends_at` optional (defaults to one day for all-day,
one hour otherwise) and after the start; the calendar must be writable.

**Update (`PATCH /calendar/events/{ref}`)** — at least one field; a scope legal for the
reference; `recurrence` only with `scope=all`; end after start.

**Range (`GET /calendar/events`)** — `from` and `to` required, `to` after `from`, at most two
years apart.

### Side effects

- **A successful write** publishes on the SSE stream and queues an incremental sync of the
  calendar to pick up side effects (new overrides, bumped sequences).
- **Disabling a calendar** deletes its local events and stops its push channel.
- **Disconnecting an account** stops its channels, revokes the grant and cascades the delete.
- **A 412 or 404 from Google** queues a sync of that calendar.

### Decisions / Why

- **Mirror instead of proxying Google**: a month view is one local query instead of many
  paginated requests, it works offline from Google, and expansion lives in one place for all
  clients.
- **Write-through, no outbox**: a queue's failure modes (stuck, reordered, stale retries) cost
  more than offline writes are worth; gv-android removed its outbox for the same reason.
- **Push and poll**: push alone is unreliable and channels expire silently; poll alone is slow.
- **15-minute poll**: it is the safety net. Four accounts at 15 minutes is about a thousand
  requests a day.
- **Worker inside the API**: it shares the repository, tokens and shutdown.
- **Signed OAuth state, not stored**: no cleanup, survives restarts. Valid for 30 minutes rather
  than single-use, so one URL can connect several accounts in a row.
- **Public callback and webhook**: Google's redirect and notifications carry no bearer token, so
  each has its own guard (signed state, per-channel token).
- **Server-side expansion**: one implementation of DST and override handling.

### Alternatives rejected

- **Scraping Google Calendar's web UI with logged-in sessions**: fragile cookies, no automated
  login, no sync tokens, no push, forged write RPCs. The 7-day expiry it aimed to avoid only
  applies to unpublished consent screens.
- **CalDAV with an app password**: Google requires OAuth for CalDAV now, and CalDAV has no push.
- **Secret `.ics` URLs**: read-only and cached for hours.
- **Service account with domain-wide delegation**: Workspace only.
- **Self-hosted CalDAV instead of Google**: a different project.
- **Verification**: if Google ever requires it, sensitive scopes only need branding, a
  justification and a demo video, not a security assessment.

### One-time Google Cloud setup

1. A project with the **Google Calendar API** enabled.
2. Consent screen **External** and **published to production**; in *Testing*, refresh tokens
   expire after 7 days. The "unverified app" warning is passed via *Advanced → Go to app*. Grants
   then only die on revoke, six months unused, or over 100 live tokens.
3. Scopes: `https://www.googleapis.com/auth/calendar` and
   `https://www.googleapis.com/auth/userinfo.email` (to identify the account).
4. A **Web application** client with redirect URI `GOOGLE_OAUTH_REDIRECT_URL`:
   `https://gv-api.lab-ocp.com/calendar/google/callback` in production,
   `http://localhost:8080/calendar/google/callback` locally. Only localhost may use HTTP.
5. For push: verify the webhook domain in Search Console and register it with the project, or
   `events.watch` is refused and only polling runs.
