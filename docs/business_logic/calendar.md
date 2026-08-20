# Calendar

### Description

Domain for the user's calendars, mirrored from Google and editable from here. It exists to
replace the Google Calendar UI without giving up Google: the phone, the invitations other
people send, and everything else that already points at those calendars keeps working, while
the day-to-day view and editing happen in gv.

Four Google accounts are connected, each with its own OAuth grant. Every event carries the
account it came from, which is what makes one view of four accounts legible.

### States / Lifecycle

**Account**: `connected` → `needs_reauth` when Google answers `invalid_grant` (the grant is
gone; only a person can restore it, by connecting the account again) → `connected` when they
do. Disconnecting revokes the grant at Google and deletes the account's data.

**Calendar**: discovered from the account's calendar list, then either synced or not
(`sync_enabled`). Each calendar independently holds a sync cursor and a push channel. A
calendar that disappears from Google is marked deleted, not removed.

**Event**: has no lifecycle of its own here — it is whatever Google says it is. A recurring
series is a master plus one row per modified or cancelled occurrence.

### Business rules

**Google is the source of truth**
- Reads are served from the mirror. Writes go to Google first and are stored only once it
  accepts them, so a row in `calendar_events` always corresponds to an event over there.
- There is no outbox. If Google refuses or is unreachable, the request fails (`502`) and
  nothing is stored: the two copies never disagree, and there is no queue to be stuck.
- Every write sends `If-Match` with the stored etag. A `412` becomes a `409` for the client —
  the event changed elsewhere, refetch and retry — rather than an overwrite of someone else's
  change.

**Sync**
- One `syncToken` per calendar, persisted. A full sync earns one, incrementals spend it.
- `410 Gone` (a spent token, or an ACL change) wipes that calendar's local events and rebuilds
  them. This is Google's documented recovery and holiday calendars go through it regularly.
- Listing parameters are fixed: `singleEvents=false`, `showDeleted=true`, `maxResults=2500`,
  and **no time bounds at all**. Google forbids `timeMin`/`timeMax`/`updatedMin`/`q`/`orderBy`
  next to a sync token and treats any other difference from the initial full sync as undefined,
  so using them once would break every incremental sync that followed. The cost is that the
  initial import cannot be bounded by date; the mitigation is that huge calendars start
  disabled.
- A cancelled one-off is deleted locally. A cancelled *override* is kept: it is the hole in a
  live series, and dropping it makes the occurrence reappear on the next expansion.
- Overrides can arrive in an earlier page than their master, so the master link is resolved
  after each pass rather than per row.
- `invalid_grant` parks the account instead of retrying: nothing but a person can fix it.
- Google sends no notification when a calendar is created, shared or unshared, so each pass
  re-reads every account's calendar list.

**Freshness: push first, poll as the safety net**
- A push channel per synced calendar makes a change in Google visible here in seconds.
- Channels expire (about a week in practice) and **cannot be renewed in place**: a replacement
  is created a day before expiry and the old one is stopped afterwards, in that order, so a
  failure to store the new one never leaves the calendar with no channel at all.
- Notifications are coalesced for two seconds per calendar. One change in Google produces
  several notifications, and one per attendee who responds; without the debounce each would
  spend a sync round-trip.
- The poll (15 minutes by default) is not decoration: Google states plainly that push
  notifications are not 100% reliable, and a channel can die without a sound. Everything the
  push path does, the poll does again.
- The webhook never syncs inline. Google retries a slow POST and eventually drops the channel.

**Recurring events**
- Stored the way Google stores them — master with `RRULE` plus overrides — and expanded on
  read. Storing the expansion would mean either unbounded rows for an endless series or a
  horizon that quietly truncates the calendar.
- Expansion happens in the event's own IANA zone, so a weekly 09:00 stays 09:00 across a DST
  change, and an all-day event is advanced in whole local days (the day of a DST change is 23
  or 25 hours long).
- An occurrence is identified by its **original** start. An override that moves an occurrence
  to another day still belongs to the slot it came from, which is how Google identifies it and
  the only stable name a client can hold.
