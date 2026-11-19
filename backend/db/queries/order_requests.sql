-- Buyer requests and their line items. Every statement is scoped by
-- workspace_id, including the ones that run inside the accept transaction.

-- name: CreateOrderRequest :one
INSERT INTO order_requests (workspace_id, buyer_name, buyer_email)
VALUES ($1, $2, $3)
RETURNING id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at;

-- name: CreateOrderRequestItems :copyfrom
-- Bulk insert through the PostgreSQL COPY protocol: one round trip for every
-- line of a request, rather than one statement per line. Constraints and
-- foreign keys still apply, so the tenant-aware checks hold.
INSERT INTO order_request_items (
    workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetOrderRequest :one
SELECT id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at
FROM order_requests
WHERE workspace_id = $1
  AND id = $2;

-- name: ListOrderRequests :many
-- Status filter and buyer search both run in SQL, parameterised. Ordering is
-- (created_at DESC, id DESC), matching the composite index and giving stable
-- pages.
SELECT id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at
FROM order_requests
WHERE workspace_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (
      sqlc.narg('search')::text IS NULL
      OR buyer_name ILIKE '%' || sqlc.narg('search')::text || '%'
      OR buyer_email ILIKE '%' || sqlc.narg('search')::text || '%'
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int
OFFSET sqlc.arg('page_offset')::int;

-- name: CountOrderRequests :one
-- Same predicate as ListOrderRequests, so pagination metadata is consistent.
SELECT count(*) AS total
FROM order_requests
WHERE workspace_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (
      sqlc.narg('search')::text IS NULL
      OR buyer_name ILIKE '%' || sqlc.narg('search')::text || '%'
      OR buyer_email ILIKE '%' || sqlc.narg('search')::text || '%'
  );

-- name: ListOrderRequestItems :many
SELECT id, workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency, created_at
FROM order_request_items
WHERE workspace_id = $1
  AND order_request_id = $2
ORDER BY product_name ASC, product_id ASC;

-- name: ListOrderRequestItemsForRequests :many
-- Loads the lines of several requests at once, so a list endpoint that needs
-- totals does not issue one query per row.
SELECT id, workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency, created_at
FROM order_request_items
WHERE workspace_id = $1
  AND order_request_id = ANY (sqlc.arg('order_request_ids')::uuid[])
ORDER BY order_request_id, product_name ASC, product_id ASC;

-- ---------------------------------------------------------------------------
-- Accept and reject
-- ---------------------------------------------------------------------------

-- name: GetOrderRequestForUpdate :one
-- Step 1 of accepting. FOR UPDATE holds the request row for the life of the
-- transaction, so two concurrent accepts of the same request serialise here:
-- the second waits, then sees status 'accepted' and is rejected with 409.
SELECT id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at
FROM order_requests
WHERE workspace_id = $1
  AND id = $2
FOR UPDATE;

-- name: SumOrderRequestQuantitiesByProduct :many
-- Aggregates the lines by product, so a product appearing more than once in a
-- request is locked and decremented once, with its total quantity.
-- Ordered by product_id to give every transaction the same lock order, which
-- is what prevents deadlocks between concurrent accepts.
SELECT product_id, sum(quantity)::int AS total_quantity
FROM order_request_items
WHERE workspace_id = $1
  AND order_request_id = $2
GROUP BY product_id
ORDER BY product_id ASC;

-- name: LockProductsForUpdate :many
-- Step 2: lock every product the request touches, in ascending id order.
-- Deterministic ordering is the deadlock guard; without it two transactions
-- holding each other's rows could wait forever.
SELECT id, workspace_id, name, price, currency, stock, created_at, updated_at
FROM products
WHERE workspace_id = $1
  AND id = ANY (sqlc.arg('product_ids')::uuid[])
ORDER BY id ASC
FOR UPDATE;

-- name: DecrementProductStock :one
-- Step 3: the stock guard is in the WHERE clause, so an oversell updates no
-- row and returns no result rather than writing a negative value. The CHECK
-- constraint backs this up at the storage level.
UPDATE products
SET stock      = stock - sqlc.arg('quantity')::int,
    updated_at = now()
WHERE workspace_id = $1
  AND id = $2
  AND stock >= sqlc.arg('quantity')::int
RETURNING id, workspace_id, name, price, currency, stock, created_at, updated_at;

-- name: MarkOrderRequestAccepted :one
-- Step 4. The status = 'new' predicate makes the transition itself atomic: a
-- request already accepted or rejected updates no row.
UPDATE order_requests
SET status     = 'accepted',
    updated_at = now()
WHERE workspace_id = $1
  AND id = $2
  AND status = 'new'
RETURNING id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at;

-- name: MarkOrderRequestRejected :one
-- Rejection touches no stock and creates no order. Returning no row means the
-- request was missing, or no longer 'new'.
UPDATE order_requests
SET status     = 'rejected',
    updated_at = now()
WHERE workspace_id = $1
  AND id = $2
  AND status = 'new'
RETURNING id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at;

-- name: ExpireStaleOrderRequests :many
-- Background job (bonus): expire requests left in 'new' for too long. Kept
-- here so the statement is reviewed alongside the other status transitions.
UPDATE order_requests
SET status     = 'rejected',
    updated_at = now()
WHERE status = 'new'
  AND created_at < sqlc.arg('older_than')::timestamptz
RETURNING id, workspace_id, buyer_name, buyer_email, status, created_at, updated_at;
