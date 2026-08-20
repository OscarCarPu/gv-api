-- ---------------------------------------------------------------------------
-- Accounts
-- ---------------------------------------------------------------------------

-- name: UpsertGoogleAccount :one
-- Re-connecting an already connected account replaces its tokens instead of failing: the
-- user re-runs the consent flow precisely when the old grant stopped working.
INSERT INTO google_accounts (email, refresh_token, access_token, access_token_expires_at, scopes, status)
VALUES (@email, @refresh_token, @access_token, @access_token_expires_at, @scopes, 'connected')
ON CONFLICT (email) DO UPDATE
SET refresh_token = EXCLUDED.refresh_token,
    access_token = EXCLUDED.access_token,
    access_token_expires_at = EXCLUDED.access_token_expires_at,
    scopes = EXCLUDED.scopes,
    status = 'connected',
    last_sync_error = NULL
RETURNING *;

-- name: GetGoogleAccount :one
SELECT * FROM google_accounts WHERE id = $1;

-- name: ListGoogleAccounts :many
SELECT * FROM google_accounts ORDER BY email;

-- name: UpdateGoogleAccountAccessToken :exec
UPDATE google_accounts
SET access_token = @access_token, access_token_expires_at = @access_token_expires_at
WHERE id = @id;

-- name: UpdateGoogleAccountStatus :exec
UPDATE google_accounts SET status = @status, last_sync_error = @last_sync_error WHERE id = @id;

-- name: UpdateGoogleAccountMeta :one
UPDATE google_accounts
SET label = COALESCE(sqlc.narg(label), label),
    color = COALESCE(sqlc.narg(color), color)
WHERE id = @id
RETURNING *;

-- name: TouchGoogleAccountSync :exec
UPDATE google_accounts SET last_sync_at = now(), last_sync_error = @last_sync_error WHERE id = @id;

-- name: DeleteGoogleAccount :execrows
DELETE FROM google_accounts WHERE id = $1;

-- ---------------------------------------------------------------------------
-- Calendars
-- ---------------------------------------------------------------------------

-- name: UpsertCalendar :one
-- Google's metadata (name, colors, role) is refreshed on every calendarList pass; the local
-- preferences (sync_enabled, visible, color_override) deliberately are not.
INSERT INTO calendars (account_id, google_calendar_id, summary, description, time_zone,
                       background_color, foreground_color, access_role, is_primary, sync_enabled)
VALUES (@account_id, @google_calendar_id, @summary, @description, @time_zone,
        @background_color, @foreground_color, @access_role, @is_primary, @sync_enabled)
ON CONFLICT (account_id, google_calendar_id) DO UPDATE
SET summary = EXCLUDED.summary,
    description = EXCLUDED.description,
    time_zone = EXCLUDED.time_zone,
    background_color = EXCLUDED.background_color,
    foreground_color = EXCLUDED.foreground_color,
    access_role = EXCLUDED.access_role,
    is_primary = EXCLUDED.is_primary,
    deleted_at = NULL
RETURNING *;

-- name: GetCalendar :one
SELECT * FROM calendars WHERE id = $1;

-- name: GetCalendarByGoogleID :one
SELECT * FROM calendars WHERE account_id = @account_id AND google_calendar_id = @google_calendar_id;

-- name: GetCalendarByChannelID :one
SELECT * FROM calendars WHERE watch_channel_id = $1;

-- name: ListCalendars :many
SELECT c.*, a.email AS account_email, a.label AS account_label, a.color AS account_color,
       a.status AS account_status
FROM calendars c
JOIN google_accounts a ON a.id = c.account_id
ORDER BY a.email, c.is_primary DESC, c.summary;

-- name: ListSyncableCalendars :many
SELECT c.* FROM calendars c
JOIN google_accounts a ON a.id = c.account_id
WHERE c.sync_enabled AND c.deleted_at IS NULL AND a.status = 'connected'
ORDER BY c.id;

-- name: ListCalendarsByAccount :many
SELECT * FROM calendars WHERE account_id = $1 ORDER BY is_primary DESC, summary;

-- name: UpdateCalendarPrefs :one
UPDATE calendars
SET sync_enabled = COALESCE(sqlc.narg(sync_enabled), sync_enabled),
    visible = COALESCE(sqlc.narg(visible), visible),
    color_override = COALESCE(sqlc.narg(color_override), color_override)
