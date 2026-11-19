# BeautyScale frontend

React + Vite + TypeScript client for the Brand Order Requests workflow, using
TanStack Query for server state and React Router for routing.

## Commands

| Command | Purpose |
| --- | --- |
| `npm install` | Install dependencies |
| `npm run dev` | Dev server on http://localhost:5173 |
| `npm run typecheck` | TypeScript, no emit |
| `npm run lint` | oxlint |
| `npm run build` | Typecheck and production build to `dist/` |
| `npm run preview` | Serve the production build |

## Configuration

Copy `.env.example` to `.env` to override `VITE_API_BASE_URL`, which defaults
to `http://localhost:8081`.

## Configuration

Copy `.env.example` to `.env`:

| Variable | Purpose |
| --- | --- |
| `VITE_API_BASE_URL` | Backend URL, default `http://localhost:8081` |
| `VITE_WORKSPACES` | Demo workspaces as JSON, from the backend's `make seed` output |

Both are inlined by Vite at build time, so changing either means restarting
the dev server or rebuilding the image.

`VITE_WORKSPACES` looks like:

```
VITE_WORKSPACES=[{"id":"84395f84-…","name":"Lumière Paris"}]
```

Leave it unset and the workspace switcher accepts a pasted workspace ID
instead, so a fresh clone still works.

## Structure

- `src/service/` — **every API call is declared here**: `client.ts` (fetch,
  workspace header, envelope unwrapping), `ApiError`, `products`,
  `orderRequests`, `workspaces`, `queryKeys`
- `src/app/` — composition root, TanStack Query client, workspace provider
- `src/hooks/` — `useWorkspace`, `useDebouncedValue`
- `src/routes/` — route table and path helpers
- `src/layouts/` — application shell with navigation and the workspace switcher
- `src/pages/` — buyer, sales and not-found pages
- `src/components/common/` — buttons, inputs, badges, cards, and the loading,
  empty, error and alert states
- `src/components/table/` — the generic `DataTable` and its pagination
- `src/components/rows/` — per-resource row renderers
- `src/features/` — feature queries, mutations and components
- `src/utils/` — date and money formatting
- `src/types/` — API, product, order request and workspace types
- `src/styles/` — theme tokens, base styles and component styles

## Conventions

**The workspace header is attached centrally.** `setActiveWorkspaceId` tells
the service layer which tenant to send, and `client.ts` attaches
`X-Workspace-ID` to every request, so no call site can forget it.

**Query keys start with the workspace ID**, so two workspaces never share a
cache entry. Switching workspaces also clears the cache, which removes any
chance of showing the previous tenant's rows during a refetch.

**Money is never parsed into a number.** Amounts stay the exact decimal
strings the API sends; turning `"49.90"` into a float would lose precision.

**The table owns no feature actions.** `DataTable` renders whatever columns,
rows and row actions its caller passes, so order requests show View, Accept
and Reject, and nothing shows Edit or Delete, which this workflow has no
operations for.

Setup, migrations, seeding and API documentation live in the backend README.
