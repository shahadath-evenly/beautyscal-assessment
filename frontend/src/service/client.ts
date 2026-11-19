import { ApiError } from "./ApiError";
import { API_BASE_URL, WORKSPACE_HEADER } from "./config";
import type { Envelope } from "../types/api";

/**
 * The workspace every request is sent as.
 *
 * Held here rather than threaded through every call site, so no service
 * function can forget the header: `request` reads it on each call, and the
 * workspace provider keeps it in step with the selector. It is a module-level
 * value because there is exactly one active workspace per browser tab.
 */
let activeWorkspaceId: string | null = null;

/** Sets the workspace sent with every subsequent request. */
export function setActiveWorkspaceId(workspaceId: string | null): void {
  activeWorkspaceId = workspaceId;
}

/** Returns the workspace currently being sent. */
export function getActiveWorkspaceId(): string | null {
  return activeWorkspaceId;
}

/** Query parameters; `undefined`, `null` and empty values are dropped. */
export type QueryParams = Record<string, string | number | boolean | undefined | null>;

export interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  query?: QueryParams;
  body?: unknown;
  signal?: AbortSignal;
  /** Overrides the active workspace for this one call. */
  workspaceId?: string;
  /** Skips the workspace header entirely, for unscoped endpoints. */
  withoutWorkspace?: boolean;
}

function buildUrl(path: string, query?: QueryParams): string {
  const base = `${API_BASE_URL.replace(/\/$/, "")}/`;
  const url = new URL(path.replace(/^\//, ""), base);

  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== null && value !== "") {
      url.searchParams.set(key, String(value));
    }
  }
  return url.toString();
}

/**
 * Performs a JSON request and unwraps the success envelope.
 *
 * Every non-2xx response becomes an {@link ApiError}, so callers never check
 * `response.ok`, and TanStack Query sees a rejected promise it can surface.
 */
export async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<Envelope<T>> {
  const { method = "GET", query, body, signal, workspaceId, withoutWorkspace } = options;

  const headers: Record<string, string> = { Accept: "application/json" };

  const workspace = workspaceId ?? activeWorkspaceId;
  if (!withoutWorkspace && workspace) {
    headers[WORKSPACE_HEADER] = workspace;
  }
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  let response: Response;
  try {
    response = await fetch(buildUrl(path, query), {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    });
  } catch (cause) {
    // An aborted request is a cancellation, not a failure: let TanStack Query
    // handle it rather than reporting it to the user.
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new ApiError(
      0,
      "network_error",
      "Could not reach the server. Check that the API is running and try again.",
    );
  }

  // 204 has no body to parse.
  const payload: unknown =
    response.status === 204 ? undefined : await response.json().catch(() => undefined);

  if (!response.ok) {
    throw ApiError.fromResponse(response.status, payload);
  }

  return payload as Envelope<T>;
}
