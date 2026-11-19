/** Shapes of the API's success and error envelopes. */

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

/** A list response, where `meta` is always present. */
export interface Paginated<T> {
  data: T[];
  meta: Meta;
}

/** One field-level problem, as the API reports it. */
export interface FieldError {
  field: string;
  message: string;
  rule?: string;
}

/** Error envelope: `{ error: { code, message, details? } }`. */
export interface ErrorEnvelope {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}

/** Query parameters shared by every listing. */
export interface PageParams {
  page?: number;
  limit?: number;
  search?: string;
}
