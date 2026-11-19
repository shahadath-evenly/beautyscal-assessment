import { ApiError } from "../../service";

export interface DescribedError {
  title: string;
  description?: string;
}

/**
 * Turns a thrown value into something a person can act on.
 *
 * An ApiError carries a client-safe message from the server, so it is shown
 * as-is. Anything else gets a generic message, because an unexpected
 * exception may contain internals that should not be displayed.
 */
export function describeError(error: unknown): DescribedError {
  if (error instanceof ApiError) {
    if (error.isNetwork) {
      return {
        title: "Could not reach the server",
        description: "Check that the API is running, then try again.",
      };
    }
    if (error.isNotFound) {
      return { title: "Not found", description: error.message };
    }
    if (error.isInsufficientStock) {
      return {
        title: error.message,
        description: describeShortStock(error.details),
      };
    }
    if (error.isValidation && error.fieldErrors.length > 0) {
      return {
        title: error.message,
        description: error.fieldErrors
          .map((field) => `${field.field}: ${field.message}`)
          .join(" · "),
      };
    }
    return { title: error.message };
  }

  return { title: "Something went wrong", description: "Please try again." };
}

/** Renders the products an acceptance was short of. */
function describeShortStock(details: unknown): string | undefined {
  if (!Array.isArray(details)) return undefined;

  const lines = details
    .filter(
      (entry): entry is { productName: string; requested: number; available: number } =>
        typeof entry === "object" &&
        entry !== null &&
        "productName" in entry &&
        "requested" in entry &&
        "available" in entry,
    )
    .map(
      (entry) =>
        `${entry.productName}: ${entry.requested} requested, ${entry.available} available`,
    );

  return lines.length > 0 ? lines.join(" · ") : undefined;
}
