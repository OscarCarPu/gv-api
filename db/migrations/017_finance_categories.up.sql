DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'transaction_type') THEN
        CREATE TYPE transaction_type AS ENUM ('income', 'expense', 'transfer');
    END IF;
END $$;

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_type_check;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_check;

ALTER TABLE transactions
    ALTER COLUMN type TYPE transaction_type USING type::transaction_type;

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_type_layout_check;
ALTER TABLE transactions
    ADD CONSTRAINT transactions_type_layout_check CHECK (
        (type = 'transfer'::transaction_type AND to_account_id IS NOT NULL AND to_account_id <> account_id)
        OR (type IN ('income'::transaction_type, 'expense'::transaction_type) AND to_account_id IS NULL)
    );

CREATE TABLE IF NOT EXISTS categories (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    parent_id   INT REFERENCES categories(id) ON DELETE RESTRICT,
    type        transaction_type NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX IF NOT EXISTS idx_categories_parent ON categories(parent_id);
CREATE INDEX IF NOT EXISTS idx_categories_type   ON categories(type);

ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS category_id INT REFERENCES categories(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_transactions_category
    ON transactions(category_id, occurred_at DESC);
