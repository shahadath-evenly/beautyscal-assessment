import { Alert, Button, Card, StatusBadge } from "../../components/common";
import { formatDateTime, formatMoney } from "../../utils/format";
import type { OrderRequestDetail } from "../../types/orderRequest";

export interface RequestConfirmationProps {
  request: OrderRequestDetail;
  onStartAnother: () => void;
}

/**
 * Shown after a successful submission.
 *
 * Every figure here comes from the server's response, not from the cart, so
 * what the buyer sees confirmed is exactly what was stored — including any
 * price the server corrected.
 */
export function RequestConfirmation({ request, onStartAnother }: RequestConfirmationProps) {
  return (
    <Card
      title="Request submitted"
      description="The sales team will review it and either accept or reject it."
      actions={
        <Button variant="primary" onClick={onStartAnother}>
          Start another request
        </Button>
      }
    >
      <Alert tone="success" title="Thank you">
        Your request has been received. Keep the reference below if you need to follow
        it up.
      </Alert>

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
        <caption className="visually-hidden">Lines of the submitted request</caption>
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
    </Card>
  );
}
