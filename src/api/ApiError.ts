/**
 * An error raised by the API layer. It carries the backend's error code and
 * HTTP status so features can render actionable messages and distinguish
 * conflicts (409) from validation failures (422).
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

  /** True when the request conflicted with the resource's current state. */
  get isConflict(): boolean {
    return this.status === 409;
  }

  /** True when the request body failed validation. */
  get isValidation(): boolean {
    return this.status === 400 || this.status === 422;
  }
}
