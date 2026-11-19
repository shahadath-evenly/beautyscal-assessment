# BeautyScale backend

Go API for the Brand Order Requests workflow: chi, pgx/v5, sqlc, Atlas,
PostgreSQL. Multi-tenant — every request is scoped by the `X-Workspace-ID`
header, and every tenant-owned query carries `workspace_id`.

## Requirements

- Go 1.26 (the toolchain is pinned in `go.mod`)
- PostgreSQL 17
- [Atlas CLI](https://atlasgo.io) for migrations
- Docker, optional — the API and database also run on the host

## Configuration

Copy the example file and adjust:

```bash
cp .env.example .env
```

`.env` is read by `config.Load()` at startup, by the Makefile and by Docker
Compose. Real environment variables always take precedence over the file, so
`HTTP_PORT=8082 go run ./cmd/api` works without editing anything.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | — (required) | PostgreSQL connection string |
| `APP_ENV` | `local` | Environment name shown at startup |
| `HTTP_PORT` | `8081` | Port the API listens on |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | `text` for humans, `json` for log collectors |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:5173` | Comma-separated; `*` allows any origin |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | `10` / `1` | pgx pool bounds |
| `REQUEST_TIMEOUT` | `15s` | Per-request handling budget |
| `READ_TIMEOUT` / `WRITE_TIMEOUT` | `30s` | Whole request and response |
| `IDLE_TIMEOUT` | `60s` | Keep-alive connections that go quiet |
| `MAX_BODY_BYTES` | `1048576` | Request body cap (1 MiB) |
| `ATLAS_DEV_URL` | `postgres://localhost:5432/bscale_atlas_dev` | Scratch database Atlas rebuilds on every run |

## Running

```bash
createdb bscale_dev          # once
make migrate-apply           # create the schema
make api                     # http://localhost:8081
```

Startup prints:

```
◇ injected env (13) from .env
PostgreSQL connection established
Backend server listening on http://localhost:8081 (development)
```

`make help` lists every target. `make check` runs gofmt, vet, build and tests.

## API

All routes are under `/api` and require the `X-Workspace-ID` header.

### `GET /api/products`

List the workspace's catalogue.

| Parameter | Default | Notes |
| --- | --- | --- |
| `page` | `1` | Must be 1 or greater |
| `limit` | `10` | 1 to 100 |
| `search` | — | Case-insensitive match on the product name |

```bash
WS=84395f84-e5e5-42ac-9460-dc3f34f9023d

curl -H "X-Workspace-ID: $WS" localhost:8081/api/products
curl -H "X-Workspace-ID: $WS" "localhost:8081/api/products?search=serum"
curl -H "X-Workspace-ID: $WS" "localhost:8081/api/products?page=2&limit=5"
```

```json
{
  "data": [
    {
      "id": "de09bed3-b265-47d9-80ec-5c6af378fb30",
      "name": "Night Repair Cream 50ml",
      "price": "89.90",
      "currency": "EUR",
      "stock": 65,
      "createdAt": "2026-11-19T14:33:22.646383+06:00",
      "updatedAt": "2026-11-19T14:33:22.646383+06:00"
    }
  ],
  "meta": { "page": 1, "limit": 10, "total": 5, "totalPages": 1 }
}
```

`GET /api/products/{id}` returns a single product, or `404` when it does not
exist **or belongs to another workspace** — the two are deliberately
indistinguishable.

Both search and pagination execute in PostgreSQL. The count query applies the
identical filter, so `total` can never disagree with the rows. Ordering is
`created_at DESC, id DESC`, which keeps pages from overlapping.

An unusable `page` or `limit` is a `400` naming the field, rather than a
silent fallback:

```json
{"error":{"code":"bad_request","message":"One or more query parameters are invalid.",
          "details":[{"field":"limit","message":"must be 100 or fewer"}]}}
```

**Money is a string**, never a JSON number: most clients parse JSON numbers as
doubles, which cannot represent every decimal amount exactly. Amounts are
`NUMERIC(12,2)` in the database and fixed-point strings on the wire, converted
through `big.Int` with no floating point anywhere.

### `POST /api/order-requests`

Submit a buyer request. The payload carries **only** buyer details, product
IDs and quantities:

```bash
curl -X POST -H "X-Workspace-ID: $WS" -H "Content-Type: application/json" \
  -d '{"buyerName":"Ada Lovelace","buyerEmail":"ada@example.com",
       "items":[{"productId":"...","quantity":2}]}' \
  localhost:8081/api/order-requests
```

Returns `201` with the created request, its lines and totals.

**Prices are never taken from the client.** Unit prices, currencies and
product names are read from PostgreSQL inside the creating transaction and
stored as snapshots on the request's lines, so a later catalogue change cannot
rewrite what a buyer was quoted. A payload carrying `unitPrice` is rejected
outright, because unknown fields are refused.

Validation:

| Rule | Response |
| --- | --- |
| Buyer name required, email valid, at least one item, quantity > 0 | `422 validation_failed` with field details |
| A product that does not exist **or belongs to another workspace** | `422` naming the product ID |
| Items spanning more than one currency | `422` explaining the mismatch |
| Malformed JSON, unknown field, empty body | `400 bad_request` |

A product listed twice is merged into one line with the quantities summed,
which keeps the stock arithmetic at acceptance unambiguous.

Creation is one transaction: the products are loaded, the request and all its
lines are inserted, and any failure leaves nothing behind.

### `GET /api/order-requests`

| Parameter | Default | Notes |
| --- | --- | --- |
| `page` | `1` | |
| `limit` | `10` | 1 to 100 |
| `search` | — | Matches buyer name or email |
| `status` | — | `new`, `accepted` or `rejected`; anything else is a `400` |

Each row carries the request's total, currency and item count, so the sales
table needs no follow-up requests. The lines of every row on the page are
loaded in a single query rather than one per row.

### `GET /api/order-requests/{id}`

Returns the buyer details, status, every line with quantity, unit price and
line total, the request total and currency, and the timestamps. `404` when it
does not exist or belongs to another workspace.

```json
{"data":{"id":"…","buyerName":"Ada Lovelace","buyerEmail":"ada@example.com",
         "status":"new","itemCount":2,"total":"225.90","currency":"EUR",
         "createdAt":"…","updatedAt":"…",
         "items":[{"productId":"…","productName":"Rose Absolute Serum 30ml",
                   "quantity":2,"unitPrice":"68.00","lineTotal":"136.00",
                   "currency":"EUR"}]}}
```

Accept and reject arrive in the next phase.

## Tenancy: the workspace middleware

Every route under `/api` is scoped to one workspace, resolved once by
`RequireWorkspace` rather than by each handler:

```
X-Workspace-ID: 84395f84-e5e5-42ac-9460-dc3f34f9023d
```

| Condition | Status | Code |
| --- | --- | --- |
| Header absent or blank | `400` | `workspace_header_missing` |
| Header is not a UUID | `400` | `workspace_header_invalid` |
| Workspace does not exist | `404` | `workspace_not_found` |
| Lookup failed | `500` | `internal_error` (cause logged, never returned) |

`404` rather than `403` for an unknown workspace is deliberate: confirming that
a workspace exists would leak the existence of other tenants. The same applies
to a resource belonging to another workspace.

`/health` and `/ready` sit outside this, because a readiness probe cannot
supply a workspace header.

**The header is the only accepted source of a workspace.** A `workspaceId` in
a request body is ignored; handlers read the value from the request context:

```go
workspaceID := middleware.MustWorkspaceID(r.Context())
products, err := svc.ListProducts(ctx, workspaceID, params)
```

`MustWorkspaceID` panics when the context has none. A handler behind the
middleware always does, so its absence is a wiring mistake — failing loudly is
safer than proceeding with a zero UUID, which would read across tenants.
`WorkspaceID(ctx)` is the non-panicking form.

Every repository and service operation takes the workspace ID as an explicit
argument. Nothing reads it from a global or re-derives it from the request.

### Request handling limits

- Request bodies are capped at `MAX_BODY_BYTES` (1 MiB by default) by
  middleware, so a large or endless body fails the read rather than buffering.
- `DecodeJSON` rejects unknown fields, trailing documents, a non-JSON
  `Content-Type` and an empty body, each with a specific message.
- Timeouts: `REQUEST_TIMEOUT` per request, `READ_TIMEOUT`/`WRITE_TIMEOUT` for
  the whole exchange, `IDLE_TIMEOUT` for idle keep-alives.

## Demo data

```bash
make seed            # insert anything missing; safe to run repeatedly
make seed-refresh    # also reset demo prices, currencies and stock
```

It prints the workspace IDs the frontend needs:

```
WORKSPACE         SLUG              ID                                    PRODUCTS  THIS RUN
Lumière Paris     lumiere-paris     84395f84-e5e5-42ac-9460-dc3f34f9023d  5         5 created
Coastal Botanics  coastal-botanics  647354ae-f8d2-4923-ac9d-b1eb21e6c229  4         4 created
```

Use one of those IDs as the `X-Workspace-ID` header, or pick the workspace in
the UI.

What it loads: two workspaces with stable slugs, five products for
Lumière Paris in EUR and four for Coastal Botanics in USD. Each workspace uses
a single currency, so a request total is meaningful without mixing. All of it
is invented sample data; there is no real customer data anywhere.

**Idempotency.** Workspaces are matched by their unique slug and products by
`(workspace, name)`, so repeated runs never duplicate a row and workspace IDs
stay stable. The whole seed runs in one transaction.

A plain `make seed` deliberately **does not** overwrite the price or stock of a
product that already exists, so re-seeding cannot quietly restore stock that a
demo acceptance consumed. Use `make seed-refresh` when you do want the demo
catalogue reset. The seed never writes order requests or orders, so it cannot
interfere with the accept-flow tests.

### Resetting the database

```bash
make db-reset CONFIRM=yes
```

**This destroys data.** It drops every table, re-applies the migrations and
re-seeds. Two guards stand in the way: it refuses without `CONFIRM=yes`,
printing the target `DATABASE_URL` first, and it refuses outright when
`APP_ENV=production`. It is intended for local development only.

To rebuild the schema without demo data, use `make migrate-fresh`.

## Migrations

Versioned migrations live in `db/migrations/`, with `atlas.sum` recording a
hash of each file. Atlas needs a scratch **dev database** to compute and
validate diffs; it is wiped on every run, so it must never point at real data:

```bash
createdb bscale_atlas_dev    # once
```

| Command | What it does |
| --- | --- |
| `make migrate-apply` | Apply pending migrations to `DATABASE_URL` |
| `make migrate-status` | Show the current version and anything pending |
| `make migrate-new name=add_x` | Create an empty migration file to write SQL into |
| `make migrate-hash` | Recompute `atlas.sum` after editing a migration |
| `make migrate-validate` | Check the directory against `atlas.sum` |
| `make migrate-fresh` | Drop every table and re-apply from scratch |
| `make verify-schema` | Assert the tenancy, stock and uniqueness invariants |

Adding a migration:

```bash
make migrate-new name=add_expiry_to_requests
$EDITOR db/migrations/<timestamp>_add_expiry_to_requests.sql
make migrate-hash            # required: an edited file fails validation otherwise
make migrate-validate
make migrate-apply
```

Migration files are immutable once applied anywhere. To change an applied
migration during local development, use `make migrate-fresh`.

The raw Atlas commands are equivalent, provided `DATABASE_URL` is exported:

```bash
atlas migrate apply  --env local
atlas migrate status --env local
```

Without `DATABASE_URL` set, Atlas fails with `required flag "url" not set`.
The make targets load `.env` for you.

Note: `atlas migrate lint` requires an Atlas Cloud account since v0.38, so it
is not part of the workflow here.

## Database layer (sqlc)

SQL lives in `db/queries/`, type-checked against the Atlas migrations and
compiled into Go by sqlc. The generated code in `internal/db/sqlc/` is never
edited by hand.

```bash
make sqlc           # regenerate after changing a query file
make sqlc-check     # fail if generated code is out of date (part of make check)
```

Configuration choices in `sqlc.yaml`:

- **`sql_package: pgx/v5`** — the generated code uses the same driver as the
  connection pool, so there is no `database/sql` layer in between.
- **UUIDs as `github.com/google/uuid.UUID`**, the type the workspace
  middleware parses `X-Workspace-ID` into.
- **Timestamps as `time.Time`**, exact because every timestamp column is
  `NOT NULL`.
- **Money as `pgtype.Numeric`** — the driver's exact decimal type. It is never
  converted to a float; the service layer renders it as a decimal string.
- **`emit_interface`** generates a `Querier` interface, so services depend on
  a contract rather than the concrete struct.
- **`emit_empty_slices`** makes list queries return `[]` rather than `nil`, so
  a JSON list is never `null`.
- **`emit_pointers_for_null_types`** makes optional filters `*string`, where
  `nil` means "no filter" — the filter is applied in SQL, never by building a
  query string in Go.

Every tenant-owned query takes `workspace_id` as its first parameter, search
runs as a parameterised `ILIKE` inside PostgreSQL, and pagination is `LIMIT`/
`OFFSET` with a deterministic `ORDER BY created_at DESC, id DESC`.

### Statements behind the accept flow

| Query | Role |
| --- | --- |
| `GetOrderRequestForUpdate` | `SELECT ... FOR UPDATE` on the request, so concurrent accepts serialise |
| `SumOrderRequestQuantitiesByProduct` | Aggregates lines by product, so a repeated product is decremented once |
| `LockProductsForUpdate` | Locks products in ascending id order — the deadlock guard |
| `DecrementProductStock` | `WHERE stock >= quantity`, so an oversell updates no row |
| `MarkOrderRequestAccepted` | `WHERE status = 'new'`, making the transition atomic |
| `CreateOrder` | One draft order; the unique constraint rejects a second |
| `CopyOrderItemsFromRequest` | Builds order lines from the request snapshots without leaving the database |

`internal/db/sqlc/queries_test.go` runs these against a real PostgreSQL inside
a transaction that is always rolled back. It skips when `DATABASE_URL` is
unset:

```bash
make check                      # includes the database tests when .env is present
go test ./internal/db/sqlc/ -v
```

## Schema

Six tables. Every tenant-owned table carries `workspace_id NOT NULL`, money is
`NUMERIC(12,2)` with an explicit `CHAR(3)` currency, and identifiers are UUIDs
defaulting to `gen_random_uuid()`.

| Table | Purpose |
| --- | --- |
| `workspaces` | Tenant root: name, unique slug |
| `products` | Catalogue: price, currency, stock |
| `order_requests` | Buyer submission: name, email, status `new`/`accepted`/`rejected` |
| `order_request_items` | Request lines with product name, unit price and currency snapshots |
| `orders` | Draft order created on acceptance |
| `order_items` | Order lines copied from the request snapshots |

### Tenant isolation

Foreign keys between tenant-owned tables are **composite on
`(workspace_id, <id>)`**, not on the id alone:

```
order_request_items (workspace_id, order_request_id) -> order_requests (workspace_id, id)
order_request_items (workspace_id, product_id)       -> products       (workspace_id, id)
orders              (workspace_id, order_request_id) -> order_requests (workspace_id, id)
order_items         (workspace_id, order_id)         -> orders         (workspace_id, id)
order_items         (workspace_id, product_id)       -> products       (workspace_id, id)
```

A row therefore cannot reference a parent in another workspace — the database
rejects it, rather than relying on every query being written correctly. These
references require `UNIQUE (workspace_id, id)` on `products`, `order_requests`
and `orders`, which is why that constraint exists alongside the primary key.

### Guarantees enforced by the database

| Rule | Mechanism |
| --- | --- |
| One order per request | `UNIQUE (workspace_id, order_request_id)` on `orders` |
| Stock never negative | `CHECK (stock >= 0)` on `products` |
| Quantities positive | `CHECK (quantity > 0)` on both item tables |
| Prices never negative | `CHECK (price >= 0)`, `CHECK (unit_price >= 0)` |
| Valid statuses | `CHECK (status IN ('new','accepted','rejected'))`, `CHECK (status = 'draft')` |
| ISO-shaped currency | `CHECK (currency ~ '^[A-Z]{3}$')` |
| One line per product per request | `UNIQUE (workspace_id, order_request_id, product_id)` |
| Referenced products survive | `ON DELETE RESTRICT` on item → product references |

The unique constraint on `orders` is deliberately a **second** line of defence.
The accept flow locks the request row with `SELECT ... FOR UPDATE` inside a
transaction; the constraint guarantees correctness even if that were wrong.

`make verify-schema` asserts all of the above against a live database with 14
checks, including both cross-tenant cases.

### Indexes

| Index | Access pattern |
| --- | --- |
| `products (workspace_id, name)` | Catalogue listing and search |
| `order_requests (workspace_id, status, created_at DESC, id DESC)` | Sales listing with status filter and stable ordering |
| `order_request_items (workspace_id, order_request_id)` | Loading a request's lines |
| `orders (workspace_id, created_at DESC, id DESC)` | Order listing |
| `order_items (workspace_id, order_id)` | Loading an order's lines |

The unique constraints on `(workspace_id, id)` double as lookup indexes for
workspace-scoped fetches by id.

### Decisions and limitations

- **No `updated_at` trigger.** Services set `updated_at` explicitly in the same
  statement that changes a row, which keeps the write path visible in the
  queries rather than hidden in a trigger.
- **Mixed currencies are not blocked by the database.** A single request must
  use one currency for its total to be meaningful; that rule is enforced in the
  service layer, since expressing it as a constraint would require a trigger.
- **Email validation is deliberately loose** in the database — a shape check
  only. Full validation happens at the HTTP boundary with validator/v10.
- **`ON DELETE CASCADE` from `workspaces`** means deleting a workspace removes
  its data. Nothing in the API deletes workspaces; this keeps test cleanup
  simple.

## Layout

```
cmd/api          HTTP server entry point
cmd/seed         idempotent demo data (implemented in a later phase)
internal/config  environment configuration and .env loading
internal/db      pgx connection pool
internal/middleware   CORS, and workspace scoping in a later phase
internal/modules      workspace, product, orderrequest, order
internal/platform     httpx envelopes, logging, server, startup output, validation
db/migrations    Atlas versioned migrations
db/queries       sqlc query files (a later phase)
db/verify        schema invariant checks
```
