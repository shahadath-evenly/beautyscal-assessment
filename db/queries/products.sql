-- Every query here is scoped by workspace_id. A product is only ever visible
-- to, or mutable by, its own workspace.

-- name: GetProduct :one
SELECT id, workspace_id, name, price, currency, stock, created_at, updated_at
FROM products
WHERE workspace_id = $1
  AND id = $2;

-- name: ListProducts :many
-- Search runs in PostgreSQL as a parameterised ILIKE; the pattern is built by
-- the database from the bound value, never by string concatenation in Go.
-- Ordering is (created_at DESC, id DESC) so pages never overlap or repeat.
SELECT id, workspace_id, name, price, currency, stock, created_at, updated_at
FROM products
WHERE workspace_id = $1
  AND (
      sqlc.narg('search')::text IS NULL
      OR name ILIKE '%' || sqlc.narg('search')::text || '%'
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int
OFFSET sqlc.arg('page_offset')::int;

-- name: CountProducts :one
-- Must apply exactly the same filter as ListProducts, or totalPages lies.
SELECT count(*) AS total
FROM products
WHERE workspace_id = $1
  AND (
      sqlc.narg('search')::text IS NULL
      OR name ILIKE '%' || sqlc.narg('search')::text || '%'
  );

-- name: ListProductsByIDs :many
-- Loads several products in one round trip, which keeps request creation free
-- of a per-line query.
SELECT id, workspace_id, name, price, currency, stock, created_at, updated_at
FROM products
WHERE workspace_id = $1
  AND id = ANY (sqlc.arg('product_ids')::uuid[])
ORDER BY id ASC;

-- name: CreateProduct :one
INSERT INTO products (workspace_id, name, price, currency, stock)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, workspace_id, name, price, currency, stock, created_at, updated_at;

-- name: GetProductByName :one
-- Lets the seed command find an existing product before inserting, so it can
-- run repeatedly without duplicating rows. (workspace_id, name) is indexed
-- but not unique, so this is a lookup rather than an upsert conflict target.
SELECT id, workspace_id, name, price, currency, stock, created_at, updated_at
FROM products
WHERE workspace_id = $1
  AND name = $2
ORDER BY created_at ASC, id ASC
LIMIT 1;

-- name: UpdateProductCatalogue :one
-- Companion to GetProductByName: refreshes price, currency and stock for a
-- product the seed has already created.
UPDATE products
SET price      = $3,
    currency   = $4,
    stock      = $5,
    updated_at = now()
WHERE workspace_id = $1
  AND id = $2
RETURNING id, workspace_id, name, price, currency, stock, created_at, updated_at;
