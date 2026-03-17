ALTER TABLE habits
    ADD COLUMN IF NOT EXISTS frequency TEXT NOT NULL DEFAULT 'daily'
        CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    ADD COLUMN IF NOT EXISTS objective REAL,
    ADD COLUMN IF NOT EXISTS current_streak INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS longest_streak INTEGER NOT NULL DEFAULT 0;
