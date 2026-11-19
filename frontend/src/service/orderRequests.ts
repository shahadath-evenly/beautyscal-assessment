import { request } from "./client";
import type { PageParams, Paginated } from "../types/api";
import type {
  AcceptResult,
  CreateOrderRequestInput,
  OrderRequest,
  OrderRequestDetail,
  OrderRequestStatus,
} from "../types/orderRequest";

/** Listing parameters, adding the status filter to the shared page params. */
export interface OrderRequestListParams extends PageParams {
  status?: OrderRequestStatus | "";
}

/** Endpoints for buyer requests and the sales review flow. */
export const orderRequestsService = {
  /** POST /api/order-requests — submit a buyer request. */
  async create(
    input: CreateOrderRequestInput,
    signal?: AbortSignal,
  ): Promise<OrderRequestDetail> {
    const response = await request<OrderRequestDetail>("/api/order-requests", {
      method: "POST",
      body: input,
      signal,
    });
    return response.data;
  },

  /** GET /api/order-requests — one page of requests. */
  async list(
    params: OrderRequestListParams = {},
    signal?: AbortSignal,
  ): Promise<Paginated<OrderRequest>> {
    const response = await request<OrderRequest[]>("/api/order-requests", {
      query: {
        page: params.page,
        limit: params.limit,
        search: params.search,
        status: params.status || undefined,
      },
      signal,
    });
    return {
      data: response.data,
      meta: response.meta ?? {
        page: params.page ?? 1,
        limit: params.limit ?? response.data.length,
        total: response.data.length,
        totalPages: 1,
      },
    };
  },

  /** GET /api/order-requests/{id} — a request with its lines and totals. */
  async get(requestId: string, signal?: AbortSignal): Promise<OrderRequestDetail> {
    const response = await request<OrderRequestDetail>(`/api/order-requests/${requestId}`, {
      signal,
    });
    return response.data;
  },

  /**
   * POST /api/order-requests/{id}/accept — reserve stock and create the draft
   * order. Safe to call twice: the second attempt returns 409 rather than
   * creating a second order.
   */
  async accept(requestId: string, signal?: AbortSignal): Promise<AcceptResult> {
    const response = await request<AcceptResult>(
      `/api/order-requests/${requestId}/accept`,
      { method: "POST", signal },
    );
    return response.data;
  },

  /** POST /api/order-requests/{id}/reject — reject without touching stock. */
  async reject(requestId: string, signal?: AbortSignal): Promise<OrderRequestDetail> {
    const response = await request<OrderRequestDetail>(
      `/api/order-requests/${requestId}/reject`,
      { method: "POST", signal },
    );
    return response.data;
  },
};
