import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { orderRequestsService, queryKeys, type OrderRequestListParams } from "../../service";
import { useWorkspaceId } from "../../hooks/useWorkspace";
import type { Paginated } from "../../types/api";
import type { OrderRequest } from "../../types/orderRequest";

/**
 * One page of the current workspace's order requests.
 *
 * The key carries the workspace and every page, search and status parameter,
 * so a cached page is reused only for exactly the same query and two
 * workspaces never share an entry. Paging and filtering happen in SQL; this
 * hook never slices the rows it is given.
 */
export function useOrderRequests(params: OrderRequestListParams) {
  const workspaceId = useWorkspaceId();

  return useQuery<Paginated<OrderRequest>>({
    queryKey: queryKeys.orderRequests.list(workspaceId ?? "none", params),
    queryFn: ({ signal }) => orderRequestsService.list(params, signal),
    // Without a workspace the API would reject the call, so the error would
    // be noise rather than information.
    enabled: workspaceId !== null,
    // Keeps the current rows visible while the next page loads.
    placeholderData: keepPreviousData,
  });
}
