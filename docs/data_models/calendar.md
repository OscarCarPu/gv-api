# Calendar - Data Models

Three levels, because that is the shape Google's model has and flattening it would lose what
the sync needs: an **account** holds the OAuth grant (one per Google login), a **calendar**
holds the sync cursor (Google issues one `syncToken` per calendar, not per account), and an
**event** is the mirrored row.

## Tables

### google_accounts

| Column | Type | Constraints |
|---|---|---|
| id | SERIAL | PRIMARY KEY |
| email | TEXT | NOT NULL, UNIQUE |
| label | TEXT | NOT NULL, DEFAULT `''` |
| color | TEXT | NOT NULL, DEFAULT `''` |
| refresh_token | BYTEA | NOT NULL — AES-256-GCM, nonce prepended |
| access_token | BYTEA | nullable, same encryption (a cache) |
| access_token_expires_at | TIMESTAMPTZ | nullable |
| scopes | TEXT | NOT NULL, DEFAULT `''` |
| status | TEXT | NOT NULL, DEFAULT `'connected'`, CHECK in (`connected`, `needs_reauth`, `revoked`) |
| last_sync_at | TIMESTAMPTZ | nullable |
| last_sync_error | TEXT | nullable |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() (updated touched by trigger) |

The refresh token is the whole feature's key: it grants read and write access to the account's
calendars and does not expire. It is encrypted because the database is backed up, and a
plaintext token in a backup is a copy of the grant.

### calendars

