/** Formatting helpers shared by row components and pages. */

/**
 * Renders an ISO timestamp in the viewer's locale and time zone. Returns an
 * em dash for a value that cannot be parsed, so a bad date never breaks a row.
 */
export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "—";

  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Renders an amount with its currency. The amount stays the exact decimal
 * string the API sent; it is never parsed into a number, which would lose
 * precision.
 */
export function formatMoney(amount: string, currency: string): string {
  return `${amount} ${currency}`;
}
