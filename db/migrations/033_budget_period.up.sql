-- Yearly budgets, for expenses that come once a year or at irregular times (property tax, car
-- maintenance). A budget now has a period; each (category, period) is its own "effective from"
-- series, with the same rules as before. A yearly row's month is January 1st of its year, and it
-- is compared with the whole calendar year's actuals instead of one month's.
ALTER TABLE budgets
    ADD COLUMN IF NOT EXISTS period TEXT NOT NULL DEFAULT 'monthly'
        CHECK (period IN ('monthly', 'yearly'));

ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_yearly_january_check;
ALTER TABLE budgets
    ADD CONSTRAINT budgets_yearly_january_check
        CHECK (period = 'monthly' OR extract(month FROM month) = 1);

ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_category_id_month_key;
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_category_id_period_month_key;
ALTER TABLE budgets
    ADD CONSTRAINT budgets_category_id_period_month_key UNIQUE (category_id, period, month);
