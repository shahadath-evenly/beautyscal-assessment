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

## Structure

- `src/api/` — the API/service layer: `http.ts` request helper, `ApiError`, config
- `src/app/` — composition root and TanStack Query client
- `src/routes/` — route table and path helpers
- `src/layouts/` — application shell
- `src/pages/` — buyer and sales pages
- `src/components/table/` — the generic `DataTable`
- `src/components/rows/` — per-resource row renderers
- `src/features/` — feature queries, mutations and components
- `src/styles/` — theme tokens and global styles

Setup, migrations, seeding and API documentation live in the repository README.
