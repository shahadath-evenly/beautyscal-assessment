-- Workspaces are the tenant root. They are the only table not scoped by a
-- workspace ID, because they define it.

-- name: GetWorkspace :one
SELECT id, name, slug, created_at
FROM workspaces
WHERE id = $1;

-- name: GetWorkspaceBySlug :one
SELECT id, name, slug, created_at
FROM workspaces
WHERE slug = $1;

-- name: ListWorkspaces :many
-- Used by the seed command and the demo workspace selector. The set is tiny
-- and bounded, so it is not paginated.
SELECT id, name, slug, created_at
FROM workspaces
ORDER BY name ASC, id ASC;

-- name: WorkspaceExists :one
-- Cheap existence check for the X-Workspace-ID middleware, which must reject
-- an unknown workspace without loading the row.
SELECT EXISTS (
    SELECT 1 FROM workspaces WHERE id = $1
) AS workspace_exists;

-- name: CreateWorkspace :one
-- Idempotent: re-running the seed updates the name rather than failing on the
-- unique slug, so demo data can be refreshed safely.
INSERT INTO workspaces (name, slug)
VALUES ($1, $2)
ON CONFLICT (slug) DO UPDATE
    SET name = EXCLUDED.name
RETURNING id, name, slug, created_at;
