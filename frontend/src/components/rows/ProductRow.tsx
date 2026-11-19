import type { ReactNode } from "react";

import type { Product } from "../../types/product";
import { formatMoney } from "../../utils/format";

export interface ProductRowProps {
  product: Product;
  /** Controls supplied by the calling feature, such as a quantity picker. */
  actions?: ReactNode;
}

/**
 * One product as a table row.
 *
 * Presentational only: it decides how a product looks, never what can be done
 * to it. The buyer page passes a quantity control as `actions`; a read-only
 * listing passes none.
 */
export function ProductRow({ product, actions }: ProductRowProps) {
  return (
    <tr>
      <td>
        <span className="row-primary">{product.name}</span>
      </td>
      <td className="table__cell--right">
        <span className="row-money">{formatMoney(product.price, product.currency)}</span>
      </td>
      <td className="table__cell--right">
        <StockCell stock={product.stock} />
      </td>
      {actions}
    </tr>
  );
}

/** Stock, flagged when it is low or gone, so a buyer sees it before ordering. */
function StockCell({ stock }: { stock: number }) {
  if (stock <= 0) {
    return <span className="badge badge--rejected">Out of stock</span>;
  }
  if (stock < 10) {
    return <span className="badge badge--low">{stock} left</span>;
  }
  return <span className="row-muted">{stock}</span>;
}
