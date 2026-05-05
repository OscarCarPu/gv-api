CREATE TABLE IF NOT EXISTS accounts (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    currency    TEXT NOT NULL DEFAULT 'EUR' CHECK (length(currency) BETWEEN 1 AND 8),
    total       NUMERIC(15,2) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transactions (
    id              SERIAL PRIMARY KEY,
    type            TEXT NOT NULL CHECK (type IN ('income','expense','transfer')),
    amount          NUMERIC(15,2) NOT NULL CHECK (amount > 0),
    account_id      INT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    to_account_id   INT REFERENCES accounts(id) ON DELETE RESTRICT,
    description     TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (type = 'transfer' AND to_account_id IS NOT NULL AND to_account_id <> account_id)
        OR (type IN ('income','expense') AND to_account_id IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_transactions_account
    ON transactions(account_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_to_account
    ON transactions(to_account_id, occurred_at DESC)
    WHERE to_account_id IS NOT NULL;

CREATE OR REPLACE FUNCTION transactions_apply_total_fn() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP IN ('UPDATE','DELETE') THEN
        IF OLD.type = 'income' THEN
            UPDATE accounts SET total = total - OLD.amount WHERE id = OLD.account_id;
        ELSIF OLD.type = 'expense' THEN
            UPDATE accounts SET total = total + OLD.amount WHERE id = OLD.account_id;
        ELSIF OLD.type = 'transfer' THEN
            UPDATE accounts SET total = total + OLD.amount WHERE id = OLD.account_id;
            UPDATE accounts SET total = total - OLD.amount WHERE id = OLD.to_account_id;
        END IF;
    END IF;

    IF TG_OP IN ('INSERT','UPDATE') THEN
        IF NEW.type = 'income' THEN
            UPDATE accounts SET total = total + NEW.amount WHERE id = NEW.account_id;
        ELSIF NEW.type = 'expense' THEN
            UPDATE accounts SET total = total - NEW.amount WHERE id = NEW.account_id;
        ELSIF NEW.type = 'transfer' THEN
            UPDATE accounts SET total = total - NEW.amount WHERE id = NEW.account_id;
            UPDATE accounts SET total = total + NEW.amount WHERE id = NEW.to_account_id;
        END IF;
    END IF;

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS transactions_apply_total ON transactions;
CREATE TRIGGER transactions_apply_total
    AFTER INSERT OR UPDATE OR DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION transactions_apply_total_fn();