WHERE id = @id
RETURNING *;

-- name: SetCalendarSyncToken :exec
UPDATE calendars
SET sync_token = @sync_token, sync_token_updated_at = now(), last_sync_at = now(),
    last_sync_error = NULL,
    last_full_sync_at = CASE WHEN @was_full::boolean THEN now() ELSE last_full_sync_at END
WHERE id = @id;

-- name: ClearCalendarSyncToken :exec
UPDATE calendars SET sync_token = NULL, sync_token_updated_at = NULL WHERE id = $1;

-- name: ClearAccountSyncTokens :exec
UPDATE calendars SET sync_token = NULL, sync_token_updated_at = NULL WHERE account_id = $1;

-- name: SetCalendarSyncError :exec
UPDATE calendars SET last_sync_error = @last_sync_error, last_sync_at = now() WHERE id = @id;

-- name: MarkCalendarsDeleted :exec
-- Calendars that were not seen in the latest calendarList pass for this account.
UPDATE calendars
SET deleted_at = now()
WHERE account_id = @account_id
  AND deleted_at IS NULL
  AND NOT (google_calendar_id = ANY(@seen_ids::text[]));

-- name: SetCalendarWatch :exec
UPDATE calendars
SET watch_channel_id = @watch_channel_id, watch_resource_id = @watch_resource_id,
    watch_token = @watch_token, watch_expires_at = @watch_expires_at
WHERE id = @id;

-- name: ClearCalendarWatch :exec
UPDATE calendars
SET watch_channel_id = NULL, watch_resource_id = NULL, watch_token = NULL, watch_expires_at = NULL
WHERE id = $1;

-- name: ListCalendarsNeedingWatch :many
-- No channel at all, or one close enough to expiry that it must be replaced now. A channel
-- cannot be renewed in place: watch() again, then stop the old one.
SELECT c.* FROM calendars c
JOIN google_accounts a ON a.id = c.account_id
WHERE c.sync_enabled AND c.deleted_at IS NULL AND a.status = 'connected'
  AND (c.watch_channel_id IS NULL OR c.watch_expires_at IS NULL OR c.watch_expires_at < @renew_before)
ORDER BY c.id;

-- name: ListWatchedCalendars :many
SELECT * FROM calendars WHERE watch_channel_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Events
-- ---------------------------------------------------------------------------

-- name: UpsertCalendarEvent :one
INSERT INTO calendar_events (
    calendar_id, google_event_id, ical_uid, etag, sequence, status, event_type,
    summary, description, location, all_day, starts_at, ends_at, start_tz, end_tz,
    recurrence, recurring_event_id, original_starts_at, organizer_email, creator_email,
    attendees, reminders, transparency, visibility, color_id, html_link, hangout_link,
    created_by_gv, google_updated_at
) VALUES (
    @calendar_id, @google_event_id, @ical_uid, @etag, @sequence, @status, @event_type,
    @summary, @description, @location, @all_day, @starts_at, @ends_at, @start_tz, @end_tz,
    @recurrence, @recurring_event_id, @original_starts_at, @organizer_email, @creator_email,
    @attendees, @reminders, @transparency, @visibility, @color_id, @html_link, @hangout_link,
    @created_by_gv, @google_updated_at
)
ON CONFLICT (calendar_id, google_event_id) DO UPDATE
SET ical_uid = EXCLUDED.ical_uid,
    etag = EXCLUDED.etag,
    sequence = EXCLUDED.sequence,
    status = EXCLUDED.status,
    event_type = EXCLUDED.event_type,
    summary = EXCLUDED.summary,
    description = EXCLUDED.description,
    location = EXCLUDED.location,
    all_day = EXCLUDED.all_day,
    starts_at = EXCLUDED.starts_at,
    ends_at = EXCLUDED.ends_at,
    start_tz = EXCLUDED.start_tz,
    end_tz = EXCLUDED.end_tz,
    recurrence = EXCLUDED.recurrence,
    recurring_event_id = EXCLUDED.recurring_event_id,
    original_starts_at = EXCLUDED.original_starts_at,
    organizer_email = EXCLUDED.organizer_email,
    creator_email = EXCLUDED.creator_email,
    attendees = EXCLUDED.attendees,
    reminders = EXCLUDED.reminders,
    transparency = EXCLUDED.transparency,
    visibility = EXCLUDED.visibility,
    color_id = EXCLUDED.color_id,
    html_link = EXCLUDED.html_link,
    hangout_link = EXCLUDED.hangout_link,
    created_by_gv = calendar_events.created_by_gv OR EXCLUDED.created_by_gv,
    google_updated_at = EXCLUDED.google_updated_at
