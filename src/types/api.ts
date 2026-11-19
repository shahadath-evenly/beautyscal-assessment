/** Shared shapes of the API's success and error envelopes. */

/** Pagination metadata returned by every list endpoint. */
export interface Meta {
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}

/** Success envelope: `{ data, meta? }`. */
export interface Envelope<T> {
  data: T;
  meta?: Meta;
}

/** A paginated response, where `meta` is always present. */
export interface Paginated<T> {
  data: T[];
  meta: Meta;
}

/** Error envelope: `{ error: { code, message, details? } }`. */
export interface ErrorEnvelope {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}

/** Query parameters common to every listing. */
export interface PageParams {
  page?: number;
  limit?: number;
  search?: string;
}
