CREATE TABLE IF NOT EXISTS plan_blocks (
    id          SERIAL PRIMARY KEY,
    plan_date   DATE        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL,
    task_id     INT         REFERENCES tasks(id) ON DELETE SET NULL,
    label       TEXT        NOT NULL CHECK (length(label) BETWEEN 1 AND 200),
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_at > started_at)
);

CREATE INDEX IF NOT EXISTS idx_plan_blocks_date
    ON plan_blocks(plan_date, started_at);

CREATE INDEX IF NOT EXISTS idx_plan_blocks_task
    ON plan_blocks(task_id)
    WHERE task_id IS NOT NULL;

CREATE OR REPLACE FUNCTION plan_blocks_touch_updated_at_fn() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS plan_blocks_touch_updated_at ON plan_blocks;
CREATE TRIGGER plan_blocks_touch_updated_at
    BEFORE UPDATE ON plan_blocks
    FOR EACH ROW EXECUTE FUNCTION plan_blocks_touch_updated_at_fn();
