-- EnsureRecurringBlocks checks "does a row already exist for (commitment, date)" then inserts —
-- a plain check-then-insert, so two concurrent range reads covering the same date can both pass
-- the check before either commits, producing two blocks for the same commitment occurrence.
-- This constraint turns that into a safe no-op at the database level (see the generation query).
CREATE UNIQUE INDEX IF NOT EXISTS plan_blocks_commitment_date_uidx
  ON plan_blocks (commitment_id, plan_date)
  WHERE commitment_id IS NOT NULL;
