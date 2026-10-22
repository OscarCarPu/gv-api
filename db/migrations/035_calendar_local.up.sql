-- Calendar stops mirroring Google: gv is the only interface. An event is just a name and a
-- time range. Recurring series, calendars, accounts and sync state are gone; only one-off
-- events survive.
DELETE FROM calendar_events WHERE recurrence IS NOT NULL OR master_id IS NOT NULL OR status = 'cancelled';

-- Plan blocks linked to a single occurrence of a series (12@...) no longer point at anything.
UPDATE plan_blocks SET event_ref = NULL WHERE event_ref LIKE '%@%';

DROP TABLE calendar_sync_runs;
DROP TABLE google_accounts CASCADE;

ALTER TABLE calendar_events
    DROP COLUMN calendar_id,
    DROP COLUMN google_event_id,
    DROP COLUMN ical_uid,
    DROP COLUMN etag,
    DROP COLUMN sequence,
    DROP COLUMN status,
    DROP COLUMN event_type,
    DROP COLUMN description,
    DROP COLUMN location,
    DROP COLUMN all_day,
    DROP COLUMN start_tz,
    DROP COLUMN end_tz,
    DROP COLUMN recurrence,
    DROP COLUMN recurring_event_id,
    DROP COLUMN master_id,
    DROP COLUMN original_starts_at,
    DROP COLUMN organizer_email,
    DROP COLUMN creator_email,
    DROP COLUMN attendees,
    DROP COLUMN reminders,
    DROP COLUMN transparency,
    DROP COLUMN visibility,
    DROP COLUMN color_id,
    DROP COLUMN html_link,
    DROP COLUMN hangout_link,
    DROP COLUMN created_by_gv,
    DROP COLUMN google_updated_at;

DROP TABLE calendars CASCADE;

ALTER TABLE calendar_events RENAME COLUMN summary TO name;
ALTER TABLE calendar_events RENAME COLUMN starts_at TO start_time;
ALTER TABLE calendar_events RENAME COLUMN ends_at TO end_time;

CREATE INDEX IF NOT EXISTS idx_calendar_events_time ON calendar_events(start_time, end_time);