- Both directions of a move are handled: an occurrence whose override moved it out of the
  queried window disappears from it, and one moved in from outside appears.

**Editing a series**
- `scope=instance` edits one occurrence. If Google has no override for it yet, one is
  materialised — by asking Google for the instance rather than constructing its id, because
  that format is documented loosely and getting it wrong writes to the wrong event.
- `scope=following` splits: the original series is ended just before the occurrence
  (`UNTIL`, minus a second because `UNTIL` is inclusive, with any `COUNT` dropped) and a new
  series starts at it. When the rule counted occurrences, the leftover count is worked out from
  how many the old series keeps — otherwise the series would get longer with every edit.
  The original is truncated first: if creating the tail then fails, the visible result is a
  series that stops early rather than two overlapping ones.
- `scope=instance`/`following` without an occurrence reference is refused. Guessing there
  rewrites a whole series.

**What cannot be written**
- Calendars with the `reader` or `freeBusyReader` role, and the event kinds Google generates
  itself (`birthday`, `fromGmail`, `workingLocation`). Both are refused here, before the
  request is sent, and both are flagged as `editable: false` on reads so a client can grey the
  buttons out instead of discovering it on submit.

**Moving between calendars**
- Same account: Google's `move`, which keeps the event id.
- Different accounts: no such operation exists, so the event is recreated on the destination
  and deleted from the source. The id changes and attendee responses are lost, and the response
  says `recreated: true`. If the delete fails after the copy exists, the copy is kept and the
  failure is reported: deleting it to "roll back" could destroy the only remaining version.

**Tokens**
- Refresh tokens are stored encrypted (AES-256-GCM, key from the environment). The database
  gets backed up; a plaintext refresh token in a backup is a copy of full calendar access.
- Access tokens are cached and refreshed a minute before expiry — a token that expires mid-sync
  turns a clean pass into a 401 halfway through.
- `access_type=offline` **and** `prompt=consent` on every consent URL. Google only issues a
  refresh token on the first authorisation, so without `prompt=consent` a reconnect returns an
  access token alone and appears to work until it expires an hour later. A consent that comes
  back without a refresh token is rejected rather than stored.

**Isolation**
- Nothing here touches `plan_blocks`, `tasks` or `habits`. A unified view is a later decision,
  deliberately not made now.

### Validations

**Create (`POST /calendar/events`)** — `calendar_id` and a non-empty `summary` required;
`starts_at` required (RFC3339, or `YYYY-MM-DD` when `all_day`); `ends_at` optional (one day
for all-day, one hour otherwise) and must be after the start; the calendar must be writable.

**Update (`PATCH /calendar/events/{ref}`)** — at least one field; the scope must be legal for
the reference (see above); `recurrence` only with `scope=all`; the resulting interval must have
the end after the start.

**Range (`GET /calendar/events`)** — `from` and `to` required, `to` strictly after `from`,
span at most two years. Unbounded expansion of endless series is the reason for the cap.

### Side effects

- **A successful write** publishes on the SSE stream and queues an incremental sync of that
  calendar. The sync is what reconciles what a write does beyond its own response: a
  materialised override, a bumped sequence on the master, a cancelled sibling.
- **Disabling a calendar** deletes its local events and stops its push channel. Stale rows that
  look current are worse than no rows.
- **Disconnecting an account** stops its channels, revokes the grant, and cascades the delete
  through calendars, events and sync runs.
- **A 412 or a 404 from Google** queues a sync of that calendar: both mean the local copy is
  behind.

### Decisions / Why

- **Why mirror at all, instead of proxying Google per request**: a month view is one query
  here and dozens of paginated requests there, it works when Google or the network does not,
  and the range/expansion logic lives in one place for gv-web and gv-android both. The price is
  a sync, and the sync is the feature.
- **Why write-through and not an outbox**: the outbox in gv-android was removed for exactly
  this reason — the queue's failure modes (stuck, reordered, retried after the user changed
  their mind) cost more than the offline writes were worth. Failing loudly beats diverging
  quietly.
