-- name: GetVariety :one
SELECT id, name, scent, flavor, power, quality, score, price, comments
FROM weed_varieties WHERE id = $1;

-- name: ListVarieties :many
SELECT id, name, scent, flavor, power, quality, score, price, comments
FROM weed_varieties
ORDER BY score DESC, price ASC;

-- name: CreateVariety :one
INSERT INTO weed_varieties (name, scent, flavor, power, quality, price, comments)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, name, scent, flavor, power, quality, score, price, comments;

-- name: UpdateVariety :one
UPDATE weed_varieties
SET name = $2, scent = $3, flavor = $4, power = $5, quality = $6, price = $7, comments = $8
WHERE id = $1
RETURNING id, name, scent, flavor, power, quality, score, price, comments;

-- name: DeleteVariety :exec
DELETE FROM weed_varieties WHERE id = $1;
