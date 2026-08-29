ALTER TABLE tasks
  ADD COLUMN estimate_hours numeric(5, 2) CHECK (estimate_hours IS NULL
    OR estimate_hours > 0);
