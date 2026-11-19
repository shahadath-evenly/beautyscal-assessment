-- Verifies the invariants the schema is supposed to guarantee. Each block
-- either succeeds where it must, or fails with the named constraint where it
-- must. Run against a database with the migrations applied:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db/verify/invariants.sql
\set ON_ERROR_STOP on
\set QUIET on
BEGIN;

-- Two workspaces and a product in each.
INSERT INTO workspaces (id, name, slug) VALUES
  ('11111111-1111-1111-1111-111111111111', 'Workspace A', 'workspace-a'),
  ('22222222-2222-2222-2222-222222222222', 'Workspace B', 'workspace-b');

INSERT INTO products (id, workspace_id, name, price, currency, stock) VALUES
  ('aaaaaaaa-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Rose Serum',  49.90, 'EUR', 10),
  ('bbbbbbbb-0000-0000-0000-000000000001', '22222222-2222-2222-2222-222222222222', 'Clay Mask',   19.50, 'USD', 5);

INSERT INTO order_requests (id, workspace_id, buyer_name, buyer_email) VALUES
  ('cccccccc-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111', 'Buyer One', 'buyer@example.com');

\echo '--- setup ok'

-- 1. A request line may reference a product in its own workspace.
INSERT INTO order_request_items
  (workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
   'aaaaaaaa-0000-0000-0000-000000000001', 'Rose Serum', 2, 49.90, 'EUR');
\echo '--- 1 same-workspace line accepted'

-- 2. CROSS-TENANT: workspace A referencing workspace B's product must fail.
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO order_request_items
    (workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency)
  VALUES
    ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
     'bbbbbbbb-0000-0000-0000-000000000001', 'Clay Mask', 1, 19.50, 'USD');
  RAISE EXCEPTION 'FAIL: cross-workspace product reference was accepted';
EXCEPTION WHEN foreign_key_violation THEN
  RAISE NOTICE '--- 2 cross-workspace product reference rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 3. CROSS-TENANT: an order in workspace B for workspace A's request must fail.
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO orders (workspace_id, order_request_id)
  VALUES ('22222222-2222-2222-2222-222222222222', 'cccccccc-0000-0000-0000-000000000001');
  RAISE EXCEPTION 'FAIL: cross-workspace order was accepted';
EXCEPTION WHEN foreign_key_violation THEN
  RAISE NOTICE '--- 3 cross-workspace order rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 4. ONE ORDER PER REQUEST: the first succeeds, the second must fail.
INSERT INTO orders (id, workspace_id, order_request_id)
VALUES ('dddddddd-0000-0000-0000-000000000001',
        '11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001');
\echo '--- 4a first order created'
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO orders (workspace_id, order_request_id)
  VALUES ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001');
  RAISE EXCEPTION 'FAIL: a second order for the same request was accepted';
EXCEPTION WHEN unique_violation THEN
  RAISE NOTICE '--- 4b duplicate order rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 5. STOCK NEVER NEGATIVE.
SAVEPOINT s;
DO $$
BEGIN
  UPDATE products SET stock = stock - 999
   WHERE id = 'aaaaaaaa-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: negative stock was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 5 negative stock rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 6. QUANTITY MUST BE POSITIVE.
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO order_request_items
    (workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency)
  VALUES
    ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
     'aaaaaaaa-0000-0000-0000-000000000001', 'Rose Serum', 0, 49.90, 'EUR');
  RAISE EXCEPTION 'FAIL: quantity 0 was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 6 non-positive quantity rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 7. STATUS VALUES are constrained.
SAVEPOINT s;
DO $$
BEGIN
  UPDATE order_requests SET status = 'cancelled'
   WHERE id = 'cccccccc-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: invalid request status was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 7a invalid request status rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

SAVEPOINT s;
DO $$
BEGIN
  UPDATE orders SET status = 'confirmed'
   WHERE id = 'dddddddd-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: non-draft order status was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 7b non-draft order status rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 8. MONEY and CURRENCY shape.
SAVEPOINT s;
DO $$
BEGIN
  UPDATE products SET price = -1 WHERE id = 'aaaaaaaa-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: negative price was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 8a negative price rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

SAVEPOINT s;
DO $$
BEGIN
  UPDATE products SET currency = 'eu' WHERE id = 'aaaaaaaa-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: malformed currency was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 8b malformed currency rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- Money must round-trip exactly, with no binary floating point drift.
