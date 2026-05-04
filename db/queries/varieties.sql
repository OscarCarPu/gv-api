-- name: GetVariety :one
SELECT id, name, scent, flavor, power, quality, score, price, comments, judge, deleted_at
FROM weed_varieties
WHERE id = $1 AND deleted_at IS NULL;

-- name: ListVarieties :many
SELECT id, name, scent, flavor, power, quality, score, price, comments, judge, deleted_at
FROM weed_varieties
WHERE deleted_at IS NULL
ORDER BY score DESC, price ASC;

-- name: CreateVariety :one
INSERT INTO weed_varieties (name, scent, flavor, power, quality, price, comments, judge)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, name, scent, flavor, power, quality, score, price, comments, judge, deleted_at;

-- name: UpdateVariety :one
UPDATE weed_varieties
SET name = $2, scent = $3, flavor = $4, power = $5, quality = $6, price = $7, comments = $8, judge = $9
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, name, scent, flavor, power, quality, score, price, comments, judge, deleted_at;

-- name: DeleteVariety :exec
UPDATE weed_varieties
SET deleted_at = now()
WHERE id = $1 AND deleted_at IS NULL;
