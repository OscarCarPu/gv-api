ALTER TABLE weed_varieties
    ADD COLUMN judge TEXT NOT NULL DEFAULT 'Oscar',
    ADD COLUMN deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_weed_varieties_alive
    ON weed_varieties(score DESC, price ASC)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_weed_varieties_score_price;

CREATE TABLE IF NOT EXISTS weed_varieties_history (
    history_id   BIGSERIAL PRIMARY KEY,
    variety_id   INT NOT NULL,
    op           TEXT NOT NULL CHECK (op IN ('INSERT', 'UPDATE', 'DELETE')),
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    name         TEXT,
    scent        REAL,
    flavor       REAL,
    power        REAL,
    quality      REAL,
    score        REAL,
    price        REAL,
    comments     TEXT,
    judge        TEXT,
    deleted_at   TIMESTAMPTZ,

    actor_ip          TEXT,
    actor_user_agent  TEXT,
    actor_device_id   TEXT,
    actor_token_kind  TEXT
);

CREATE INDEX IF NOT EXISTS idx_weed_varieties_history_variety
    ON weed_varieties_history(variety_id, changed_at DESC);

CREATE OR REPLACE FUNCTION weed_varieties_audit_fn() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    v weed_varieties%ROWTYPE;
    op_name text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v := OLD;
        op_name := 'DELETE';
    ELSE
        v := NEW;
        op_name := TG_OP;
    END IF;

    INSERT INTO weed_varieties_history (
        variety_id, op,
        name, scent, flavor, power, quality, score, price, comments, judge, deleted_at,
        actor_ip, actor_user_agent, actor_device_id, actor_token_kind
    ) VALUES (
        v.id, op_name,
        v.name, v.scent, v.flavor, v.power, v.quality, v.score, v.price, v.comments, v.judge, v.deleted_at,
        NULLIF(current_setting('app.actor_ip', true), ''),
        NULLIF(current_setting('app.actor_user_agent', true), ''),
        NULLIF(current_setting('app.actor_device_id', true), ''),
        NULLIF(current_setting('app.actor_token_kind', true), '')
    );

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS weed_varieties_audit ON weed_varieties;
CREATE TRIGGER weed_varieties_audit
    AFTER INSERT OR UPDATE OR DELETE ON weed_varieties
    FOR EACH ROW EXECUTE FUNCTION weed_varieties_audit_fn();

INSERT INTO weed_varieties_history (
    variety_id, op, changed_at,
    name, scent, flavor, power, quality, score, price, comments, judge, deleted_at,
    actor_token_kind
)
SELECT id, 'INSERT', now(),
    name, scent, flavor, power, quality, score, price, comments, judge, deleted_at,
    'backfill'
FROM weed_varieties;
