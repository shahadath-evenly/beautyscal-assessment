import { useMutation, useQueryClient } from "@tanstack/react-query";

import { orderRequestsService, queryKeys } from "../../service";
import { useWorkspaceId } from "../../hooks/useWorkspace";
import type { CreateOrderRequestInput, OrderRequestDetail } from "../../types/orderRequest";

/**
 * Submits a buyer request.
 *
 * Mutations never retry (see the query client), which matters here: a retried
 * submission would create a second request for one click.
 */
export function useCreateOrderRequest() {
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceId();

  return useMutation<OrderRequestDetail, Error, CreateOrderRequestInput>({
    mutationFn: (input) => orderRequestsService.create(input),
    onSuccess: () => {
      if (!workspaceId) return;
      // The sales listing now has a new row, and stock is unchanged until
      // acceptance, so only the request lists need refreshing.
      void queryClient.invalidateQueries({
        queryKey: queryKeys.orderRequests.all(workspaceId),
      });
    },
  });
}
