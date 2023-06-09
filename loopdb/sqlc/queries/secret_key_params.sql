-- name: GetSecretKeyParams :one
SELECT
  params
FROM
  secret_key_params
ORDER BY id DESC
LIMIT 1;

-- name: InsertSecretKeyParams :exec
INSERT INTO secret_key_params (
    params
) VALUES (
    $1
);
