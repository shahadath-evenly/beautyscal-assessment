import { useQuery } from "@tanstack/react-query";

import { orderRequestsService, queryKeys } from "../../service";
import { useWorkspaceId } from "../../hooks/useWorkspace";
import type { OrderRequestDetail } from "../../types/orderRequest";

/**
 * One request with its lines and totals.
 *
 * The listing carries only a summary, so the detail view fetches the request
 * again rather than reusing a row: the lines, their price snapshots and the
 * status may all have moved on since the page was loaded.
 */
export function useOrderRequest(requestId: string | null) {
  const workspaceId = useWorkspaceId();

  return useQuery<OrderRequestDetail>({
    queryKey: queryKeys.orderRequests.detail(workspaceId ?? "none", requestId ?? "none"),
    queryFn: ({ signal }) => orderRequestsService.get(requestId as string, signal),
    enabled: workspaceId !== null && requestId !== null,
  });
}
