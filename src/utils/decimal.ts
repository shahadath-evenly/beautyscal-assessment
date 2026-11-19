/**
 * Exact decimal arithmetic for money, on strings.
 *
 * Amounts arrive from the API as decimal strings because JSON numbers are
 * IEEE doubles in every browser, and a double cannot hold 0.1 exactly. Doing
 * the arithmetic in `bigint` minor units keeps every total exact: 0.10 × 3 is
 * 0.30, not 0.30000000000000004.
 *
 * These totals are for display only. The server recomputes every amount from
 * its own catalogue when a request is submitted, so nothing here is trusted.
 */

/** Decimal places the schema stores, matching NUMERIC(12,2). */
const SCALE = 2;

/** Parses a decimal string into minor units, e.g. "49.90" to 4990n. */
export function toMinorUnits(amount: string): bigint {
  const trimmed = amount.trim();
  if (!/^-?\d+(\.\d+)?$/.test(trimmed)) {
    throw new Error(`not a decimal amount: ${amount}`);
  }

  const negative = trimmed.startsWith("-");
  const digits = negative ? trimmed.slice(1) : trimmed;
  const [whole, fraction = ""] = digits.split(".");

  // Pad or truncate to the stored scale. Values come from the API already at
  // scale 2, so truncation only ever drops zeros.
  const padded = (fraction + "0".repeat(SCALE)).slice(0, SCALE);
  const value = BigInt(whole + padded);

  return negative ? -value : value;
}

/** Renders minor units back as a decimal string, always with two places. */
export function fromMinorUnits(minor: bigint): string {
  const negative = minor < 0n;
  const digits = (negative ? -minor : minor).toString().padStart(SCALE + 1, "0");

  const split = digits.length - SCALE;
  const formatted = `${digits.slice(0, split)}.${digits.slice(split)}`;

  return negative ? `-${formatted}` : formatted;
}

/** Multiplies a decimal amount by a whole quantity, exactly. */
export function multiply(amount: string, quantity: number): string {
  if (!Number.isInteger(quantity)) {
    throw new Error(`quantity must be a whole number: ${quantity}`);
  }
  return fromMinorUnits(toMinorUnits(amount) * BigInt(quantity));
}

/** Adds decimal amounts, exactly. Callers must only sum one currency. */
export function sum(amounts: string[]): string {
  return fromMinorUnits(
    amounts.reduce((total, amount) => total + toMinorUnits(amount), 0n),
  );
}
