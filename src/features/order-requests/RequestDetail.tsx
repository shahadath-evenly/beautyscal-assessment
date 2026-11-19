import {
  Alert,
  Button,
  ErrorState,
  LoadingState,
  Modal,
  StatusBadge,
  describeError,
} from "../../components/common";
import { formatDateTime, formatMoney } from "../../utils/format";
import { useOrderRequest } from "./useOrderRequest";

export interface RequestDetailProps {
  requestId: string | null;
  onClose: () => void;
  onAccept: (requestId: string) => void;
  onReject: (requestId: string) => void;
  /** True while this request's accept or reject is in flight. */
  pending: boolean;
  /** The failure from the last action on this request, if it failed. */
  actionError?: unknown;
}

/**
 * One request in a dialog: buyer, status, lines and total, with Accept and
 * Reject.
 *
 * Every figure comes from the server's stored snapshots, not from the
 * catalogue, so a request shows the prices that applied when it was
 * submitted even if the product has been repriced since.
 */
export function RequestDetail({
  requestId,
  onClose,
  onAccept,
  onReject,
  pending,
  actionError,
}: RequestDetailProps) {
  const query = useOrderRequest(requestId);
  const request = query.data;
  const decided = request !== undefined && request.status !== "new";

  return (
    <Modal
      open={requestId !== null}
      title="Order request"
      description="Review the lines, then accept to create a draft order or reject the request."
      onClose={onClose}
      dismissible={!pending}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={pending}>
            Close
          </Button>
          {request && request.status === "new" && (
            <>
              <Button
                variant="danger"
                onClick={() => onReject(request.id)}
                disabled={pending}
              >
                Reject
              </Button>
              <Button
                variant="success"
                onClick={() => onAccept(request.id)}
                loading={pending}
              >
                Accept request
              </Button>
            </>
          )}
        </>
      }
    >
      {query.isPending && <LoadingState message="Loading the request…" />}

      {query.error !== null && !query.isPending && (
        <ErrorState error={query.error} onRetry={() => void query.refetch()} />
      )}

      {request && (
        <>
          {actionError !== undefined && actionError !== null && (
            <Alert tone="danger" title={describeError(actionError).title}>
              {describeError(actionError).description ??
                "The request was not changed."}
            </Alert>
          )}

          {decided && (
            <Alert tone={request.status === "accepted" ? "success" : "info"}>
              {request.status === "accepted"
                ? "This request has been accepted and its stock reserved on a draft order."
                : "This request has been rejected. No stock was reserved."}
            </Alert>
          )}

          <dl className="summary-list">
            <div className="summary-list__row">
              <dt>Reference</dt>
              <dd className="summary-list__mono">{request.id}</dd>
            </div>
            <div className="summary-list__row">
              <dt>Status</dt>
              <dd>
                <StatusBadge status={request.status} />
              </dd>
            </div>
            <div className="summary-list__row">
              <dt>Buyer</dt>
              <dd>
                {request.buyerName}
                <span className="row-secondary">{request.buyerEmail}</span>
              </dd>
            </div>
            <div className="summary-list__row">
              <dt>Submitted</dt>
              <dd>{formatDateTime(request.createdAt)}</dd>
            </div>
          </dl>

          <table className="table confirmation-table">
            <caption className="visually-hidden">Lines of this request</caption>
            <thead>
              <tr>
                <th scope="col">Product</th>
                <th scope="col" className="table__cell--right">
                  Quantity
                </th>
                <th scope="col" className="table__cell--right">
                  Unit price
                </th>
                <th scope="col" className="table__cell--right">
                  Line total
                </th>
              </tr>
            </thead>
            <tbody>
              {request.items.map((item) => (
                <tr key={item.productId}>
                  <td>{item.productName}</td>
                  <td className="table__cell--right row-money">{item.quantity}</td>
                  <td className="table__cell--right row-money">
                    {formatMoney(item.unitPrice, item.currency)}
                  </td>
                  <td className="table__cell--right row-money">
                    {formatMoney(item.lineTotal, item.currency)}
                  </td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr>
                <th scope="row" colSpan={3} className="table__cell--right">
                  Total
                </th>
                <td className="table__cell--right row-money">
                  <strong>{formatMoney(request.total, request.currency)}</strong>
                </td>
              </tr>
            </tfoot>
          </table>
        </>
      )}
    </Modal>
  );
}
