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
