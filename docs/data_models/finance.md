# Finance - Data Models

## Enum

### transaction_type

```sql
CREATE TYPE transaction_type AS ENUM ('income', 'expense', 'transfer');
```

Used by both `transactions.type` and `categories.type`. The API rejects any transaction whose category has a mismatched type (e.g. an `income` transaction tagged with an `expense` category).

## Tables

### accounts

| Column     | Type            | Constraints                                                       |
|------------|-----------------|-------------------------------------------------------------------|
| id         | SERIAL          | PRIMARY KEY                                                       |
| name       | TEXT            | NOT NULL, CHECK (`length(name) BETWEEN 1 AND 40`)                 |
| total      | NUMERIC(15,2)   | NOT NULL, DEFAULT `0`. Maintained by trigger on `transactions`.   |
| created_at | TIMESTAMPTZ     | NOT NULL, DEFAULT `now()`                                         |

`total` is denormalized but write-protected at the API level: the HTTP layer never accepts it as input. The only writes from the application come from the `transactions_apply_total` trigger.

**Opening balances**: no `initial_balance` column. Seed `total` directly via SQL; the trigger maintains it from there. This keeps fake "opening balance" income out of the flow stats while the net-worth series still anchors on it.

### categories

| Column     | Type             | Constraints                                                                  |
|------------|------------------|------------------------------------------------------------------------------|
| id         | SERIAL           | PRIMARY KEY                                                                  |
| name       | TEXT             | NOT NULL, CHECK (`length(name) BETWEEN 1 AND 40`)                            |
| parent_id  | INT              | nullable, REFERENCES `categories(id)` ON DELETE RESTRICT                     |
| type       | transaction_type | NOT NULL                                                                     |
| created_at | TIMESTAMPTZ      | NOT NULL, DEFAULT `now()`                                                    |
|            |                  | CHECK (`parent_id IS NULL OR parent_id <> id`)                               |

**Indexes:**
- `idx_categories_parent` on `(parent_id)`.
- `idx_categories_type` on `(type)`.

The hierarchy depth is unconstrained at the schema level. The seed data uses two levels (root + leaves). The schema does not enforce that a child's `type` matches its parent's `type` — that is a convention upheld in seed data and the UI.

### transactions

| Column          | Type              | Constraints                                                            |
|-----------------|-------------------|------------------------------------------------------------------------|
| id              | SERIAL            | PRIMARY KEY                                                            |
| type            | transaction_type  | NOT NULL                                                               |
| amount          | NUMERIC(15,2)     | NOT NULL, CHECK (`amount > 0`)                                         |
| account_id      | INT               | NOT NULL, REFERENCES `accounts(id)` ON DELETE RESTRICT                 |
| to_account_id   | INT               | nullable, REFERENCES `accounts(id)` ON DELETE RESTRICT                 |
| category_id     | INT               | nullable, REFERENCES `categories(id)` ON DELETE RESTRICT (API-required)|
| description     | TEXT              | nullable                                                               |
| occurred_at     | TIMESTAMPTZ       | NOT NULL, DEFAULT `now()`                                              |
| created_at      | TIMESTAMPTZ       | NOT NULL, DEFAULT `now()`                                              |

Row-level CHECK (`transactions_type_layout_check`):

```
(type = 'transfer' AND to_account_id IS NOT NULL AND to_account_id <> account_id)
OR (type IN ('income','expense') AND to_account_id IS NULL)
```

`category_id` is nullable in the schema (for bulk imports) but required by the API, and its `type` must match the transaction's (checked in the service, 400 on mismatch).

**Indexes:**
- `idx_transactions_account` on `(account_id, occurred_at DESC)`.
- `idx_transactions_to_account` on `(to_account_id, occurred_at DESC) WHERE to_account_id IS NOT NULL`.
- `idx_transactions_category` on `(category_id, occurred_at DESC)`.

### budgets

| Column      | Type          | Constraints                                                          |
|-------------|---------------|----------------------------------------------------------------------|
| id          | SERIAL        | PRIMARY KEY                                                          |
| category_id | INT           | NOT NULL, REFERENCES `categories(id)` ON DELETE CASCADE              |
| period      | TEXT          | NOT NULL, DEFAULT `'monthly'`, CHECK (`monthly` or `yearly`)         |
| month       | DATE          | NOT NULL, CHECK (first day of the month; January 1st when yearly)    |
| amount      | NUMERIC(15,2) | nullable, CHECK (`amount IS NULL OR amount >= 0`)                    |
| created_at  | TIMESTAMPTZ   | NOT NULL, DEFAULT `now()`                                            |
|             |               | UNIQUE (`category_id`, `period`, `month`)                            |

A row means "from this period on, the budget is `amount`": the budget of a period in effect at
month M is the row of that (category, period) with the greatest `month <= M`. Yearly rows sit on
January 1st, so the same lookup picks the right year. `NULL` ends the budget from that period;
`0` is a real budget. A one-period exception is two rows (the exception, and the previous value
restored in the next period).

Writes run in one transaction and finish with `CollapseBudgets`, which deletes rows of the series
that change nothing (equal to the previous row, or a leading `NULL`), so each series is always
the minimal list of changes. Deleting a category deletes its budgets instead of blocking with
409. Transfer categories are rejected by the API, not the schema.

## Trigger: `transactions_apply_total`

`AFTER INSERT OR UPDATE OR DELETE ON transactions FOR EACH ROW`. The handler function `transactions_apply_total_fn`:

- On **INSERT** — applies `NEW`.
- On **DELETE** — reverses `OLD`.
- On **UPDATE** — reverses `OLD` then applies `NEW`. This is correct even when `account_id`, `to_account_id`, `type`, or `amount` changes.

Per type:

| Type     | Effect                                                              |
|----------|---------------------------------------------------------------------|
| income   | `accounts[account_id].total += amount`                              |
| expense  | `accounts[account_id].total -= amount`                              |
| transfer | `accounts[account_id].total -= amount; accounts[to_account_id].total += amount` |

The trigger runs in the writing transaction, so totals never desync; concurrent writers serialize on the account row lock.

## Stats queries

Read-only queries in `db/queries/finance.sql` behind `/finance/stats/*`:

- **`GetNetWorthSeries(from, to, granularity)`** — buckets aligned to `date_trunc(granularity, from)`; each is `SUM(accounts.total)` minus the income/expense deltas after the bucket ends.
- **`GetCategoryStats(type, from, to, account_id?)`** — `SUM`, `COUNT` and `share` per category; `account_id` matches either end of a transfer.
- **`GetMonthlyStats(from, to, account_id?, category_id?)`** — income and expense per `YYYY-MM`; transfers excluded.
- **`GetEarliestTransactionDate()`** — default `from` for "all time".

Defaults: `to` = now, `from` = earliest transaction (or now − 6 months), `granularity` = `day`.

## Notes

- All money values use `NUMERIC(15,2)` end-to-end. In Go they are `github.com/shopspring/decimal.Decimal` (configured via sqlc override) and serialized as JSON strings.
- DELETE of an account is hard but blocked by `ON DELETE RESTRICT` if any transaction still references it. Delete the transactions first (which the trigger will then reverse from `total`), then delete the account.
- DELETE of a category is also `ON DELETE RESTRICT` against both `transactions.category_id` and `categories.parent_id` self-references.
- There is no soft delete and no audit history table on this feature.
- Accounts are currency-agnostic: amounts are bare `NUMERIC(15,2)` values with no currency tagging. Mixing currencies on the same instance is up to the user.
