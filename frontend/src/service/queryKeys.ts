import type { OrderRequestListParams } from "./orderRequests";
import type { PageParams } from "../types/api";

/**
 * Query keys for TanStack Query.
 *
 * Every key begins with the workspace ID. Two workspaces therefore never share
 * a cache entry, so switching workspaces cannot show the previous tenant's
 * rows while a refetch is in flight. Keys also carry every page, filter and
 * search parameter, so a cached page is only reused for the same query.
 */
export const queryKeys = {
  products: {
    all: (workspaceId: string) => ["products", workspaceId] as const,
    list: (workspaceId: string, params: PageParams) =>
      [
        "products",
        workspaceId,
        "list",
        params.page ?? 1,
        params.limit ?? null,
        params.search ?? "",
      ] as const,
    detail: (workspaceId: string, productId: string) =>
      ["products", workspaceId, "detail", productId] as const,
  },

  orderRequests: {
    all: (workspaceId: string) => ["order-requests", workspaceId] as const,
    list: (workspaceId: string, params: OrderRequestListParams) =>
      [
        "order-requests",
        workspaceId,
        "list",
        params.page ?? 1,
        params.limit ?? null,
        params.search ?? "",
        params.status ?? "",
      ] as const,
    detail: (workspaceId: string, requestId: string) =>
      ["order-requests", workspaceId, "detail", requestId] as const,
  },
};