DO $$
DECLARE total NUMERIC(12,2);
BEGIN
  SELECT sum(quantity * unit_price) INTO total FROM order_request_items;
  IF total <> 99.80 THEN
    RAISE EXCEPTION 'FAIL: money arithmetic drifted, got %', total;
  END IF;
  RAISE NOTICE '--- 8c money arithmetic exact (2 x 49.90 = %)', total;
END $$;

-- 9. BUYER EMAIL shape.
SAVEPOINT s;
DO $$
BEGIN
  UPDATE order_requests SET buyer_email = 'not-an-email'
   WHERE id = 'cccccccc-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: malformed email was accepted';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 9 malformed buyer email rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 10. DUPLICATE PRODUCT LINE on one request is rejected, so quantities are
--     merged rather than split across rows.
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO order_request_items
    (workspace_id, order_request_id, product_id, product_name, quantity, unit_price, currency)
  VALUES
    ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
     'aaaaaaaa-0000-0000-0000-000000000001', 'Rose Serum', 3, 49.90, 'EUR');
  RAISE EXCEPTION 'FAIL: duplicate product line was accepted';
EXCEPTION WHEN unique_violation THEN
  RAISE NOTICE '--- 10 duplicate product line rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 11. A product still referenced by a request cannot be deleted.
SAVEPOINT s;
DO $$
BEGIN
  DELETE FROM products WHERE id = 'aaaaaaaa-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: a referenced product was deleted';
EXCEPTION WHEN foreign_key_violation THEN
  RAISE NOTICE '--- 11 deleting a referenced product rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 12. ONE EMAIL EVENT PER REQUEST per event type: a retried acceptance must
--     not queue a second message to the buyer.
SAVEPOINT s;
INSERT INTO email_outbox
  (workspace_id, order_request_id, event_type, recipient_email, recipient_name,
   subject, body_text, body_html)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
   'order_request_accepted', 'ada@example.com', 'Ada Lovelace',
   'Your order request has been accepted', 'plain', '<p>rich</p>');
DO $$
BEGIN
  INSERT INTO email_outbox
    (workspace_id, order_request_id, event_type, recipient_email, recipient_name,
     subject, body_text, body_html)
  VALUES
    ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
     'order_request_accepted', 'ada@example.com', 'Ada Lovelace',
     'A second message', 'plain', '<p>rich</p>');
  RAISE EXCEPTION 'FAIL: a duplicate email event was accepted';
EXCEPTION WHEN unique_violation THEN
  RAISE NOTICE '--- 12 duplicate email event rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 13. A SENT EVENT MUST SAY WHEN. Nothing may claim delivery without a
--     timestamp, and nothing unsent may carry one.
SAVEPOINT s;
INSERT INTO email_outbox
  (workspace_id, order_request_id, event_type, recipient_email, recipient_name,
   subject, body_text, body_html)
VALUES
  ('11111111-1111-1111-1111-111111111111', 'cccccccc-0000-0000-0000-000000000001',
   'order_request_accepted', 'ada@example.com', 'Ada Lovelace',
   'Your order request has been accepted', 'plain', '<p>rich</p>');
DO $$
BEGIN
  UPDATE email_outbox SET status = 'sent'
   WHERE order_request_id = 'cccccccc-0000-0000-0000-000000000001';
  RAISE EXCEPTION 'FAIL: an event was marked sent without a sent_at';
EXCEPTION WHEN check_violation THEN
  RAISE NOTICE '--- 13 sent event without a timestamp rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

-- 14. AN EMAIL EVENT CANNOT CROSS WORKSPACES: the composite foreign key
--     matches the request on its workspace as well as its ID.
SAVEPOINT s;
DO $$
BEGIN
  INSERT INTO email_outbox
    (workspace_id, order_request_id, event_type, recipient_email, recipient_name,
     subject, body_text, body_html)
  VALUES
    ('22222222-2222-2222-2222-222222222222', 'cccccccc-0000-0000-0000-000000000001',
     'order_request_accepted', 'ada@example.com', 'Ada Lovelace',
     'Your order request has been accepted', 'plain', '<p>rich</p>');
  RAISE EXCEPTION 'FAIL: a cross-workspace email event was accepted';
EXCEPTION WHEN foreign_key_violation THEN
  RAISE NOTICE '--- 14 cross-workspace email event rejected';
END $$;
ROLLBACK TO SAVEPOINT s;

\echo '--- ALL INVARIANTS HOLD'
ROLLBACK;
