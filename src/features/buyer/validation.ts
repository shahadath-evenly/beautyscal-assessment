import type { Cart } from "./useCart";
import type { BuyerDetails } from "./RequestSummary";

export type BuyerFieldErrors = Partial<Record<"name" | "email" | "items", string>>;

/**
 * Deliberately permissive: it catches an obviously wrong address without
 * rejecting a valid but unusual one. The server validates properly, and its
 * answer is the one that counts.
 */
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * Checks the form before it is sent.
 *
 * This is for fast feedback only. The same rules are enforced by the API, so
 * a request that slips past here is still rejected with field-level errors.
 */
export function validateRequest(buyer: BuyerDetails, cart: Cart): BuyerFieldErrors {
  const errors: BuyerFieldErrors = {};

  if (buyer.name.trim() === "") {
    errors.name = "Your name is required.";
  } else if (buyer.name.trim().length > 200) {
    errors.name = "Your name must be 200 characters or fewer.";
  }

  const email = buyer.email.trim();
  if (email === "") {
    errors.email = "An email address is required.";
  } else if (!EMAIL_PATTERN.test(email)) {
    errors.email = "Enter a valid email address.";
  }

  if (cart.isEmpty) {
    errors.items = "Add at least one product to your request.";
  }

  return errors;
}
