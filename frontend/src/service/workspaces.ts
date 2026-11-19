import type { Workspace } from "../types/workspace";

/**
 * The demo workspaces offered by the switcher.
 *
 * This is a demo convenience, not authentication. The backend derives the
 * tenant solely from the X-Workspace-ID header it validates; choosing here
 * only decides which header value is sent.
 *
 * The list comes from `VITE_WORKSPACES`, a JSON array of `{id, name}` that
 * the seed command's output fills in:
 *
 *   VITE_WORKSPACES=[{"id":"84395f84-…","name":"Lumière Paris"}]
 *
 * When it is unset or malformed the switcher falls back to letting the user
 * paste a workspace ID, so the app still works on a fresh clone.
 */
function parseConfigured(): Workspace[] {
  const raw = import.meta.env.VITE_WORKSPACES;
  if (!raw) return [];

  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];

    return parsed.filter(
      (entry): entry is Workspace =>
        typeof entry === "object" &&
        entry !== null &&
        typeof (entry as Workspace).id === "string" &&
        typeof (entry as Workspace).name === "string",
    );
  } catch {
    // A malformed value must not break the app; the manual entry remains.
    return [];
  }
}

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const workspacesService = {
  /** The configured demo workspaces, possibly empty. */
  list(): Workspace[] {
    return parseConfigured();
  },

  /** Whether a pasted value is a usable workspace ID. */
  isValidId(value: string): boolean {
    return UUID_PATTERN.test(value.trim());
  },
};
