import { ApiError } from "./ApiError";
import { API_BASE_URL, WORKSPACE_HEADER } from "./config";
import type { Envelope, ErrorEnvelope } from "../types/api";

/** Options accepted by {@link request}. */
export interface RequestOptions {
  method?: "GET" | "POST";
  /** Workspace ID sent as the X-Workspace-ID header. */
  workspaceId?: string;
  /** Query string parameters; `undefined` and empty values are dropped. */
  query?: Record<string, string | number | undefined>;
  /** JSON request body. */
  body?: unknown;
  signal?: AbortSignal;
}

function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = new URL(path.replace(/^\//, ""), `${API_BASE_URL.replace(/\/$/, "")}/`);
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== "") {
      url.searchParams.set(key, String(value));
    }
  }
  return url.toString();
}

/**
 * Performs a JSON request against the API and unwraps the success envelope.
 * Non-2xx responses are turned into {@link ApiError}, so callers never have to
 * inspect `response.ok` themselves.
 */
export async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<Envelope<T>> {
  const { method = "GET", workspaceId, query, body, signal } = options;

  const headers: Record<string, string> = { Accept: "application/json" };
  if (workspaceId) {
    headers[WORKSPACE_HEADER] = workspaceId;
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
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new ApiError(0, "network_error", "Could not reach the server. Check your connection and try again.");
  }

  const payload: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    const envelope = payload as ErrorEnvelope | undefined;
    throw new ApiError(
      response.status,
      envelope?.error?.code ?? "unexpected_error",
      envelope?.error?.message ?? "Something went wrong. Please try again.",
      envelope?.error?.details,
    );
  }

  return payload as Envelope<T>;
}
