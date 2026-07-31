-- LLM usage/cost ledger for the "Voz" assistant. Stores no request text —
-- only model, phase, token counts, and computed cost — so there is nothing
-- sensitive to leak. cost_usd is NUMERIC(12,6) to hold sub-cent costs exactly.
CREATE TABLE IF NOT EXISTS assistant_usage (
    id                 INTEGER PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model              TEXT NOT NULL,
    phase              TEXT NOT NULL CHECK (phase IN ('decide', 'summarize')),
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    cost_usd           NUMERIC(12, 6) NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_assistant_usage_created_at ON assistant_usage (created_at);
