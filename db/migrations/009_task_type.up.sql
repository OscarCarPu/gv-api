ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS task_type   TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS recurrence  INTEGER;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT chk_task_type_values
        CHECK (task_type IN ('standard', 'continuous', 'recurring'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT chk_recurrence_positive
        CHECK (recurrence IS NULL OR recurrence > 0);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE tasks ADD CONSTRAINT chk_recurrence_required
        CHECK (
            (task_type = 'recurring' AND recurrence IS NOT NULL) OR
            (task_type != 'recurring' AND recurrence IS NULL)
        );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
