import { useCallback, useMemo, useState } from "react";

import type { Product } from "../../types/product";
import { multiply, sum } from "../../utils/decimal";

/** One selected product and how many of it the buyer wants. */
export interface CartLine {
  product: Product;
  quantity: number;
  /** quantity × unit price, computed exactly. For display only. */
  lineTotal: string;
}

export interface Cart {
  lines: CartLine[];
  /** The currency every line shares, or null when the cart is empty. */
  currency: string | null;
  /** Sum of the line totals, exact. For display only. */
  total: string;
  itemCount: number;
  isEmpty: boolean;

  quantityOf: (productId: string) => number;
  setQuantity: (product: Product, quantity: number) => void;
  remove: (productId: string) => void;
  clear: () => void;
  /** Why a product cannot be added, or null when it can. */
  blockedReason: (product: Product) => string | null;
}

/**
 * The buyer's selection.
 *
 * Held in component state rather than a store: it belongs to one page and is
 * deliberately not persisted, since prices and stock can change between
 * visits and a stale cart would quote amounts the server would reject.
 *
 * Totals here are for display. The server reads prices from its own catalogue
 * when the request is submitted, so nothing computed here is trusted.
 */
export function useCart(): Cart {
  const [quantities, setQuantities] = useState<Map<string, { product: Product; quantity: number }>>(
    new Map(),
  );

  const lines = useMemo<CartLine[]>(
    () =>
      Array.from(quantities.values()).map(({ product, quantity }) => ({
        product,
        quantity,
        lineTotal: multiply(product.price, quantity),
      })),
    [quantities],
  );

  const currency = lines[0]?.product.currency ?? null;

  const total = useMemo(
    () => sum(lines.map((line) => line.lineTotal)),
    [lines],
  );

  const setQuantity = useCallback((product: Product, quantity: number) => {
    setQuantities((previous) => {
      const next = new Map(previous);

      // Clamp to the stock the server reported, so the UI cannot ask for more
      // than exists. The server enforces this again at acceptance.
      const clamped = Math.max(0, Math.min(Math.trunc(quantity), product.stock));

      if (clamped <= 0) {
        next.delete(product.id);
      } else {
        next.set(product.id, { product, quantity: clamped });
      }
      return next;
    });
  }, []);

  const remove = useCallback((productId: string) => {
    setQuantities((previous) => {
      const next = new Map(previous);
      next.delete(productId);
      return next;
    });
  }, []);

  const clear = useCallback(() => setQuantities(new Map()), []);

  const quantityOf = useCallback(
    (productId: string) => quantities.get(productId)?.quantity ?? 0,
    [quantities],
  );

  const blockedReason = useCallback(
    (product: Product): string | null => {
      if (product.stock <= 0) {
        return "Out of stock";
      }
      // One request must use one currency, or its total would be meaningless.
      // The server rejects a mixed-currency request, so the UI prevents
      // building one rather than letting it fail on submit.
      if (currency !== null && product.currency !== currency) {
        return `This request is in ${currency}`;
      }
      return null;
    },
    [currency],
  );

  return {
    lines,
    currency,
    total,
    itemCount: lines.reduce((count, line) => count + line.quantity, 0),
    isEmpty: lines.length === 0,
    quantityOf,
    setQuantity,
    remove,
    clear,
    blockedReason,
  };
}
