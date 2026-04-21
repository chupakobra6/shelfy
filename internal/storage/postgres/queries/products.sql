-- name: ListActiveProducts :many
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE user_id = $1 AND status = 'active'
ORDER BY expires_on ASC, id DESC;

-- name: CountActiveProducts :one
SELECT COUNT(*)::bigint
FROM products
WHERE user_id = $1 AND status = 'active';

-- name: ListActiveProductsPage :many
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE user_id = $1 AND status = 'active'
ORDER BY expires_on ASC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListSoonProducts :many
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE user_id = $1
  AND status = 'active'
  AND expires_on >= $2::date
  AND expires_on <= $3::date
ORDER BY expires_on ASC, id DESC;

-- name: CountSoonProducts :one
SELECT COUNT(*)::bigint
FROM products
WHERE user_id = $1
  AND status = 'active'
  AND expires_on >= $2::date
  AND expires_on <= $3::date;

-- name: ListSoonProductsPage :many
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE user_id = $1
  AND status = 'active'
  AND expires_on >= $2::date
  AND expires_on <= $3::date
ORDER BY expires_on ASC, id DESC
LIMIT $4 OFFSET $5;

-- name: ListExpiredProducts :many
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE user_id = $1
  AND status = 'active'
  AND expires_on < $2::date
ORDER BY expires_on ASC, id DESC;

-- name: GetProduct :one
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE id = $1;

-- name: GetProductByID :one
SELECT id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at
FROM products
WHERE id = $1;

-- name: CreateProduct :one
INSERT INTO products (user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind)
VALUES ($1, $2, $3, $4, $5, 'active', $6)
RETURNING id, user_id, name, normalized_name, expires_on, raw_deadline_phrase, status, source_kind, created_at, closed_at;

-- name: UpdateProductStatus :exec
UPDATE products
SET status = $2,
    closed_at = CASE WHEN $2 = 'active' THEN NULL ELSE NOW() END
WHERE id = $1;

-- name: ActiveProductsExist :one
SELECT EXISTS (
    SELECT 1
    FROM products
    WHERE id = ANY($1::bigint[])
      AND status = 'active'
);
