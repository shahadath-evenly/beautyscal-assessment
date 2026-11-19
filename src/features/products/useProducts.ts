import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { productsService, queryKeys } from "../../service";
import { useWorkspaceId } from "../../hooks/useWorkspace";
import type { PageParams, Paginated } from "../../types/api";
import type { Product } from "../../types/product";

/**
 * One page of the current workspace's catalogue.
 *
 * The key carries the workspace and every page and search parameter, so two
 * workspaces never share a cache entry and a cached page is reused only for
 * the same query. `keepPreviousData` holds the current rows visible while the
 * next page loads, so paging does not blank the table.
 */
export function useProducts(params: PageParams) {
  const workspaceId = useWorkspaceId();

  return useQuery<Paginated<Product>>({
    queryKey: queryKeys.products.list(workspaceId ?? "none", params),
    queryFn: ({ signal }) => productsService.list(params, signal),
    // Nothing is fetched before a workspace is chosen: the API would reject
    // it, and the error would be noise rather than information.
    enabled: workspaceId !== null,
    placeholderData: keepPreviousData,
  });
}
