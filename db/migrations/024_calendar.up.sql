-- Calendar: a local mirror of the user's Google calendars, editable from here.
--
-- Three levels, because that is how Google's model is shaped and flattening it would lose
-- information the sync needs: an *account* holds the OAuth grant (one per Google login), a
-- *calendar* holds the sync cursor (Google issues one syncToken per calendar, not per
-- account), and an *event* is the row that is actually mirrored.
--
-- Google is the source of truth. Writes from here go to Google first and are stored only
-- once it accepts them, so a row in calendar_events always corresponds to something that
-- exists over there.

-- One row per connected Google account. The refresh token is the long-lived secret of the
-- whole feature: it is stored encrypted (AES-256-GCM, key from GOOGLE_TOKEN_KEY) rather than
-- in plaintext, because the database is backed up and the token grants full calendar access.
CREATE TABLE IF NOT EXISTS google_accounts (
    id                      SERIAL PRIMARY KEY,
    -- The account's own address, read from Google at connect time. This is the "source
    -- account" surfaced on every event.
    email                   TEXT        NOT NULL UNIQUE,
    label                   TEXT        NOT NULL DEFAULT '',
    color                   TEXT        NOT NULL DEFAULT '',
    refresh_token           BYTEA       NOT NULL,
    -- Cached access token, also encrypted. Kept so a restart does not spend a refresh
    -- round-trip per account before the first sync.
    access_token            BYTEA,
    access_token_expires_at TIMESTAMPTZ,
    scopes                  TEXT        NOT NULL DEFAULT '',
    -- needs_reauth is set when Google answers invalid_grant: the grant is gone and only the
    -- user can restore it, so syncing stops instead of hammering a dead token.
    status                  TEXT        NOT NULL DEFAULT 'connected'
                                        CHECK (status IN ('connected', 'needs_reauth', 'revoked')),
    last_sync_at            TIMESTAMPTZ,
    last_sync_error         TEXT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per entry of an account's calendarList.
CREATE TABLE IF NOT EXISTS calendars (
    id                  SERIAL PRIMARY KEY,
    account_id          INT         NOT NULL REFERENCES google_accounts(id) ON DELETE CASCADE,
    google_calendar_id  TEXT        NOT NULL,
    summary             TEXT        NOT NULL DEFAULT '',
    description         TEXT        NOT NULL DEFAULT '',
    -- The calendar's default IANA zone. Recurring events are expanded in the event's own
    -- zone when it has one and in this one otherwise; expanding in UTC would drift by an
    -- hour across a DST boundary.
    time_zone           TEXT        NOT NULL DEFAULT 'UTC',
    background_color    TEXT        NOT NULL DEFAULT '',
    foreground_color    TEXT        NOT NULL DEFAULT '',
    color_override      TEXT        NOT NULL DEFAULT '',
    -- owner/writer are writable; reader/freeBusyReader are not, and the API rejects writes
    -- to them itself rather than letting Google answer 403 halfway through.
    access_role         TEXT        NOT NULL DEFAULT 'reader',
    is_primary          BOOLEAN     NOT NULL DEFAULT FALSE,
    -- sync_enabled is the escape hatch for calendars that are big and useless here (public
    -- holidays, birthdays): the initial import cannot be bounded by date, so the only way
    -- to not pay for one is to not sync it.
    sync_enabled        BOOLEAN     NOT NULL DEFAULT TRUE,
    visible             BOOLEAN     NOT NULL DEFAULT TRUE,
    -- NULL means the next pass does a full sync. Cleared on 410 Gone.
    sync_token          TEXT,
    sync_token_updated_at TIMESTAMPTZ,
    last_full_sync_at   TIMESTAMPTZ,
    last_sync_at        TIMESTAMPTZ,
    last_sync_error     TEXT,
    -- Push notification channel. watch_token is our own secret, echoed back by Google in
    -- X-Goog-Channel-Token, which is the only thing authenticating the webhook.
    watch_channel_id    TEXT,
    watch_resource_id   TEXT,
    watch_token         TEXT,
    watch_expires_at    TIMESTAMPTZ,
    -- The calendar disappeared from Google's calendarList. Kept (not deleted) so its events
    -- do not vanish from a view the user is looking at without explanation.
    deleted_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, google_calendar_id)
);