RETURNING *;

-- name: GetCalendarEvent :one
SELECT * FROM calendar_events WHERE id = $1;

-- name: GetCalendarEventByGoogleID :one
SELECT * FROM calendar_events WHERE calendar_id = @calendar_id AND google_event_id = @google_event_id;

-- name: DeleteCalendarEvent :execrows
DELETE FROM calendar_events WHERE id = $1;

-- name: DeleteCalendarEventByGoogleID :execrows
DELETE FROM calendar_events WHERE calendar_id = @calendar_id AND google_event_id = @google_event_id;

-- name: DeleteCalendarEventsForCalendar :execrows
-- Used on 410 Gone: the client store for that calendar is wiped and rebuilt from scratch.
DELETE FROM calendar_events WHERE calendar_id = $1;

-- name: LinkCalendarEventMasters :exec
-- An exception can arrive in an earlier page than its master, so the link is resolved after
-- each pass rather than per row.
UPDATE calendar_events e
SET master_id = m.id
FROM calendar_events m
WHERE e.calendar_id = @calendar_id
  AND e.recurring_event_id IS NOT NULL
  AND e.master_id IS NULL
  AND m.calendar_id = e.calendar_id
  AND m.google_event_id = e.recurring_event_id;

-- name: ListEventsInRange :many
-- One-off events (and single instances of nothing) that overlap the window. Masters and
-- their exceptions are fetched separately because a series has no end to compare against.
SELECT * FROM calendar_events
WHERE calendar_id = ANY(@calendar_ids::int[])
  AND recurrence IS NULL
  AND recurring_event_id IS NULL
  AND status <> 'cancelled'
  AND starts_at < @range_end
  AND (ends_at > @range_start OR (ends_at = starts_at AND starts_at >= @range_start))
ORDER BY starts_at;

-- name: ListRecurringMasters :many
SELECT * FROM calendar_events
WHERE calendar_id = ANY(@calendar_ids::int[])
  AND recurrence IS NOT NULL
  AND status <> 'cancelled'
  AND starts_at < @range_end
ORDER BY starts_at;

-- name: ListEventExceptions :many
SELECT * FROM calendar_events
WHERE master_id = ANY(@master_ids::int[])
ORDER BY original_starts_at;

-- name: ListEventExceptionsByMaster :many
SELECT * FROM calendar_events WHERE master_id = $1 ORDER BY original_starts_at;

-- name: PurgeCancelledEvents :execrows
-- A cancelled one-off is of no interest once it is old; a cancelled *instance* is a hole in
-- a live series and must outlive it, so it is never purged here.
DELETE FROM calendar_events
WHERE status = 'cancelled' AND recurring_event_id IS NULL AND updated_at < @older_than;

-- name: CountCalendarEvents :one
SELECT count(*) FROM calendar_events WHERE calendar_id = $1;

-- ---------------------------------------------------------------------------
-- Sync runs
-- ---------------------------------------------------------------------------

-- name: CreateSyncRun :one
INSERT INTO calendar_sync_runs (calendar_id, trigger, kind)
VALUES (@calendar_id, @trigger, @kind)
RETURNING *;

-- name: FinishSyncRun :exec
UPDATE calendar_sync_runs
SET finished_at = now(), pages = @pages, upserted = @upserted, deleted = @deleted, error = @error
WHERE id = @id;

-- name: ListRecentSyncRuns :many
SELECT * FROM calendar_sync_runs ORDER BY started_at DESC LIMIT $1;

-- name: ListOrphanOverridesInRange :many
-- Overrides whose master has not been synced (or no longer exists). They are rare — the link
-- is resolved after every pass — but without this they would be invisible rather than merely
-- detached, and an invisible event is worse than an odd-looking one.
SELECT * FROM calendar_events
WHERE calendar_id = ANY(@calendar_ids::int[])
  AND recurring_event_id IS NOT NULL
  AND master_id IS NULL
  AND status <> 'cancelled'
  AND starts_at < @range_end
  AND (ends_at > @range_start OR (ends_at = starts_at AND starts_at >= @range_start))
ORDER BY starts_at;
