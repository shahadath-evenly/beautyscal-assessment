import type { ErrorEnvelope, FieldError } from "../types/api";

/**
 * An error raised by the service layer.
 *
 * It carries the backend's error code and HTTP status, so features can
 * distinguish a conflict (409) from a validation failure (422) and render
 * something useful rather than a generic message.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: unknown;

  constructor(status: number, code: string, message: string, details?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }

  /** True when the resource's current state forbids the action. */
  get isConflict(): boolean {
    return this.status === 409;
  }

  /** True when the request was rejected for invalid input. */
  get isValidation(): boolean {
    return this.status === 400 || this.status === 422;
  }

  /** True when the resource does not exist, or belongs to another workspace. */
  get isNotFound(): boolean {
    return this.status === 404;
  }

  /** True when the server could not be reached at all. */
  get isNetwork(): boolean {
    return this.status === 0;
  }

  /** True when accepting failed because the catalogue is short of stock. */
  get isInsufficientStock(): boolean {
    return this.code === "insufficient_stock";
  }

  /**
   * Field-level problems, when the API reported any. Returns an empty array
   * otherwise, so callers can map over it without checking.
   */
  get fieldErrors(): FieldError[] {
    if (!Array.isArray(this.details)) return [];
    return this.details.filter(
      (detail): detail is FieldError =>
        typeof detail === "object" &&
        detail !== null &&
        "field" in detail &&
        "message" in detail,
    );
  }

  /** Builds an ApiError from a failed response and its decoded body. */
  static fromResponse(status: number, payload: unknown): ApiError {
    const envelope = payload as ErrorEnvelope | undefined;
    return new ApiError(
      status,
      envelope?.error?.code ?? "unexpected_error",
      envelope?.error?.message ?? "Something went wrong. Please try again.",
      envelope?.error?.details,
    );
  }
}