CREATE INDEX IF NOT EXISTS idx_calendars_account ON calendars(account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendars_watch_channel
    ON calendars(watch_channel_id) WHERE watch_channel_id IS NOT NULL;

-- One row per Google event. Recurring series are stored the way Google stores them —
-- a master carrying the RRULE plus one row per modified or cancelled instance — and expanded
-- on read. Storing the expansion instead would mean either an unbounded number of rows for
-- an infinite series or a horizon that silently truncates the calendar.
CREATE TABLE IF NOT EXISTS calendar_events (
    id                  SERIAL PRIMARY KEY,
    calendar_id         INT         NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
    google_event_id     TEXT        NOT NULL,
    -- Stable across copies of the same event in different accounts; what a cross-account
    -- de-duplication would key on.
    ical_uid            TEXT        NOT NULL DEFAULT '',
    -- Google's version marker, sent back as If-Match on every write so a change made
    -- elsewhere in the meantime is a 412 rather than a silent overwrite.
    etag                TEXT        NOT NULL DEFAULT '',
    sequence            INT         NOT NULL DEFAULT 0,
    status              TEXT        NOT NULL DEFAULT 'confirmed',
    -- default/birthday/fromGmail/workingLocation/focusTime/outOfOffice. The derived ones are
    -- not editable and the API says so up front.
    event_type          TEXT        NOT NULL DEFAULT 'default',
    summary             TEXT        NOT NULL DEFAULT '',
    description         TEXT        NOT NULL DEFAULT '',
    location            TEXT        NOT NULL DEFAULT '',
    all_day             BOOLEAN     NOT NULL DEFAULT FALSE,
    -- Resolved instants, always set, so range queries are a plain B-tree scan. An all-day
    -- event is stored as midnight-to-midnight *in start_tz* with an exclusive end, which is
    -- Google's start.date/end.date convention pinned to a zone: keeping only the instants
    -- means one code path for every range query, and the original dates are recoverable by
    -- rendering starts_at in start_tz.
    starts_at           TIMESTAMPTZ NOT NULL,
    ends_at             TIMESTAMPTZ NOT NULL,
    start_tz            TEXT        NOT NULL DEFAULT '',
    end_tz              TEXT        NOT NULL DEFAULT '',
    -- RRULE/EXDATE/RDATE lines exactly as Google returns them; re-parsed on expansion.
    recurrence          TEXT[],
    -- Set on an instance that overrides or cancels one occurrence of a series.
    recurring_event_id  TEXT,
    master_id           INT         REFERENCES calendar_events(id) ON DELETE CASCADE,
    -- Which occurrence this row overrides. Matched against the expansion, so a moved
    -- instance is still recognised as belonging to its original slot.
    original_starts_at  TIMESTAMPTZ,
    organizer_email     TEXT        NOT NULL DEFAULT '',
    creator_email       TEXT        NOT NULL DEFAULT '',
    attendees           JSONB       NOT NULL DEFAULT '[]'::jsonb,
    reminders           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    transparency        TEXT        NOT NULL DEFAULT '',
    visibility          TEXT        NOT NULL DEFAULT '',
    color_id            TEXT        NOT NULL DEFAULT '',
    html_link           TEXT        NOT NULL DEFAULT '',
    hangout_link        TEXT        NOT NULL DEFAULT '',
    created_by_gv       BOOLEAN     NOT NULL DEFAULT FALSE,
    google_updated_at   TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (calendar_id, google_event_id)
);

-- The range scan behind GET /calendar/events.
CREATE INDEX IF NOT EXISTS idx_calendar_events_range
    ON calendar_events(calendar_id, starts_at, ends_at);
-- Masters are read in full for every range query (a series has no upper bound to index on),
-- so keep them cheap to find.
CREATE INDEX IF NOT EXISTS idx_calendar_events_masters
    ON calendar_events(calendar_id) WHERE recurrence IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_calendar_events_master_id
    ON calendar_events(master_id) WHERE master_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_calendar_events_ical_uid
    ON calendar_events(ical_uid) WHERE ical_uid <> '';

-- What a sync pass did. Push channels die quietly and sync tokens expire in the middle of the
-- night; without this the only evidence of either is a calendar that stopped changing.
CREATE TABLE IF NOT EXISTS calendar_sync_runs (
    id            SERIAL PRIMARY KEY,
    calendar_id   INT         REFERENCES calendars(id) ON DELETE CASCADE,
    trigger       TEXT        NOT NULL CHECK (trigger IN ('poll', 'webhook', 'manual', 'connect')),
    kind          TEXT        NOT NULL CHECK (kind IN ('full', 'incremental')),
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at   TIMESTAMPTZ,
    pages         INT         NOT NULL DEFAULT 0,
    upserted      INT         NOT NULL DEFAULT 0,
    deleted       INT         NOT NULL DEFAULT 0,
    error         TEXT
);

CREATE INDEX IF NOT EXISTS idx_calendar_sync_runs_calendar
    ON calendar_sync_runs(calendar_id, started_at DESC);

CREATE OR REPLACE FUNCTION calendar_touch_updated_at_fn() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS google_accounts_touch_updated_at ON google_accounts;
CREATE TRIGGER google_accounts_touch_updated_at
    BEFORE UPDATE ON google_accounts
    FOR EACH ROW EXECUTE FUNCTION calendar_touch_updated_at_fn();

DROP TRIGGER IF EXISTS calendars_touch_updated_at ON calendars;
CREATE TRIGGER calendars_touch_updated_at
    BEFORE UPDATE ON calendars
    FOR EACH ROW EXECUTE FUNCTION calendar_touch_updated_at_fn();

DROP TRIGGER IF EXISTS calendar_events_touch_updated_at ON calendar_events;
CREATE TRIGGER calendar_events_touch_updated_at
    BEFORE UPDATE ON calendar_events
    FOR EACH ROW EXECUTE FUNCTION calendar_touch_updated_at_fn();
