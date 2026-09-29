-- Monthly budgets per category.
--
-- A row is effective from its month onwards, until the next row for the same category: the
-- budget for month M is the row with the greatest month <= M. Setting a budget once therefore
-- carries it into every later month, and a one-month exception is two rows (the exception at M
-- and the previous value restored at M+1). A NULL amount ends the budget from that month on;
-- 0 is a real budget ("spend nothing here").
--
-- Only income and expense categories are budgeted (transfers net out); the API enforces it.
-- Budgets belong to their category, so deleting the category deletes them.
CREATE TABLE IF NOT EXISTS budgets (
    id          SERIAL PRIMARY KEY,
    category_id INT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    month       DATE NOT NULL CHECK (extract(day FROM month) = 1),
    amount      NUMERIC(15,2) CHECK (amount IS NULL OR amount >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (category_id, month)
);
