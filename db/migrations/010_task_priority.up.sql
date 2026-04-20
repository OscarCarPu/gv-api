ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 3;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT chk_priority_range
        CHECK (priority BETWEEN 1 AND 5);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
