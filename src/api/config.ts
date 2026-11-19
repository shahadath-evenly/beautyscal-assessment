/**
 * API base URL. Set `VITE_API_BASE_URL` to point the app at a different
 * backend; the default matches the local backend port used by this project.
 */
export const API_BASE_URL: string =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8081";

/** Header the backend uses to scope every request to one tenant. */
export const WORKSPACE_HEADER = "X-Workspace-ID";

/** Default page size, matching the backend default. */
export const DEFAULT_PAGE_SIZE = 10;
