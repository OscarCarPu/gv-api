-- name: OutagesStart :one
SELECT
  ends_at
FROM
  outages
WHERE
  kind = 'start';

-- name: LastOutage :one
SELECT
  id,
  ends_at,
  kind
FROM
  outages
ORDER BY
  ends_at DESC
LIMIT 1
FOR UPDATE;

-- name: ExtendOutage :exec
UPDATE
  outages
SET
  ends_at = $2
WHERE
  id = $1;

-- name: AddOutage :exec
INSERT INTO outages (ends_at, kind)
  VALUES ($1, $2);

-- name: ListOutageRanges :many
SELECT
  kind,
  started_at::timestamptz AS started_at,
  ends_at
FROM (
  SELECT
    kind,
    ends_at,
    lag(ends_at) OVER (ORDER BY ends_at) AS started_at
  FROM
    outages) AS r
WHERE
  started_at IS NOT NULL
  AND ends_at > @from_at::timestamptz
  AND started_at < @to_at::timestamptz
ORDER BY
  ends_at;
