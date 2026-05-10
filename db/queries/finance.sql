-- name: GetAccount :one
SELECT id, name, total, created_at
FROM accounts
WHERE id = $1;

-- name: ListAccounts :many
SELECT id, name, total, created_at
FROM accounts
ORDER BY name ASC;

-- name: CreateAccount :one
INSERT INTO accounts (name)
VALUES ($1)
RETURNING id, name, total, created_at;

-- name: UpdateAccount :one
UPDATE accounts
SET name = $2
WHERE id = $1
RETURNING id, name, total, created_at;

-- name: DeleteAccount :exec
DELETE FROM accounts
WHERE id = $1;

-- name: GetCategory :one
SELECT id, name, parent_id, type, created_at
FROM categories
WHERE id = $1;

-- name: GetCategoryType :one
SELECT type
FROM categories
WHERE id = $1;

-- name: ListCategories :many
SELECT id, name, parent_id, type, created_at
FROM categories
ORDER BY type ASC, name ASC;

-- name: CreateCategory :one
INSERT INTO categories (name, parent_id, type)
VALUES ($1, $2, $3)
RETURNING id, name, parent_id, type, created_at;

-- name: UpdateCategory :one
UPDATE categories
SET name = $2, parent_id = $3, type = $4
WHERE id = $1
RETURNING id, name, parent_id, type, created_at;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = $1;

-- name: GetTransaction :one
SELECT id, type, amount, account_id, to_account_id, description, occurred_at, created_at, category_id
FROM transactions
WHERE id = $1;

-- name: ListTransactions :many
SELECT id, type, amount, account_id, to_account_id, description, occurred_at, created_at, category_id
FROM transactions
ORDER BY occurred_at DESC, id DESC;

-- name: ListTransactionsByAccount :many
SELECT id, type, amount, account_id, to_account_id, description, occurred_at, created_at, category_id
FROM transactions
WHERE account_id = $1 OR to_account_id = $1
ORDER BY occurred_at DESC, id DESC;

-- name: CreateTransaction :one
INSERT INTO transactions (type, amount, account_id, to_account_id, category_id, description, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, COALESCE(sqlc.narg('occurred_at')::timestamptz, now()))
RETURNING id, type, amount, account_id, to_account_id, description, occurred_at, created_at, category_id;

-- name: UpdateTransaction :one
UPDATE transactions
SET type = $2, amount = $3, account_id = $4, to_account_id = $5, category_id = $6, description = $7, occurred_at = $8
WHERE id = $1
RETURNING id, type, amount, account_id, to_account_id, description, occurred_at, created_at, category_id;

-- name: DeleteTransaction :exec
DELETE FROM transactions
WHERE id = $1;

-- name: GetAccountsTotal :one
SELECT COALESCE(SUM(total), 0::numeric)::numeric AS total
FROM accounts;

-- name: GetMonthlyTotals :one
SELECT
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'::transaction_type),  0::numeric)::numeric AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'::transaction_type), 0::numeric)::numeric AS expense
FROM transactions
WHERE occurred_at >= $1;

-- name: GetEarliestTransactionDate :one
SELECT MIN(occurred_at)::timestamptz AS earliest FROM transactions;

-- name: ListRecentTransactions :many
SELECT
    t.id,
    t.type,
    t.amount,
    a.name  AS account_name,
    ta.name AS to_account_name,
    c.name  AS category_name,
    t.description,
    t.occurred_at
FROM transactions t
JOIN accounts a       ON a.id  = t.account_id
LEFT JOIN accounts ta ON ta.id = t.to_account_id
LEFT JOIN categories c ON c.id = t.category_id
WHERE t.occurred_at >= $1
ORDER BY t.occurred_at DESC, t.id DESC;

-- name: GetNetWorthSeries :many
-- Reconstructs net worth at the end of each period (day/week/month) by walking
-- back from the current accounts.total snapshot.
-- Granularity must be one of 'day' | 'week' | 'month'.
WITH
current_total AS (
    SELECT COALESCE(SUM(total), 0)::numeric AS total FROM accounts
),
buckets AS (
    SELECT generate_series(
        date_trunc(sqlc.arg('granularity')::text, sqlc.arg('from_at')::timestamptz),
        sqlc.arg('to_at')::timestamptz,
        ('1 ' || sqlc.arg('granularity')::text)::interval
    ) AS bucket_start
)
SELECT
    b.bucket_start::timestamptz AS bucket_at,
    (
        (SELECT total FROM current_total) - COALESCE((
            SELECT SUM(
                CASE t.type
                    WHEN 'income'::transaction_type  THEN  t.amount
                    WHEN 'expense'::transaction_type THEN -t.amount
                    ELSE 0::numeric
                END
            )
            FROM transactions t
            WHERE t.occurred_at > LEAST(
                b.bucket_start + ('1 ' || sqlc.arg('granularity')::text)::interval - interval '1 microsecond',
                sqlc.arg('to_at')::timestamptz
            )
        ), 0::numeric)
    )::numeric AS total
FROM buckets b
ORDER BY b.bucket_start;

-- name: GetCategoryStats :many
-- Sums and counts transactions of a given type per category in a date range,
-- optionally filtered by account_id (matches account_id OR to_account_id for
-- transfers).
WITH filtered AS (
    SELECT t.category_id, t.amount
    FROM transactions t
    WHERE t.type = sqlc.arg('type')::transaction_type
      AND t.occurred_at >= sqlc.arg('from_at')::timestamptz
      AND t.occurred_at <= sqlc.arg('to_at')::timestamptz
      AND (
          sqlc.narg('account_id')::int IS NULL
          OR t.account_id    = sqlc.narg('account_id')::int
          OR t.to_account_id = sqlc.narg('account_id')::int
      )
),
totals AS (
    SELECT COALESCE(SUM(amount), 0)::numeric AS total_amount FROM filtered
)
SELECT
    f.category_id,
    COALESCE(c.name, 'Sin categoría')::text AS name,
    SUM(f.amount)::numeric AS amount,
    COUNT(*)::bigint AS tx_count,
    CASE
        WHEN (SELECT total_amount FROM totals) > 0
        THEN (SUM(f.amount) / (SELECT total_amount FROM totals))::float8
        ELSE 0::float8
    END AS share
FROM filtered f
LEFT JOIN categories c ON c.id = f.category_id
GROUP BY f.category_id, c.name
ORDER BY SUM(f.amount) DESC, name ASC;

-- name: GetMonthlyStats :many
-- Returns one row per calendar month with summed income, expense in the date
-- range. Optional account_id (matches source or destination) and category_id
-- filters.
SELECT
    to_char(date_trunc('month', occurred_at), 'YYYY-MM')::text AS month,
    COALESCE(SUM(amount) FILTER (WHERE type = 'income'::transaction_type),  0::numeric)::numeric AS income,
    COALESCE(SUM(amount) FILTER (WHERE type = 'expense'::transaction_type), 0::numeric)::numeric AS expense
FROM transactions
WHERE occurred_at >= sqlc.arg('from_at')::timestamptz
  AND occurred_at <= sqlc.arg('to_at')::timestamptz
  AND type IN ('income'::transaction_type, 'expense'::transaction_type)
  AND (
      sqlc.narg('account_id')::int IS NULL
      OR account_id    = sqlc.narg('account_id')::int
      OR to_account_id = sqlc.narg('account_id')::int
  )
  AND (
      sqlc.narg('category_id')::int IS NULL
      OR category_id = sqlc.narg('category_id')::int
  )
GROUP BY date_trunc('month', occurred_at)
ORDER BY date_trunc('month', occurred_at);
