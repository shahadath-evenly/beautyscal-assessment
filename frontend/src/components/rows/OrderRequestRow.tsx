import type { ReactNode } from "react";

import { StatusBadge } from "../common";
import type { OrderRequest } from "../../types/orderRequest";
import { formatDateTime, formatMoney } from "../../utils/format";

export interface OrderRequestRowProps {
  request: OrderRequest;
  /** View, Accept and Reject are passed in by the sales feature. The row
   *  shows no action of its own, and never Edit or Delete, which this
   *  workflow does not have. */
  actions?: ReactNode;
}

/** One order request as a table row. */
export function OrderRequestRow({ request, actions }: OrderRequestRowProps) {
  return (
    <tr>
      <td>
        <span className="row-primary">{request.buyerName}</span>
        <span className="row-secondary">{request.buyerEmail}</span>
      </td>
      <td>
        <StatusBadge status={request.status} />
      </td>
      <td className="table__cell--right table__cell--desktop">
        <span className="row-muted">{request.itemCount}</span>
      </td>
      <td className="table__cell--right">
        <span className="row-money">{formatMoney(request.total, request.currency)}</span>
      </td>
      <td className="table__cell--desktop">
        <span className="row-muted">{formatDateTime(request.createdAt)}</span>
      </td>
      {actions}
    </tr>
  );
}
