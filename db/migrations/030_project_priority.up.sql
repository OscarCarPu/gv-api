-- A project's priority is the default for tasks created in it without an explicit one.
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS priority INTEGER NOT NULL DEFAULT 3;

DO $$ BEGIN
    ALTER TABLE projects ADD CONSTRAINT chk_project_priority_range
        CHECK (priority BETWEEN 1 AND 5);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;
