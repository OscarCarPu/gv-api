CREATE TABLE IF NOT EXISTS weed_varieties (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    scent REAL NOT NULL CHECK (scent >= 0 AND scent <= 10),
    flavor REAL NOT NULL CHECK (flavor >= 0 AND flavor <= 10),
    power REAL NOT NULL CHECK (power >= 0 AND power <= 10),
    quality REAL NOT NULL CHECK (quality >= 0 AND quality <= 10),
    score REAL NOT NULL GENERATED ALWAYS AS ((scent + flavor + power + quality) / 4.0) STORED,
    price REAL NOT NULL,
    comments TEXT
);

CREATE INDEX IF NOT EXISTS idx_weed_varieties_score_price ON weed_varieties(score DESC, price ASC);
