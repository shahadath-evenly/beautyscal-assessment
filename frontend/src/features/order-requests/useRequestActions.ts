import { useMutation, useQueryClient } from "@tanstack/react-query";

import { orderRequestsService, queryKeys } from "../../service";
import { useWorkspaceId } from "../../hooks/useWorkspace";
import type {
  AcceptResult,
  OrderRequestDetail,
} from "../../types/orderRequest";

/**
 * Accept and reject, as the sales page uses them.
 *
 * Mutations never retry (see the query client). That matters more here than
 * anywhere else: a retried accept would be a second attempt to reserve stock,
 * and although the backend answers the second one with 409 rather than
 * creating a second order, the retry would turn a success into a visible
 * conflict.
 *
 * Both mutations expose the request they are working on through
 * `mutation.variables`, so the page can disable exactly the row in flight
 * without tracking its own state.
 */
export function useRequestActions() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();

  /** Refreshes everything a transition invalidated. */
  const refresh = (detail: OrderRequestDetail) => {
    if (!workspaceId) return;

    // The detail is already in hand, so seed it rather than refetching: the
    // open panel shows the new status immediately.
    queryClient.setQueryData(
      queryKeys.orderRequests.detail(workspaceId, detail.id),
      detail,
    );
    // Status changed, so every filtered listing may now be wrong.
    void queryClient.invalidateQueries({
      queryKey: queryKeys.orderRequests.all(workspaceId),
    });
  };

  const accept = useMutation<AcceptResult, Error, string>({
    mutationFn: (requestId) => orderRequestsService.accept(requestId),
    onSuccess: (result) => {
      refresh(result.request);
      if (!workspaceId) return;
      // Acceptance decremented stock, so the buyer catalogue is now stale.
      void queryClient.invalidateQueries({
        queryKey: queryKeys.products.all(workspaceId),
      });
    },
  });

  const reject = useMutation<OrderRequestDetail, Error, string>({
    mutationFn: (requestId) => orderRequestsService.reject(requestId),
    // Rejection never touches stock, so the catalogue is left alone.
    onSuccess: refresh,
  });

  /** The request currently being acted on, if any. */
  const pendingId =
    (accept.isPending ? accept.variables : undefined) ??
    (reject.isPending ? reject.variables : undefined) ??
    null;

  return { accept, reject, pendingId, busy: pendingId !== null };
}
