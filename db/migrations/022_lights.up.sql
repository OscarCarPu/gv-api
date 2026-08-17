-- Bulbs used to live in a LIGHTS env var, which meant adding one to the house was an edit to
-- a JSON string and a redeploy. They are furniture, not configuration: they get added, named,
-- moved and thrown away, and that belongs in the database with everything else that changes.
--
-- The id is a slug of the name rather than an identity column: it is what appears in URLs
-- (/domotics/lights/bedroom), in logs and as the key three clients hold their local state
-- under, and a name is far easier to read there than a number. It is assigned once, at
-- creation, so renaming a bulb never breaks a client that is mid-command.
CREATE TABLE IF NOT EXISTS lights (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    -- The BLE address. Unique because adding the same bulb twice gives two cards that fight
    -- over one radio link.
    address TEXT NOT NULL UNIQUE,
    -- Which wire protocol drives it; see internal/lights/protocol.go.
    protocol TEXT NOT NULL,
    supports_color BOOLEAN NOT NULL DEFAULT TRUE,
    supports_color_temp BOOLEAN NOT NULL DEFAULT TRUE,
    min_color_temp DOUBLE PRECISION NOT NULL DEFAULT 2200,
    max_color_temp DOUBLE PRECISION NOT NULL DEFAULT 6500,
    -- Per-bulb quirks a protocol may read (characteristic UUIDs, keys), so one odd lamp is a
    -- row rather than a new protocol.
    options JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