- **Why push *and* poll**: push alone is not reliable (Google says so) and its channels expire
  silently; poll alone means minutes of staleness. Together, one covers the other's failure.
- **Why the poll is 15 minutes rather than 1**: it is the safety net, not the mechanism. With
  webhooks working, a shorter interval buys nothing but quota; four accounts at 15 minutes is
  about a thousand requests a day against a million-a-day limit.
- **Why the sync worker lives in the API process**: it needs the same repository, tokens and
  shutdown as everything else. An external cron would have to authenticate against this API to
  ask it to do what it already knows how to do, and the real trigger is a webhook anyway.
- **Why the state parameter is signed rather than stored**: it is single-use by being
  short-lived, needs no cleanup, and survives a restart in the middle of a consent flow, which
  a value in memory would not.
- **Why the callback and the webhook are public**: they have to be. Google's redirect lands on
  the API host, where the web app's session cookie does not exist, and Google's notification
  POST carries no credentials at all. Each gets a guard of its own — a signed state, a
  per-channel secret — rather than an exemption.
- **Why expansion is server-side**: two clients would otherwise each grow their own half-right
  version of DST handling and override precedence.

### Alternatives considered and rejected

**Keeping four browser sessions logged in and scraping Google Calendar's own frontend.** The
idea was to avoid "logging in every 7 days" — but that expiry is exclusively a property of the
*Testing* consent screen, and publishing the app removes it. What the approach would cost
instead: cookies Google invalidates on a password change, a security event or a new IP, each
time needing a manual re-login with no warning; no automated login at all (headless browsers
are blocked); no sync tokens, so full snapshot diffs instead of incremental changes; **no push
notifications, so no near-real-time**, which is the requirement that started this; writes that
mean forging internal RPCs against real calendars; `/u/0`, `/u/1` indices that reorder when an
account is added; and automated access outside the published APIs, on four accounts in daily
use. It trades a stable credential for a fragile one and loses the two features that matter.

**CalDAV with an app password.** Closed off: Google has required OAuth for CalDAV/CardDAV since
2023–2025, and app passwords no longer work there. CalDAV has no push either, so it could not
deliver near-real-time updates regardless.

**The secret `.ics` URL per calendar.** No login needed, but read-only and cached by Google for
hours. Fails both requirements.

**A service account with domain-wide delegation.** Google Workspace only; these are personal
accounts.

**Self-hosting CalDAV (Radicale/Baikal) as the source of truth and dropping Google.** The only
architecture that genuinely removes the dependency. It also means DAVx⁵ on the phone and
invitations from other people still landing in Gmail. A different project, not a shortcut for
this one.

**If Google ever pushes back on the unverified app**: with only *sensitive* scopes (Calendar's
are), verification asks for branding, a justification and a demo video — not the third-party
security assessment that *restricted* scopes require. It is a route, not a wall.

### One-time Google Cloud setup

1. A Google Cloud project with the **Google Calendar API** enabled.
2. Consent screen: **External**, and **published to production**. This is not cosmetic: while
   the app is in *Testing*, refresh tokens expire after **7 days** and every account has to be
   reconnected weekly. Published-but-unverified shows a "Google hasn't verified this app"
   warning, which is passed with *Advanced → Go to app*; the 100-user cap is irrelevant for one
   person. With Calendar scopes the grant then only dies on revoke, six months unused, or more
   than 100 live tokens for the client — a password change does **not** revoke it (that only
   applies to Gmail scopes).
3. Scopes: `https://www.googleapis.com/auth/calendar` and
   `https://www.googleapis.com/auth/userinfo.email` (the second is only used to learn which
   account was connected).
4. A **Web application** client whose redirect URI is `GOOGLE_OAUTH_REDIRECT_URL`, i.e.
   `https://gv-api.lab-ocp.com/calendar/google/callback` in production and
   `http://localhost:8080/calendar/google/callback` for development. Google requires HTTPS for
   everything except localhost, so a LAN IP is not a usable redirect.
5. For push notifications: the webhook's domain must be **verified** (Search Console) and
   registered with the Cloud project. Without that step `events.watch` is refused and the
   feature runs on the poll alone.
