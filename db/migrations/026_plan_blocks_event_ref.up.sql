ALTER TABLE plan_blocks
  ADD COLUMN event_ref text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_plan_blocks_event_ref ON plan_blocks (event_ref)
WHERE
  event_ref IS NOT NULL;
