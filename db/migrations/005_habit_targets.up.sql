ALTER TABLE habits
    ADD COLUMN IF NOT EXISTS target_min REAL,
    ADD COLUMN IF NOT EXISTS target_max REAL,
    ADD COLUMN IF NOT EXISTS recording_required BOOLEAN NOT NULL DEFAULT true;

UPDATE habits SET target_min = objective WHERE objective IS NOT NULL;

ALTER TABLE habits DROP COLUMN IF EXISTS objective;
