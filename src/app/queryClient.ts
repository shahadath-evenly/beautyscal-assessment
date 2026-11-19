import { QueryClient } from "@tanstack/react-query";

import { ApiError } from "../api/ApiError";

/**
 * Builds the TanStack Query client. Returned from a function rather than
 * created at module scope so tests can construct an isolated client.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        // Client errors (bad workspace, not found, conflict) will not succeed
        // on a retry, so only retry transient failures, and only once.
        retry: (failureCount, error) => {
          if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
            return false;
          }
          return failureCount < 1;
        },
      },
      mutations: {
        // Accept and reject must never be retried automatically: a retry could
        // race with the first attempt.
        retry: false,
      },
    },
  });
}
