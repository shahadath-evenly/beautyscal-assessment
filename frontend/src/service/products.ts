import { request } from "./client";
import type { PageParams, Paginated } from "../types/api";
import type { Product } from "../types/product";

/** Endpoints for the workspace's product catalogue. */
export const productsService = {
  /** GET /api/products — one page of the catalogue. */
  async list(params: PageParams = {}, signal?: AbortSignal): Promise<Paginated<Product>> {
    const response = await request<Product[]>("/api/products", {
      query: { page: params.page, limit: params.limit, search: params.search },
      signal,
    });
    // Listings always carry meta; the fallback keeps the type honest.
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

  /** GET /api/products/{id} — one product. */
  async get(productId: string, signal?: AbortSignal): Promise<Product> {
    const response = await request<Product>(`/api/products/${productId}`, { signal });
    return response.data;
  },
};
