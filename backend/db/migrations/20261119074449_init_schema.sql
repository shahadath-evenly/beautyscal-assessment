-- Initial schema for the Brand Order Requests workflow.
--
-- Tenancy: every tenant-owned table carries workspace_id, and every foreign
-- key between tenant-owned tables is composite on (workspace_id, <id>). That
-- makes a cross-workspace reference impossible at the database level, not
-- merely unlikely: a row cannot point at a parent belonging to another
-- workspace, because the parent is matched on workspace_id as well.
--
-- Money: NUMERIC(12,2) throughout, never a floating-point type. Prices and
-- currencies are snapshotted onto request and order items, so a later price
-- change never rewrites the history of a submitted request.

-- gen_random_uuid() is built in from PostgreSQL 13 onward; the extension is
-- created defensively for older servers.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- workspaces: the tenant root.
-- ---------------------------------------------------------------------------
CREATE TABLE workspaces (
    id         UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT workspaces_slug_key UNIQUE (slug),
    CONSTRAINT workspaces_name_not_blank CHECK (length(btrim(name)) > 0),
    -- Slugs appear in URLs and seed scripts, so the shape is pinned here.
    CONSTRAINT workspaces_slug_format CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

-- ---------------------------------------------------------------------------
-- products: the catalogue a workspace publishes.
-- ---------------------------------------------------------------------------
CREATE TABLE products (
    id           UUID PRIMARY KEY        DEFAULT gen_random_uuid(),
    workspace_id UUID           NOT NULL,
    name         TEXT           NOT NULL,
    price        NUMERIC(12, 2) NOT NULL,
    currency     CHAR(3)        NOT NULL,
    stock        INTEGER        NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ    NOT NULL DEFAULT now(),

    CONSTRAINT products_workspace_id_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,

    -- Target of the tenant-aware composite foreign keys below. Redundant with
    -- the primary key on its own, but required for a composite reference.
    CONSTRAINT products_workspace_id_id_key UNIQUE (workspace_id, id),

    CONSTRAINT products_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT products_price_non_negative CHECK (price >= 0),
    -- Stock can never go negative: accepting a request that would oversell
    -- fails here even if application logic were wrong.
    CONSTRAINT products_stock_non_negative CHECK (stock >= 0),
    CONSTRAINT products_currency_format CHECK (currency ~ '^[A-Z]{3}$')
);

-- Listing and search are always scoped to one workspace and ordered by name.
CREATE INDEX products_workspace_id_name_idx ON products (workspace_id, name);

-- ---------------------------------------------------------------------------
-- order_requests: what a buyer submits for review.
-- ---------------------------------------------------------------------------
CREATE TABLE order_requests (
    id           UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id UUID        NOT NULL,
    buyer_name   TEXT        NOT NULL,
    buyer_email  TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'new',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT order_requests_workspace_id_fkey
        FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE CASCADE,

    CONSTRAINT order_requests_workspace_id_id_key UNIQUE (workspace_id, id),

    CONSTRAINT order_requests_status_check
        CHECK (status IN ('new', 'accepted', 'rejected')),
    CONSTRAINT order_requests_buyer_name_not_blank
        CHECK (length(btrim(buyer_name)) > 0),
    -- A deliberately permissive shape check. Full validation happens at the
    -- HTTP boundary; this only stops obvious rubbish reaching storage.
    CONSTRAINT order_requests_buyer_email_format
        CHECK (buyer_email ~ '^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$')
);

-- The sales listing filters by status and sorts newest first, within a
-- workspace. Matches the ORDER BY created_at DESC, id DESC used by the API.
CREATE INDEX order_requests_workspace_id_status_created_at_idx
    ON order_requests (workspace_id, status, created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- order_request_items: the lines of a request, with price snapshots.
-- ---------------------------------------------------------------------------
CREATE TABLE order_request_items (
    id               UUID PRIMARY KEY        DEFAULT gen_random_uuid(),
    workspace_id     UUID           NOT NULL,
    order_request_id UUID           NOT NULL,
    product_id       UUID           NOT NULL,
    product_name     TEXT           NOT NULL,
    quantity         INTEGER        NOT NULL,
    unit_price       NUMERIC(12, 2) NOT NULL,
    currency         CHAR(3)        NOT NULL,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT now(),

    -- Tenant-aware: the parent request must belong to the same workspace.
    CONSTRAINT order_request_items_request_fkey
        FOREIGN KEY (workspace_id, order_request_id)
        REFERENCES order_requests (workspace_id, id) ON DELETE CASCADE,

    -- Tenant-aware: the product must belong to the same workspace. RESTRICT
    -- keeps a product that a request refers to from being deleted.
    CONSTRAINT order_request_items_product_fkey
        FOREIGN KEY (workspace_id, product_id)
        REFERENCES products (workspace_id, id) ON DELETE RESTRICT,

    CONSTRAINT order_request_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT order_request_items_unit_price_non_negative CHECK (unit_price >= 0),
    CONSTRAINT order_request_items_currency_format CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT order_request_items_product_name_not_blank
        CHECK (length(btrim(product_name)) > 0),

    -- One line per product per request: quantities are merged rather than
    -- duplicated, which keeps request totals and stock maths unambiguous.
    CONSTRAINT order_request_items_request_product_key
        UNIQUE (workspace_id, order_request_id, product_id)
);

CREATE INDEX order_request_items_workspace_id_order_request_id_idx
    ON order_request_items (workspace_id, order_request_id);

-- ---------------------------------------------------------------------------
-- orders: the draft order created when a request is accepted.
-- ---------------------------------------------------------------------------
CREATE TABLE orders (
    id               UUID PRIMARY KEY     DEFAULT gen_random_uuid(),
    workspace_id     UUID        NOT NULL,
    order_request_id UUID        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'draft',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT orders_request_fkey
        FOREIGN KEY (workspace_id, order_request_id)
        REFERENCES order_requests (workspace_id, id) ON DELETE RESTRICT,

    -- At most one order per request. This is the database-level guarantee
    -- that a double-click, a retry or a race cannot produce two orders; the
    -- accept transaction's row lock is the first line of defence, this is the
    -- second.
    CONSTRAINT orders_workspace_id_order_request_id_key
        UNIQUE (workspace_id, order_request_id),

    CONSTRAINT orders_workspace_id_id_key UNIQUE (workspace_id, id),

    CONSTRAINT orders_status_check CHECK (status = 'draft')
);

CREATE INDEX orders_workspace_id_created_at_idx
    ON orders (workspace_id, created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- order_items: the lines of a draft order, copied from the request snapshot.
-- ---------------------------------------------------------------------------
CREATE TABLE order_items (
    id           UUID PRIMARY KEY        DEFAULT gen_random_uuid(),
    workspace_id UUID           NOT NULL,
    order_id     UUID           NOT NULL,
    product_id   UUID           NOT NULL,
    product_name TEXT           NOT NULL,
    quantity     INTEGER        NOT NULL,
    unit_price   NUMERIC(12, 2) NOT NULL,
    currency     CHAR(3)        NOT NULL,
    created_at   TIMESTAMPTZ    NOT NULL DEFAULT now(),

    CONSTRAINT order_items_order_fkey
        FOREIGN KEY (workspace_id, order_id)
        REFERENCES orders (workspace_id, id) ON DELETE CASCADE,

    CONSTRAINT order_items_product_fkey
        FOREIGN KEY (workspace_id, product_id)
        REFERENCES products (workspace_id, id) ON DELETE RESTRICT,

    CONSTRAINT order_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT order_items_unit_price_non_negative CHECK (unit_price >= 0),
    CONSTRAINT order_items_currency_format CHECK (currency ~ '^[A-Z]{3}$'),
    CONSTRAINT order_items_product_name_not_blank
        CHECK (length(btrim(product_name)) > 0),

    CONSTRAINT order_items_order_product_key
        UNIQUE (workspace_id, order_id, product_id)
);

CREATE INDEX order_items_workspace_id_order_id_idx
    ON order_items (workspace_id, order_id);
