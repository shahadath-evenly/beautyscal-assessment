/**
 * API configuration.
 *
 * `VITE_API_BASE_URL` is inlined at build time by Vite, so changing it means
 * rebuilding. It defaults to the backend's local port.
 */
export const API_BASE_URL: string =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8081";

/** Header the backend uses to scope every request to one tenant. */
export const WORKSPACE_HEADER = "X-Workspace-ID";

/** Default page size, matching the backend default. */
export const DEFAULT_PAGE_SIZE = 10;

/** Page sizes offered in listing controls. */
export const PAGE_SIZE_OPTIONS = [10, 25, 50, 100];
