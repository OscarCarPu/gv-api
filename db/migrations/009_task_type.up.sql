ALTER TABLE tasks
    ADD COLUMN task_type   TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN recurrence  INTEGER;

ALTER TABLE tasks
    ADD CONSTRAINT chk_task_type_values
        CHECK (task_type IN ('standard', 'continuous', 'recurring')),
    ADD CONSTRAINT chk_recurrence_positive
        CHECK (recurrence IS NULL OR recurrence > 0),
    ADD CONSTRAINT chk_recurrence_required
        CHECK (
            (task_type = 'recurring' AND recurrence IS NOT NULL) OR
            (task_type != 'recurring' AND recurrence IS NULL)
        );
