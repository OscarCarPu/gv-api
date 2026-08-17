-- name: ListLights :many
SELECT id, name, model, address, protocol, supports_color, supports_color_temp,
       min_color_temp, max_color_temp, options
FROM lights
ORDER BY name;

-- name: GetLight :one
SELECT id, name, model, address, protocol, supports_color, supports_color_temp,
       min_color_temp, max_color_temp, options
FROM lights
WHERE id = $1;

-- name: CreateLight :one
INSERT INTO lights (id, name, model, address, protocol, supports_color, supports_color_temp,
                    min_color_temp, max_color_temp, options)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, name, model, address, protocol, supports_color, supports_color_temp,
          min_color_temp, max_color_temp, options;

-- name: UpdateLight :one
UPDATE lights
SET name = $2, model = $3, protocol = $4, supports_color = $5, supports_color_temp = $6,
    min_color_temp = $7, max_color_temp = $8, options = $9
WHERE id = $1
RETURNING id, name, model, address, protocol, supports_color, supports_color_temp,
          min_color_temp, max_color_temp, options;

-- name: DeleteLight :execrows
DELETE FROM lights WHERE id = $1;
