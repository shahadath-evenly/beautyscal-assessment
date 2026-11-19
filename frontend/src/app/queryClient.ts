import { QueryClient } from "@tanstack/react-query";

import { ApiError } from "../service";

/**
 * Builds the TanStack Query client.
 *
 * Returned from a function rather than created at module scope, so tests can
 * construct an isolated client and the app can hold one in state.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        // A 4xx will not succeed on a retry: a missing workspace, an unknown
        // resource or a conflict is a decision, not a hiccup. Only transient
        // failures are retried, and only once.
        retry: (failureCount, error) => {
          if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
            return false;
          }
          return failureCount < 1;
        },
      },
      mutations: {
        // Accepting must never be retried automatically: a retry could race
        // with the first attempt, and the server would answer 409 for a call
        // the user did not knowingly make.
        retry: false,
      },
    },
  });
}
