import type { OrderRequestStatus } from "../../types/orderRequest";

const LABELS: Record<OrderRequestStatus, string> = {
  new: "New",
  accepted: "Accepted",
  rejected: "Rejected",
};

/** Colour-coded badge for a request's status: new violet, accepted green,
 *  rejected red. The label carries the meaning; colour only reinforces it. */
export function StatusBadge({ status }: { status: OrderRequestStatus }) {
  return <span className={`badge badge--${status}`}>{LABELS[status] ?? status}</span>;
}
