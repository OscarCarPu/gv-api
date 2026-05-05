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

`total` is denormalized but write-protected: the API never accepts it as input, and the only writes come from the `transactions_apply_total` trigger.

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

`category_id` is nullable in the schema for forward compatibility with bulk imports, but the API requires it on every Create and Update. The category's `type` must match the transaction's `type`; this is checked by the service layer with a `SELECT type FROM categories WHERE id = $1` lookup before each write, returning 400 on mismatch.

**Indexes:**
- `idx_transactions_account` on `(account_id, occurred_at DESC)`.
- `idx_transactions_to_account` on `(to_account_id, occurred_at DESC) WHERE to_account_id IS NOT NULL`.
- `idx_transactions_category` on `(category_id, occurred_at DESC)`.

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

Since the trigger fires per row inside the same transaction as the write, account totals can never desync from the transaction history. Concurrent writers serialize on the row-level lock taken by the `UPDATE accounts ...` statement.

## Notes

- All money values use `NUMERIC(15,2)` end-to-end. In Go they are `github.com/shopspring/decimal.Decimal` (configured via sqlc override) and serialized as JSON strings.
- DELETE of an account is hard but blocked by `ON DELETE RESTRICT` if any transaction still references it. Delete the transactions first (which the trigger will then reverse from `total`), then delete the account.
- DELETE of a category is also `ON DELETE RESTRICT` against both `transactions.category_id` and `categories.parent_id` self-references.
- There is no soft delete and no audit history table on this feature — unlike `weed_varieties`. If audit becomes a requirement, the pattern from `015_weed_varieties_audit.up.sql` can be lifted onto these tables.
- Accounts are currency-agnostic: amounts are bare `NUMERIC(15,2)` values with no currency tagging. Mixing currencies on the same instance is up to the user.