| Column | Type | Constraints |
|---|---|---|
| id | SERIAL | PRIMARY KEY |
| account_id | INT | NOT NULL, FK → `google_accounts.id` ON DELETE CASCADE |
| google_calendar_id | TEXT | NOT NULL |
| summary / description | TEXT | NOT NULL, DEFAULT `''` |
| time_zone | TEXT | NOT NULL, DEFAULT `'UTC'` |
| background_color / foreground_color | TEXT | NOT NULL, DEFAULT `''` (Google's) |
| color_override | TEXT | NOT NULL, DEFAULT `''` (local) |
| access_role | TEXT | NOT NULL, DEFAULT `'reader'` — `owner`/`writer` are writable |
| is_primary | BOOLEAN | NOT NULL, DEFAULT FALSE |
| sync_enabled | BOOLEAN | NOT NULL, DEFAULT TRUE |
| visible | BOOLEAN | NOT NULL, DEFAULT TRUE |
| sync_token | TEXT | nullable — NULL means the next pass is a full sync |
| sync_token_updated_at / last_full_sync_at / last_sync_at | TIMESTAMPTZ | nullable |
| last_sync_error | TEXT | nullable |
| watch_channel_id / watch_resource_id / watch_token | TEXT | nullable |
| watch_expires_at | TIMESTAMPTZ | nullable |
| deleted_at | TIMESTAMPTZ | nullable — gone from Google, kept here |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() (updated touched by trigger) |

**Indexes:**
- UNIQUE (`account_id`, `google_calendar_id`)
- `idx_calendars_account` on (`account_id`)
- `idx_calendars_watch_channel` UNIQUE on (`watch_channel_id`) WHERE NOT NULL — the webhook's
  only lookup key

`watch_token` is a secret this app generates. Google echoes it back in `X-Goog-Channel-Token`,
and it is the only thing authenticating a webhook that by necessity is public.

The three columns Google owns (`summary`, colours, `access_role`) are refreshed on every
calendar-list pass; the three the user owns (`sync_enabled`, `visible`, `color_override`)
deliberately are not.

### calendar_events

| Column | Type | Constraints |
|---|---|---|
| id | SERIAL | PRIMARY KEY |
| calendar_id | INT | NOT NULL, FK → `calendars.id` ON DELETE CASCADE |
| google_event_id | TEXT | NOT NULL |
| ical_uid | TEXT | NOT NULL, DEFAULT `''` — stable across accounts; what a cross-account dedup would key on |
| etag | TEXT | NOT NULL, DEFAULT `''` — sent as `If-Match` on every write |
| sequence | INT | NOT NULL, DEFAULT 0 |
| status | TEXT | NOT NULL, DEFAULT `'confirmed'` |
| event_type | TEXT | NOT NULL, DEFAULT `'default'` |
| summary / description / location | TEXT | NOT NULL, DEFAULT `''` |
| all_day | BOOLEAN | NOT NULL, DEFAULT FALSE |
| starts_at / ends_at | TIMESTAMPTZ | NOT NULL |
| start_tz / end_tz | TEXT | NOT NULL, DEFAULT `''` (IANA) |
| recurrence | TEXT[] | nullable — `RRULE`/`EXDATE`/`RDATE` as Google returns them |
| recurring_event_id | TEXT | nullable — Google's id of the master |
| master_id | INT | nullable, FK → `calendar_events.id` ON DELETE CASCADE |
| original_starts_at | TIMESTAMPTZ | nullable — which occurrence an override replaces |
| organizer_email / creator_email | TEXT | NOT NULL, DEFAULT `''` |
| attendees | JSONB | NOT NULL, DEFAULT `'[]'` |
| reminders | JSONB | NOT NULL, DEFAULT `'{}'` |
| transparency / visibility / color_id | TEXT | NOT NULL, DEFAULT `''` |
| html_link / hangout_link | TEXT | NOT NULL, DEFAULT `''` |
| created_by_gv | BOOLEAN | NOT NULL, DEFAULT FALSE |
| google_updated_at | TIMESTAMPTZ | nullable |
| created_at / updated_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() (updated touched by trigger) |

**Indexes:**
- UNIQUE (`calendar_id`, `google_event_id`) — the upsert target for every sync
- `idx_calendar_events_range` on (`calendar_id`, `starts_at`, `ends_at`)
- `idx_calendar_events_masters` on (`calendar_id`) WHERE `recurrence IS NOT NULL`
- `idx_calendar_events_master_id` on (`master_id`) WHERE NOT NULL
- `idx_calendar_events_ical_uid` on (`ical_uid`) WHERE `ical_uid <> ''`

Three kinds of row live here:

| Kind | Recognised by |
|---|---|
| one-off | `recurrence IS NULL AND recurring_event_id IS NULL` |
| series master | `recurrence IS NOT NULL` |
| override of one occurrence | `recurring_event_id IS NOT NULL` (+ `original_starts_at`) |

`starts_at`/`ends_at` are always set, so a range query is a plain B-tree scan. An all-day event
is stored as midnight-to-midnight *in `start_tz`* with an exclusive end — Google's
`start.date`/`end.date` convention pinned to a zone, which is what makes one code path work for
every range query; the original dates are recovered by rendering `starts_at` in `start_tz`.

Masters have no upper bound to index on (a series can be endless), so a range read fetches every
master of the selected calendars whose start is before the window's end and expands them in the
service.

### calendar_sync_runs

| Column | Type | Constraints |
|---|---|---|
| id | SERIAL | PRIMARY KEY |
| calendar_id | INT | nullable, FK → `calendars.id` ON DELETE CASCADE |
| trigger | TEXT | NOT NULL, CHECK in (`poll`, `webhook`, `manual`, `connect`) |
| kind | TEXT | NOT NULL, CHECK in (`full`, `incremental`) |
| started_at | TIMESTAMPTZ | NOT NULL, DEFAULT now() |
| finished_at | TIMESTAMPTZ | nullable — still NULL means it never finished |
| pages / upserted / deleted | INT | NOT NULL, DEFAULT 0 |
| error | TEXT | nullable |

**Indexes:** `idx_calendar_sync_runs_calendar` on (`calendar_id`, `started_at DESC`)

Push channels die quietly and sync tokens expire in the middle of the night. Without this table
the only evidence of either is a calendar that stopped changing.

## Relationships

```
google_accounts (1) --< (many) calendars (1) --< (many) calendar_events
                                        \--< (many) calendar_sync_runs

calendar_events (1 master) --< (many overrides)   [master_id, self-FK, ON DELETE CASCADE]
```

Every FK cascades. Disconnecting an account is meant to leave nothing behind, and an override
means nothing without its master.

## Notes

- **Why not store the expansion of a series**: an endless series has no row count, and picking
  a horizon means the calendar silently ends somewhere. The master plus its overrides is the
  complete description; the window is a read concern.
- **Why cancelled overrides are never purged**: they are the holes in a live series. Purging one
  makes the occurrence reappear. `PurgeCancelledEvents` only removes cancelled rows that have no
  `recurring_event_id`.
- **Why `updated_at` drives the purge**: it means "cancelled and untouched for 90 days". The
  touch trigger keeps it honest, which is also why a test has to disable that trigger to age a
  row.
- **Why `deleted_at` on calendars instead of a delete**: a calendar that stops being shared
  should not take a month of visible events with it without explanation, and a re-share keeps
  the local preferences.
