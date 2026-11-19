-- Draft orders created when a request is accepted, and their line items.

-- name: CreateOrder :one
-- Step 5 of accepting. The unique constraint on (workspace_id,
-- order_request_id) means a second insert for the same request fails with a
-- unique violation, which the service maps to 409 Conflict. That is the
-- database-level guarantee behind the row lock.
INSERT INTO orders (workspace_id, order_request_id)
VALUES ($1, $2)
RETURNING id, workspace_id, order_request_id, status, created_at, updated_at;

-- name: CreateOrderItems :copyfrom
-- Step 6: copies the request's snapshots onto the order, so the order records
-- the prices that were quoted rather than today's prices. COPY keeps this to
-- one round trip regardless of line count.
INSERT INTO order_items (
    workspace_id, order_id, product_id, product_name, quantity, unit_price, currency
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CopyOrderItemsFromRequest :many
-- Alternative to CreateOrderItems that never leaves the database: the order
-- lines are built directly from the request's lines, so the snapshots cannot
-- drift in transit. Quantities are aggregated by product to match the single
-- stock decrement each product receives.
INSERT INTO order_items (
    workspace_id, order_id, product_id, product_name, quantity, unit_price, currency
)
SELECT
    item.workspace_id,
    sqlc.arg('order_id')::uuid,
    item.product_id,
    min(item.product_name),
    sum(item.quantity)::int,
    min(item.unit_price),
    min(item.currency)
FROM order_request_items AS item
WHERE item.workspace_id = sqlc.arg('workspace_id')::uuid
  AND item.order_request_id = sqlc.arg('order_request_id')::uuid
GROUP BY item.workspace_id, item.product_id
RETURNING id, workspace_id, order_id, product_id, product_name, quantity, unit_price, currency, created_at;

-- name: GetOrderByRequest :one
SELECT id, workspace_id, order_request_id, status, created_at, updated_at
FROM orders
WHERE workspace_id = $1
  AND order_request_id = $2;

-- name: GetOrder :one
SELECT id, workspace_id, order_request_id, status, created_at, updated_at
FROM orders
WHERE workspace_id = $1
  AND id = $2;

-- name: ListOrderItems :many
SELECT id, workspace_id, order_id, product_id, product_name, quantity, unit_price, currency, created_at
FROM order_items
WHERE workspace_id = $1
  AND order_id = $2
ORDER BY product_name ASC, product_id ASC;

-- name: ListOrders :many
SELECT id, workspace_id, order_request_id, status, created_at, updated_at
FROM orders
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int
OFFSET sqlc.arg('page_offset')::int;

-- name: CountOrders :one
SELECT count(*) AS total
FROM orders
WHERE workspace_id = $1;
