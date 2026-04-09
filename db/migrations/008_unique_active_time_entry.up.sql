-- Only one active (unfinished) time entry can exist at a time.
CREATE UNIQUE INDEX idx_time_entries_one_active
ON time_entries ((true))
WHERE finished_at IS NULL;
